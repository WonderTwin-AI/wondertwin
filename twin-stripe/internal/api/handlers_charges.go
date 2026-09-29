package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// CreateCharge handles POST /v1/charges (direct charge creation).
func (h *Handler) CreateCharge(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	amountStr := r.FormValue("amount")
	currency := r.FormValue("currency")
	if amountStr == "" || currency == "" {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", missingParamMessage(r, "amount", "currency"))
		return
	}
	amount, _ := strconv.ParseInt(amountStr, 10, 64)

	// Check card behavior if source is a payment method.
	if source := r.FormValue("source"); source != "" {
		if behavior := h.checkCardBehavior(source); !behavior.Succeed && behavior.Code != "" {
			chargeID := h.store.StripeID(h.store.Charges.NextID())
			ch := store.Charge{
				ID: chargeID, Object: "charge", Amount: amount, Currency: currency,
				Customer: r.FormValue("customer"), PaymentMethod: source, Status: "failed",
				FailureCode: behavior.Code, FailureMessage: behavior.Message,
				Metadata: parseMetadata(r), Created: h.store.Now(),
			}
			h.store.Charges.Set(chargeID, ch)
			h.emitEvent("charge.failed", mapFromJSON(ch))
			writeError(w, http.StatusPaymentRequired, apiError{
				Type: "card_error", Code: behavior.Code, DeclineCode: behavior.DeclineCode,
				Message: behavior.Message, Charge: chargeID,
			})
			return
		}
	}

	id := h.store.StripeID(h.store.Charges.NextID())
	ch := store.Charge{
		ID:            id,
		Object:        "charge",
		Amount:        amount,
		Currency:      currency,
		Customer:      r.FormValue("customer"),
		Description:   r.FormValue("description"),
		PaymentMethod: r.FormValue("source"),
		Status:        "succeeded",
		Captured:      true,
		Paid:          true,
		Livemode:      false,
		Metadata:      parseMetadata(r),
		Created:       h.store.Now(),
	}

	h.store.Charges.Set(id, ch)
	h.store.CreditBalance("", currency, amount)
	h.store.RecordBalanceTransaction("charge", id, currency, amount, 0)
	h.emitEvent("charge.succeeded", mapFromJSON(ch))
	twincore.JSON(w, http.StatusOK, ch)
}

// UpdateCharge handles POST /v1/charges/{id}.
func (h *Handler) UpdateCharge(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ch, ok := h.store.Charges.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such charge: "+id)
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if v := r.FormValue("description"); v != "" {
		ch.Description = v
	}
	if v := r.FormValue("customer"); v != "" {
		ch.Customer = v
	}
	if meta := parseMetadata(r); len(meta) > 0 {
		ch.Metadata = meta
	}

	h.store.Charges.Set(id, ch)
	h.emitEvent("charge.updated", mapFromJSON(ch))
	twincore.JSON(w, http.StatusOK, ch)
}

func (h *Handler) GetCharge(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ch, ok := h.store.Charges.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such charge: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, ch)
}

func (h *Handler) ListCharges(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r, 10)
	customerFilter := r.URL.Query().Get("customer")
	piFilter := r.URL.Query().Get("payment_intent")
	page, ok := paginate(w, r, h.store.Charges, "charge", limit, func(ch store.Charge) bool {
		return (customerFilter == "" || ch.Customer == customerFilter) &&
			(piFilter == "" || ch.PaymentIntent == piFilter)
	})
	if !ok {
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/charges",
		"has_more": page.HasMore,
		"data":     page.Data,
	})
}
