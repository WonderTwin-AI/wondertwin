package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
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
