package api

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

func (h *Handler) CreatePaymentIntent(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	amountStr := r.FormValue("amount")
	currency := r.FormValue("currency")
	if amountStr == "" || currency == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required params: amount, currency.")
		return
	}
	amount, _ := strconv.ParseInt(amountStr, 10, 64)

	id := h.store.PaymentIntents.NextID()
	captureMethod := r.FormValue("capture_method")
	if captureMethod == "" {
		captureMethod = "automatic"
	}
	confirmMethod := r.FormValue("confirmation_method")
	if confirmMethod == "" {
		confirmMethod = "automatic"
	}

	pi := store.PaymentIntent{
		ID:                 id,
		Object:             "payment_intent",
		Amount:             amount,
		Currency:           currency,
		Customer:           r.FormValue("customer"),
		Description:        r.FormValue("description"),
		Status:             "requires_payment_method",
		CaptureMethod:      captureMethod,
		ConfirmationMethod: confirmMethod,
		ClientSecret:       id + "_secret_" + randomHex(12),
		Livemode:           false,
		Metadata:           parseMetadata(r),
		Created:            store.Now(),
	}

	// If payment_method provided and confirm=true, auto-succeed.
	pm := r.FormValue("payment_method")
	confirm := r.FormValue("confirm")
	if pm != "" {
		pi.PaymentMethod = pm
		pi.Status = "requires_confirmation"
	}
	if confirm == "true" && pm != "" {
		pi.Status = "succeeded"
		pi.AmountReceived = amount
		// Create a charge.
		chargeID := h.createChargeForPI(&pi)
		pi.LatestCharge = chargeID
		h.store.CreditBalance("", currency, amount)
		h.store.RecordBalanceTransaction("charge", chargeID, currency, amount, 0)
	}

	h.store.PaymentIntents.Set(id, pi)
	h.dispatcher.Enqueue("payment_intent.created", mapFromJSON(pi))
	if pi.Status == "succeeded" {
		h.dispatcher.Enqueue("payment_intent.succeeded", mapFromJSON(pi))
	}
	twincore.JSON(w, http.StatusOK, pi)
}

func (h *Handler) GetPaymentIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pi, ok := h.store.PaymentIntents.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment_intent: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, pi)
}

func (h *Handler) ConfirmPaymentIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pi, ok := h.store.PaymentIntents.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment_intent: "+id)
		return
	}
	if pi.Status != "requires_payment_method" && pi.Status != "requires_confirmation" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "payment_intent_unexpected_state",
			"This PaymentIntent's status is "+pi.Status+", which is not confirmable.")
		return
	}

	if err := parseFormOrJSON(r); err == nil {
		if pm := r.FormValue("payment_method"); pm != "" {
			pi.PaymentMethod = pm
		}
	}

	pi.Status = "succeeded"
	pi.AmountReceived = pi.Amount
	chargeID := h.createChargeForPI(&pi)
	pi.LatestCharge = chargeID
	h.store.CreditBalance("", pi.Currency, pi.Amount)
	h.store.RecordBalanceTransaction("charge", chargeID, pi.Currency, pi.Amount, 0)

	h.store.PaymentIntents.Set(id, pi)
	h.dispatcher.Enqueue("payment_intent.succeeded", mapFromJSON(pi))
	twincore.JSON(w, http.StatusOK, pi)
}

func (h *Handler) CancelPaymentIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pi, ok := h.store.PaymentIntents.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment_intent: "+id)
		return
	}
	if pi.Status == "succeeded" || pi.Status == "canceled" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "payment_intent_unexpected_state",
			"This PaymentIntent's status is "+pi.Status+", which is not cancelable.")
		return
	}

	pi.Status = "canceled"
	pi.CanceledAt = store.Now()
	if err := parseFormOrJSON(r); err == nil {
		if reason := r.FormValue("cancellation_reason"); reason != "" {
			pi.CancellationReason = reason
		}
	}

	h.store.PaymentIntents.Set(id, pi)
	h.dispatcher.Enqueue("payment_intent.canceled", mapFromJSON(pi))
	twincore.JSON(w, http.StatusOK, pi)
}

func (h *Handler) ListPaymentIntents(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.PaymentIntents.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/payment_intents",
		"has_more": page.HasMore,
		"data":     page.Data,
	})
}

func (h *Handler) createChargeForPI(pi *store.PaymentIntent) string {
	id := h.store.Charges.NextID()
	ch := store.Charge{
		ID:            id,
		Object:        "charge",
		Amount:        pi.Amount,
		Currency:      pi.Currency,
		Customer:      pi.Customer,
		Description:   pi.Description,
		PaymentIntent: pi.ID,
		PaymentMethod: pi.PaymentMethod,
		Status:        "succeeded",
		Captured:      pi.CaptureMethod == "automatic",
		Paid:          true,
		Livemode:      false,
		Metadata:      pi.Metadata,
		Created:       store.Now(),
	}
	h.store.Charges.Set(id, ch)
	h.dispatcher.Enqueue("charge.succeeded", mapFromJSON(ch))
	return id
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
