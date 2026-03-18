package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

func (h *Handler) CreateSetupIntent(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	id := h.store.SetupIntents.NextID()
	usage := r.FormValue("usage")
	if usage == "" {
		usage = "off_session"
	}

	si := store.SetupIntent{
		ID:            id,
		Object:        "setup_intent",
		Customer:      r.FormValue("customer"),
		PaymentMethod: r.FormValue("payment_method"),
		Status:        "requires_payment_method",
		ClientSecret:  id + "_secret_" + randomHex(12),
		Usage:         usage,
		Livemode:      false,
		Metadata:      parseMetadata(r),
		Created:       store.Now(),
	}

	if si.PaymentMethod != "" {
		si.Status = "requires_confirmation"
	}

	// If confirm=true and payment_method provided, auto-succeed.
	if r.FormValue("confirm") == "true" && si.PaymentMethod != "" {
		si.Status = "succeeded"
	}

	h.store.SetupIntents.Set(id, si)
	h.dispatcher.Enqueue("setup_intent.created", mapFromJSON(si))
	if si.Status == "succeeded" {
		h.dispatcher.Enqueue("setup_intent.succeeded", mapFromJSON(si))
	}
	twincore.JSON(w, http.StatusOK, si)
}

func (h *Handler) GetSetupIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	si, ok := h.store.SetupIntents.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such setup_intent: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, si)
}

func (h *Handler) ConfirmSetupIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	si, ok := h.store.SetupIntents.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such setup_intent: "+id)
		return
	}
	if si.Status != "requires_payment_method" && si.Status != "requires_confirmation" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "setup_intent_unexpected_state",
			"This SetupIntent's status is "+si.Status+", which is not confirmable.")
		return
	}

	if err := parseFormOrJSON(r); err == nil {
		if pm := r.FormValue("payment_method"); pm != "" {
			si.PaymentMethod = pm
		}
	}

	si.Status = "succeeded"
	h.store.SetupIntents.Set(id, si)
	h.dispatcher.Enqueue("setup_intent.succeeded", mapFromJSON(si))
	twincore.JSON(w, http.StatusOK, si)
}

func (h *Handler) CancelSetupIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	si, ok := h.store.SetupIntents.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such setup_intent: "+id)
		return
	}
	if si.Status == "succeeded" || si.Status == "canceled" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "setup_intent_unexpected_state",
			"This SetupIntent's status is "+si.Status+", which is not cancelable.")
		return
	}

	si.Status = "canceled"
	h.store.SetupIntents.Set(id, si)
	h.dispatcher.Enqueue("setup_intent.canceled", mapFromJSON(si))
	twincore.JSON(w, http.StatusOK, si)
}

func (h *Handler) ListSetupIntents(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.SetupIntents.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/setup_intents",
		"has_more": page.HasMore,
		"data":     page.Data,
	})
}
