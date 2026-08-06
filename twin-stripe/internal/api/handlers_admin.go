package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// fundRequest is the JSON body for POST /admin/accounts/{id}/fund.
type fundRequest struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// AdminFundAccount handles POST /admin/accounts/{id}/fund.
// Adds funds to a connected account's available balance.
func (h *Handler) AdminFundAccount(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")

	// Verify account exists
	if _, ok := h.store.Accounts.Get(accountID); !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such account: '"+accountID+"'")
		return
	}

	var req fundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	if req.Amount <= 0 {
		twincore.Error(w, http.StatusBadRequest, "amount must be positive")
		return
	}
	if req.Currency == "" {
		req.Currency = "usd"
	}

	h.store.CreditBalance(accountID, req.Currency, req.Amount)

	// Record balance transaction for admin funding
	h.store.RecordBalanceTransaction("adjustment", "", req.Currency, req.Amount, 0)

	balance := h.store.GetBalance(accountID)
	twincore.JSON(w, http.StatusOK, balance)
}

// AdminAdvanceSubscriptions handles POST /admin/subscriptions/advance.
// Simulates time passing: transitions trialing→active, handles cancel_at_period_end,
// and renews active subscriptions past their period end.
func (h *Handler) AdminAdvanceSubscriptions(w http.ResponseWriter, r *http.Request) {
	now := h.store.Clock.Now().Unix()
	var advanced []string

	subs := h.store.Subscriptions.Filter(func(_ string, s store.Subscription) bool {
		return s.Status == "trialing" || s.Status == "active"
	})

	for _, sub := range subs {
		changed := false

		// Trialing → Active: if past trial_end.
		if sub.Status == "trialing" && sub.TrialEnd > 0 && now >= sub.TrialEnd {
			sub.Status = "active"
			changed = true
			h.dispatcher.Enqueue("customer.subscription.updated", mapFromJSON(sub))
		}

		// Active + cancel_at_period_end + past period_end → Canceled.
		if sub.Status == "active" && sub.CancelAtPeriodEnd && now >= sub.CurrentPeriodEnd {
			sub.Status = "canceled"
			sub.CanceledAt = now
			changed = true
			h.dispatcher.Enqueue("customer.subscription.deleted", mapFromJSON(sub))
		}

		// Active + past period_end → Renew (advance period, create invoice).
		if sub.Status == "active" && !sub.CancelAtPeriodEnd && now >= sub.CurrentPeriodEnd {
			// Advance the period.
			periodDuration := sub.CurrentPeriodEnd - sub.CurrentPeriodStart
			if periodDuration <= 0 {
				periodDuration = 30 * 86400 // default 30 days
			}
			sub.CurrentPeriodStart = sub.CurrentPeriodEnd
			sub.CurrentPeriodEnd = sub.CurrentPeriodStart + periodDuration

			// Create renewal invoice.
			if sub.Items != nil {
				invoiceID := h.createSubscriptionInvoice(&sub, sub.Items.Data)
				sub.LatestInvoice = invoiceID
			}

			changed = true
			h.dispatcher.Enqueue("customer.subscription.updated", mapFromJSON(sub))
		}

		if changed {
			h.store.Subscriptions.Set(sub.ID, sub)
			advanced = append(advanced, sub.ID)
		}
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"advanced": advanced,
		"count":    len(advanced),
	})
}

// AdminAuthenticatePaymentIntent handles POST /admin/payment_intents/{id}/authenticate.
// Simulates a user completing 3D Secure authentication.
func (h *Handler) AdminAuthenticatePaymentIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pi, ok := h.store.PaymentIntents.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment_intent: "+id)
		return
	}
	if pi.Status != "requires_action" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "payment_intent_unexpected_state",
			"This PaymentIntent's status is "+pi.Status+". Only requires_action can be authenticated.")
		return
	}

	// Complete the payment — create charge, credit balance.
	chargeID := h.createChargeForPI(&pi)
	pi.LatestCharge = chargeID

	if pi.CaptureMethod == "manual" {
		pi.Status = "requires_capture"
	} else {
		pi.Status = "succeeded"
		pi.AmountReceived = pi.Amount
		h.store.CreditBalance("", pi.Currency, pi.Amount)
		h.store.RecordBalanceTransaction("charge", chargeID, pi.Currency, pi.Amount, 0)
	}

	h.store.PaymentIntents.Set(id, pi)
	if pi.Status == "succeeded" {
		h.emitEvent("payment_intent.succeeded", mapFromJSON(pi))
	} else {
		h.emitEvent("payment_intent.amount_capturable_updated", mapFromJSON(pi))
	}
	twincore.JSON(w, http.StatusOK, pi)
}

// AdminResolveDispute handles POST /admin/disputes/{id}/resolve.
// Resolves a dispute as won or lost. Won re-credits the balance.
func (h *Handler) AdminResolveDispute(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	dp, ok := h.store.Disputes.Get(id)
	if !ok {
		twincore.Error(w, http.StatusNotFound, "No such dispute: "+id)
		return
	}
	if dp.Status == "won" || dp.Status == "lost" {
		twincore.Error(w, http.StatusBadRequest, "Dispute already resolved: "+dp.Status)
		return
	}

	if err := parseFormOrJSON(r); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	outcome := r.FormValue("outcome")
	if outcome != "won" && outcome != "lost" {
		twincore.Error(w, http.StatusBadRequest, "outcome must be 'won' or 'lost'")
		return
	}

	dp.Status = outcome
	if outcome == "won" {
		// Re-credit the disputed amount.
		h.store.CreditBalance("", dp.Currency, dp.Amount)
		h.store.RecordBalanceTransaction("adjustment", id, dp.Currency, dp.Amount, 0)
	}

	h.store.Disputes.Set(id, dp)
	h.emitEvent("charge.dispute.closed", mapFromJSON(dp))
	twincore.JSON(w, http.StatusOK, dp)
}

// AdminCreateDispute handles POST /admin/disputes.
// Creates a dispute on a charge, debits the platform balance.
func (h *Handler) AdminCreateDispute(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	chargeID := r.FormValue("charge")
	if chargeID == "" {
		twincore.Error(w, http.StatusBadRequest, "Missing required param: charge")
		return
	}
	ch, ok := h.store.Charges.Get(chargeID)
	if !ok {
		twincore.Error(w, http.StatusNotFound, "No such charge: "+chargeID)
		return
	}

	reason := r.FormValue("reason")
	if reason == "" {
		reason = "fraudulent"
	}

	id := h.store.Disputes.NextID()
	dp := store.Dispute{
		ID:            id,
		Object:        "dispute",
		Amount:        ch.Amount,
		Currency:      ch.Currency,
		Charge:        chargeID,
		PaymentIntent: ch.PaymentIntent,
		Reason:        reason,
		Status:        "needs_response",
		Livemode:      false,
		Metadata:      parseMetadata(r),
		Created:       h.store.Now(),
	}

	// Mark charge as disputed.
	ch.Disputed = true
	h.store.Charges.Set(chargeID, ch)

	// Debit balance for the dispute amount.
	h.store.DebitBalance("", ch.Currency, ch.Amount)
	h.store.RecordBalanceTransaction("adjustment", id, ch.Currency, -ch.Amount, 0)

	h.store.Disputes.Set(id, dp)
	h.emitEvent("charge.dispute.created", mapFromJSON(dp))
	twincore.JSON(w, http.StatusOK, dp)
}
