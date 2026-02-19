package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// Payout transition timing (in simulated seconds from creation).
const (
	payoutInTransitDelay = 3600     // 1 hour: pending → in_transit
	payoutPaidDelay      = 3600 * 2 // 2 hours: pending → paid (via in_transit)
)

// CreatePayout handles POST /v1/payouts.
func (h *Handler) CreatePayout(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", err.Error())
		return
	}

	amountStr := r.FormValue("amount")
	if amountStr == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing",
			"Missing required param: amount.")
		return
	}
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_invalid",
			"Invalid integer: "+amountStr)
		return
	}

	currency := r.FormValue("currency")
	if currency == "" {
		currency = "usd"
	}

	// Get account from Stripe-Account header
	accountID := stripeAccountFromRequest(r)

	id := h.store.Payouts.NextID()
	now := h.store.Clock.Now().Unix()

	payout := store.Payout{
		ID:          id,
		Object:      "payout",
		Amount:      amount,
		Currency:    currency,
		Description: r.FormValue("description"),
		Method:      "standard",
		Status:      store.PayoutStatusPending,
		Type:        "bank_account",
		Metadata:    extractMetadata(r),
		ArrivalDate: now + 86400*2, // +2 days
		Created:     now,
	}

	if method := r.FormValue("method"); method != "" {
		payout.Method = method
	}

	// Debit balance if account specified
	if accountID != "" {
		if err := h.store.DebitBalance(accountID, currency, amount); err != nil {
			twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "balance_insufficient",
				"You have insufficient funds in your Stripe account.")
			return
		}
	}

	h.store.Payouts.Set(id, payout)

	// Record balance transaction for payout
	h.store.RecordBalanceTransaction("payout", payout.ID, payout.Currency, -payout.Amount, 0)

	// Emit payout.created webhook
	h.emitEvent("payout.created", payoutToMap(payout))

	twincore.JSON(w, http.StatusOK, payout)
}

// GetPayout handles GET /v1/payouts/{id}.
// Checks simulated clock to advance payout status before returning.
func (h *Handler) GetPayout(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	payout, ok := h.store.Payouts.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such payout: '"+id+"'")
		return
	}

	// Advance state based on simulated clock
	h.advancePayoutState(&payout)
	h.store.Payouts.Set(id, payout)

	twincore.JSON(w, http.StatusOK, payout)
}

// ListPayouts handles GET /v1/payouts.
func (h *Handler) ListPayouts(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	// Advance all payout states before listing
	h.advanceAllPayoutStates()

	page := h.store.Payouts.Paginate(cursor, limit)

	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/payouts",
		"data":     page.Data,
		"has_more": page.HasMore,
	})
}

// AdminFailPayout handles POST /admin/payouts/{id}/fail
// Forces a payout to the failed state.
func (h *Handler) AdminFailPayout(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	payout, ok := h.store.Payouts.Get(id)
	if !ok {
		twincore.Error(w, http.StatusNotFound, "No such payout: "+id)
		return
	}

	if payout.Status == store.PayoutStatusPaid {
		twincore.Error(w, http.StatusBadRequest, "Payout already paid, cannot fail")
		return
	}

	payout.Status = store.PayoutStatusFailed
	payout.FailureCode = "could_not_process"
	payout.FailureMessage = "The bank could not process this payout."
	h.store.Payouts.Set(id, payout)

	h.emitEvent("payout.failed", payoutToMap(payout))

	twincore.JSON(w, http.StatusOK, payout)
}

// advancePayoutState checks the simulated clock and transitions a single payout.
func (h *Handler) advancePayoutState(p *store.Payout) {
	now := h.store.Clock.Now().Unix()
	elapsed := now - p.Created

	switch p.Status {
	case store.PayoutStatusPending:
		if elapsed >= payoutPaidDelay {
			p.Status = store.PayoutStatusPaid
			h.emitEvent("payout.paid", payoutToMap(*p))
		} else if elapsed >= payoutInTransitDelay {
			p.Status = store.PayoutStatusInTransit
			h.emitEvent("payout.updated", payoutToMap(*p))
		}
	case store.PayoutStatusInTransit:
		if elapsed >= payoutPaidDelay {
			p.Status = store.PayoutStatusPaid
			h.emitEvent("payout.paid", payoutToMap(*p))
		}
	}
}

// advanceAllPayoutStates processes all payouts in the store.
func (h *Handler) advanceAllPayoutStates() {
	ids := h.store.Payouts.ListIDs()
	for _, id := range ids {
		p, ok := h.store.Payouts.Get(id)
		if !ok {
			continue
		}
		oldStatus := p.Status
		h.advancePayoutState(&p)
		if p.Status != oldStatus {
			h.store.Payouts.Set(id, p)
		}
	}
}

func payoutToMap(p store.Payout) map[string]any {
	data, _ := json.Marshal(p)
	var m map[string]any
	json.Unmarshal(data, &m)
	return m
}
