package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func (h *Handler) CreatePaymentIntent(w http.ResponseWriter, r *http.Request) {
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

	id := h.store.StripeID(h.store.PaymentIntents.NextID())
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
		pm = h.resolvePaymentMethod(pm)
		pi.PaymentMethod = pm
		pi.Status = "requires_confirmation"
	}
	if confirm == "true" && pm != "" {
		// Check card behavior for test cards.
		behavior := h.checkCardBehavior(pm)
		if !behavior.Succeed && behavior.Code != "" {
			h.store.PaymentIntents.Set(id, pi)
			h.emitEvent("payment_intent.created", mapFromJSON(pi))
			h.declinePayment(w, &pi, behavior)
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

	twincore.JSON(w, http.StatusOK, pi)
}

// UpdatePaymentIntent handles POST /v1/payment_intents/{id}.
func (h *Handler) UpdatePaymentIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pi, ok := h.store.PaymentIntents.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment_intent: "+id)
		return
	}
	if pi.Status == "succeeded" || pi.Status == "canceled" {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "payment_intent_unexpected_state",
			"This PaymentIntent's status is "+pi.Status+", which is not updatable.")
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
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
		pi.PaymentMethod = h.resolvePaymentMethod(v)
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
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment_intent: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, pi)
}

func (h *Handler) ConfirmPaymentIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pi, ok := h.store.PaymentIntents.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment_intent: "+id)
		return
	}
	if pi.Status != "requires_payment_method" && pi.Status != "requires_confirmation" {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "payment_intent_unexpected_state",
			"This PaymentIntent's status is "+pi.Status+", which is not confirmable.")
		return
	}

	if err := parseFormOrJSON(r); err == nil {
		if pm := r.FormValue("payment_method"); pm != "" {
			pi.PaymentMethod = h.resolvePaymentMethod(pm)
		}
	}
	if pi.PaymentMethod == "" {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "payment_intent_unexpected_state",
			"You cannot confirm this PaymentIntent because it's missing a payment method. To confirm the PaymentIntent with "+pi.ID+", specify a payment method attached to this customer along with the customer ID.")
		return
	}

	// Check card behavior for test cards.
	behavior := h.checkCardBehavior(pi.PaymentMethod)
	if !behavior.Succeed && behavior.Code != "" {
		h.declinePayment(w, &pi, behavior)
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
	twincore.JSON(w, http.StatusOK, pi)
}

// CapturePaymentIntent handles POST /v1/payment_intents/{id}/capture.
func (h *Handler) CapturePaymentIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pi, ok := h.store.PaymentIntents.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment_intent: "+id)
		return
	}
	if pi.Status != "requires_capture" {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "payment_intent_unexpected_state",
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
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "amount_too_large",
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
	twincore.JSON(w, http.StatusOK, pi)
}

func (h *Handler) CancelPaymentIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pi, ok := h.store.PaymentIntents.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment_intent: "+id)
		return
	}
	if pi.Status == "succeeded" || pi.Status == "canceled" {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "payment_intent_unexpected_state",
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
	twincore.JSON(w, http.StatusOK, pi)
}

func (h *Handler) ListPaymentIntents(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r, 10)
	customerFilter := r.URL.Query().Get("customer")
	page, ok := paginate(w, r, h.store.PaymentIntents, "payment_intent", limit, func(pi store.PaymentIntent) bool {
		return customerFilter == "" || pi.Customer == customerFilter
	})
	if !ok {
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/payment_intents",
		"has_more": page.HasMore,
		"data":     page.Data,
	})
}

func (h *Handler) createChargeForPI(pi *store.PaymentIntent) string {
	id := h.store.StripeID(h.store.Charges.NextID())
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

// declinePayment records a declined confirmation the way Stripe does: a
// failed charge, the intent back to requires_payment_method with the decline
// as last_payment_error, and a 402 card_error carrying the charge, the intent
// and the payment method.
func (h *Handler) declinePayment(w http.ResponseWriter, pi *store.PaymentIntent, b cardBehavior) {
	var pmObj *store.PaymentMethod
	if pm, ok := h.store.PaymentMethods.Get(pi.PaymentMethod); ok {
		pmObj = &pm
	}

	chargeID := h.store.StripeID(h.store.Charges.NextID())
	ch := store.Charge{
		ID:             chargeID,
		Object:         "charge",
		Amount:         pi.Amount,
		Currency:       pi.Currency,
		Customer:       pi.Customer,
		Description:    pi.Description,
		PaymentIntent:  pi.ID,
		PaymentMethod:  pi.PaymentMethod,
		Status:         "failed",
		FailureCode:    b.Code,
		FailureMessage: b.Message,
		Metadata:       pi.Metadata,
		Created:        h.store.Now(),
	}
	h.store.Charges.Set(chargeID, ch)
	h.emitEvent("charge.failed", mapFromJSON(ch))

	docURL := ""
	if _, ok := documentedErrorCodes[b.Code]; ok {
		docURL = errorDocURL(b.Code)
	}
	pi.Status = "requires_payment_method"
	pi.LatestCharge = chargeID
	pi.PaymentMethod = ""
	pi.LastPaymentError = &store.PaymentError{
		Type:          "card_error",
		Code:          b.Code,
		DeclineCode:   b.DeclineCode,
		Message:       b.Message,
		DocURL:        docURL,
		Charge:        chargeID,
		PaymentMethod: pmObj,
	}
	h.store.PaymentIntents.Set(pi.ID, *pi)
	h.emitEvent("payment_intent.payment_failed", mapFromJSON(pi))

	writeError(w, http.StatusPaymentRequired, apiError{
		Type:          "card_error",
		Code:          b.Code,
		DeclineCode:   b.DeclineCode,
		Message:       b.Message,
		DocURL:        docURL,
		Charge:        chargeID,
		PaymentIntent: pi,
		PaymentMethod: pmObj,
	})
}
