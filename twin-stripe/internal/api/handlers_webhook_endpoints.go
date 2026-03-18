package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

func (h *Handler) CreateWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	url := r.FormValue("url")
	if url == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required param: url.")
		return
	}

	id := h.store.WebhookEndpoints.NextID()
	we := store.WebhookEndpoint{
		ID:            id,
		Object:        "webhook_endpoint",
		URL:           url,
		Status:        "enabled",
		EnabledEvents: r.Form["enabled_events[]"],
		Secret:        "whsec_" + randomHex(16),
		APIVersion:    r.FormValue("api_version"),
		Description:   r.FormValue("description"),
		Livemode:      false,
		Metadata:      parseMetadata(r),
		Created:       store.Now(),
	}
	if we.EnabledEvents == nil {
		we.EnabledEvents = []string{}
	}

	h.store.WebhookEndpoints.Set(id, we)
	twincore.JSON(w, http.StatusOK, we)
}

func (h *Handler) GetWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	we, ok := h.store.WebhookEndpoints.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such webhook_endpoint: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, we)
}

func (h *Handler) UpdateWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	we, ok := h.store.WebhookEndpoints.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such webhook_endpoint: "+id)
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if v := r.FormValue("url"); v != "" {
		we.URL = v
	}
	if v := r.FormValue("description"); v != "" {
		we.Description = v
	}
	if v := r.FormValue("disabled"); v == "true" {
		we.Status = "disabled"
	} else if v == "false" {
		we.Status = "enabled"
	}
	if events := r.Form["enabled_events[]"]; len(events) > 0 {
		we.EnabledEvents = events
	}
	if meta := parseMetadata(r); len(meta) > 0 {
		we.Metadata = meta
	}

	h.store.WebhookEndpoints.Set(id, we)
	twincore.JSON(w, http.StatusOK, we)
}

func (h *Handler) DeleteWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.WebhookEndpoints.Delete(id) {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such webhook_endpoint: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]any{"id": id, "object": "webhook_endpoint", "deleted": true})
}

func (h *Handler) ListWebhookEndpoints(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.WebhookEndpoints.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/webhook_endpoints", "has_more": page.HasMore, "data": page.Data,
	})
}
