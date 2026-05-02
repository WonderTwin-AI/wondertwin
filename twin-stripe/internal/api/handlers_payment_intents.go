package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twinkit/workspace"
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
		ClientSecret:       id + "_secret_" + h.randomHex(12),
		Livemode:           false,
		Metadata:           parseMetadata(r),
		Created:            h.store.Now(),
	}

	// If payment_method provided and confirm=true, auto-succeed.
	pm := r.FormValue("payment_method")
	confirm := r.FormValue("confirm")
	if pm != "" {
		pi.PaymentMethod = pm
		pi.Status = "requires_confirmation"
	}
	if confirm == "true" && pm != "" {
		// Check card behavior for test cards.
		behavior := h.checkCardBehavior(pm)
		if !behavior.Succeed && behavior.DeclineCode != "" {
			twincore.StripeError(w, http.StatusPaymentRequired, "card_error", behavior.DeclineCode, behavior.Message)
			return
		}
		if behavior.RequiresAction {
			pi.Status = "requires_action"
		} else {
			// Create a charge.
			chargeID := h.createChargeForPI(&pi)
			pi.LatestCharge = chargeID

			if captureMethod == "manual" {
				pi.Status = "requires_capture"
			} else {
				pi.Status = "succeeded"
				pi.AmountReceived = amount
				h.store.CreditBalance("", currency, amount)
				h.store.RecordBalanceTransaction("charge", chargeID, currency, amount, 0)
			}
		}
	}

	h.store.PaymentIntents.Set(id, pi)
	h.emitEvent("payment_intent.created", mapFromJSON(pi))
	if pi.Status == "succeeded" {
		h.emitEvent("payment_intent.succeeded", mapFromJSON(pi))
	} else if pi.Status == "requires_capture" {
		h.emitEvent("payment_intent.amount_capturable_updated", mapFromJSON(pi))
	}

	// Track payment intent lifecycle through workspace engine for telemetry.
	if h.wsEngine != nil {
		h.wsEngine.CreateEntity(context.Background(), &workspace.Entity{
			ID:     id,
			Type:   "payment_intent",
			Status: pi.Status,
			Properties: map[string]any{
				"amount":         pi.Amount,
				"currency":       pi.Currency,
				"capture_method": pi.CaptureMethod,
			},
		})
	}

	twincore.JSON(w, http.StatusOK, pi)
}

// UpdatePaymentIntent handles POST /v1/payment_intents/{id}.
func (h *Handler) UpdatePaymentIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pi, ok := h.store.PaymentIntents.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment_intent: "+id)
		return
	}
	if pi.Status == "succeeded" || pi.Status == "canceled" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "payment_intent_unexpected_state",
			"This PaymentIntent's status is "+pi.Status+", which is not updatable.")
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if v := r.FormValue("amount"); v != "" {
		if a, err := strconv.ParseInt(v, 10, 64); err == nil {
			pi.Amount = a
		}
	}
	if v := r.FormValue("currency"); v != "" {
		pi.Currency = v
	}
	if v := r.FormValue("customer"); v != "" {
		pi.Customer = v
	}
	if v := r.FormValue("description"); v != "" {
		pi.Description = v
	}
	if v := r.FormValue("payment_method"); v != "" {
		pi.PaymentMethod = v
		if pi.Status == "requires_payment_method" {
			pi.Status = "requires_confirmation"
		}
	}
	if meta := parseMetadata(r); len(meta) > 0 {
		pi.Metadata = meta
	}

	h.store.PaymentIntents.Set(id, pi)
	h.emitEvent("payment_intent.updated", mapFromJSON(pi))
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

	// Check card behavior for test cards.
	behavior := h.checkCardBehavior(pi.PaymentMethod)
	if !behavior.Succeed && behavior.DeclineCode != "" {
		twincore.StripeError(w, http.StatusPaymentRequired, "card_error", behavior.DeclineCode, behavior.Message)
		return
	}
	if behavior.RequiresAction {
		pi.Status = "requires_action"
		h.store.PaymentIntents.Set(id, pi)
		h.emitEvent("payment_intent.requires_action", mapFromJSON(pi))
		twincore.JSON(w, http.StatusOK, pi)
		return
	}

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
	if h.wsEngine != nil {
		h.wsEngine.TransitionStatus(context.Background(), "payment_intent", id, pi.Status)
	}
	twincore.JSON(w, http.StatusOK, pi)
}

// CapturePaymentIntent handles POST /v1/payment_intents/{id}/capture.
func (h *Handler) CapturePaymentIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pi, ok := h.store.PaymentIntents.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment_intent: "+id)
		return
	}
	if pi.Status != "requires_capture" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "payment_intent_unexpected_state",
			"This PaymentIntent's status is "+pi.Status+". Only a PaymentIntent with status requires_capture can be captured.")
		return
	}

	captureAmount := pi.Amount
	if err := parseFormOrJSON(r); err == nil {
		if v := r.FormValue("amount_to_capture"); v != "" {
			if a, err := strconv.ParseInt(v, 10, 64); err == nil {
				captureAmount = a
			}
		}
	}
	if captureAmount > pi.Amount {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "amount_too_large",
			"Capture amount exceeds the authorized amount.")
		return
	}

	pi.Status = "succeeded"
	pi.AmountReceived = captureAmount

	// Update charge captured flag.
	if pi.LatestCharge != "" {
		ch, ok := h.store.Charges.Get(pi.LatestCharge)
		if ok {
			ch.Captured = true
			ch.Amount = captureAmount
			h.store.Charges.Set(pi.LatestCharge, ch)
		}
	}

	h.store.CreditBalance("", pi.Currency, captureAmount)
	h.store.RecordBalanceTransaction("charge", pi.LatestCharge, pi.Currency, captureAmount, 0)
	h.store.PaymentIntents.Set(id, pi)
	if pi.Status == "succeeded" {
		h.emitEvent("payment_intent.succeeded", mapFromJSON(pi))
	} else {
		h.emitEvent("payment_intent.amount_capturable_updated", mapFromJSON(pi))
	}
	if h.wsEngine != nil {
		h.wsEngine.TransitionStatus(context.Background(), "payment_intent", id, pi.Status)
	}
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
	pi.CanceledAt = h.store.Now()
	if err := parseFormOrJSON(r); err == nil {
		if reason := r.FormValue("cancellation_reason"); reason != "" {
			pi.CancellationReason = reason
		}
	}

	h.store.PaymentIntents.Set(id, pi)
	h.emitEvent("payment_intent.canceled", mapFromJSON(pi))
	if h.wsEngine != nil {
		h.wsEngine.TransitionStatus(context.Background(), "payment_intent", id, "canceled")
	}
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
		Created:       h.store.Now(),
	}
	h.store.Charges.Set(id, ch)
	h.emitEvent("charge.succeeded", mapFromJSON(ch))
	return id
}

// randomHex draws n random bytes from the deterministic per-twin
// Rand and returns them as a 2n-char hex string. Routing through
// store.RandHex (rather than crypto/rand) is the determinism
// contract: two seeded runs produce identical client_secret /
// webhook secret values.
func (h *Handler) randomHex(n int) string {
	return h.store.RandHex(n)
}
