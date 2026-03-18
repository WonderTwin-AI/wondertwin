package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

func (h *Handler) CreateRefund(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	// Refund can reference a charge or payment_intent.
	chargeID := r.FormValue("charge")
	piID := r.FormValue("payment_intent")

	var ch store.Charge
	var found bool

	if chargeID != "" {
		ch, found = h.store.Charges.Get(chargeID)
	} else if piID != "" {
		pi, ok := h.store.PaymentIntents.Get(piID)
		if !ok {
			twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "resource_missing", "No such payment_intent: "+piID)
			return
		}
		if pi.LatestCharge != "" {
			ch, found = h.store.Charges.Get(pi.LatestCharge)
			chargeID = pi.LatestCharge
		}
	}

	if !found {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Must provide charge or payment_intent.")
		return
	}

	amount := ch.Amount - ch.AmountRefunded
	if v := r.FormValue("amount"); v != "" {
		if a, err := strconv.ParseInt(v, 10, 64); err == nil {
			amount = a
		}
	}

	if amount > ch.Amount-ch.AmountRefunded {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "amount_too_large",
			"Refund amount is greater than the unrefunded amount.")
		return
	}

	id := h.store.Refunds.NextID()
	ref := store.Refund{
		ID:            id,
		Object:        "refund",
		Amount:        amount,
		Currency:      ch.Currency,
		Charge:        chargeID,
		PaymentIntent: ch.PaymentIntent,
		Reason:        r.FormValue("reason"),
		Status:        "succeeded",
		Metadata:      parseMetadata(r),
		Created:       store.Now(),
	}

	// Update charge refund tracking.
	ch.AmountRefunded += amount
	if ch.AmountRefunded >= ch.Amount {
		ch.Refunded = true
	}
	h.store.Charges.Set(chargeID, ch)

	// Debit the platform balance.
	h.store.DebitBalance("", ch.Currency, amount)
	h.store.RecordBalanceTransaction("refund", id, ch.Currency, -amount, 0)

	h.store.Refunds.Set(id, ref)
	h.dispatcher.Enqueue("charge.refunded", mapFromJSON(ch))
	twincore.JSON(w, http.StatusOK, ref)
}

func (h *Handler) GetRefund(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ref, ok := h.store.Refunds.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such refund: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, ref)
}

func (h *Handler) ListRefunds(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.Refunds.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/refunds",
		"has_more": page.HasMore,
		"data":     page.Data,
	})
}
