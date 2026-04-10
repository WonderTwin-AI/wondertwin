package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-increase/internal/store"
)

// GetEvent handles GET /events/{event_id}.
func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "event_id")
	event, ok := h.store.Events.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No event found for %s", id))
		return
	}
	twincore.JSON(w, http.StatusOK, event)
}

// ListEvents handles GET /events.
func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	cursor, limit := paginationParams(r)

	page := h.store.Events.Paginate(cursor, limit)

	data := make([]any, len(page.Data))
	for i, event := range page.Data {
		data[i] = event
	}

	resp := listResponse(data, page.Cursor)
	twincore.JSON(w, http.StatusOK, resp)
}

// CreateEventSubscription handles POST /event_subscriptions.
func (h *Handler) CreateEventSubscription(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL                     string   `json:"url"`
		SelectedEventCategories []string `json:"selected_event_category_ids,omitempty"`
		IdempotencyKey          string   `json:"idempotency_key,omitempty"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		increaseError(w, http.StatusBadRequest, "invalid_parameters", "Invalid JSON body")
		return
	}

	if req.URL == "" {
		increaseError(w, http.StatusBadRequest, "invalid_parameters", "url is required")
		return
	}

	id := h.store.EventSubscriptions.NextID()
	now := h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z")

	sub := store.EventSubscription{
		ID:                      id,
		Type:                    "event_subscription",
		URL:                     req.URL,
		Status:                  store.EventSubscriptionStatusActive,
		SelectedEventCategories: req.SelectedEventCategories,
		CreatedAt:               now,
		IdempotencyKey:          req.IdempotencyKey,
	}

	h.store.EventSubscriptions.Set(id, sub)
	h.emitEvent("event_subscription.created", id, "event_subscription")

	twincore.JSON(w, http.StatusOK, sub)
}

// GetEventSubscription handles GET /event_subscriptions/{event_subscription_id}.
func (h *Handler) GetEventSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "event_subscription_id")
	sub, ok := h.store.EventSubscriptions.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No event subscription found for %s", id))
		return
	}
	twincore.JSON(w, http.StatusOK, sub)
}

// UpdateEventSubscription handles PATCH /event_subscriptions/{event_subscription_id}.
func (h *Handler) UpdateEventSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "event_subscription_id")
	sub, ok := h.store.EventSubscriptions.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No event subscription found for %s", id))
		return
	}

	var req struct {
		Status string `json:"status,omitempty"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		increaseError(w, http.StatusBadRequest, "invalid_parameters", "Invalid JSON body")
		return
	}

	if req.Status != "" {
		sub.Status = req.Status
	}

	h.store.EventSubscriptions.Set(id, sub)
	h.emitEvent("event_subscription.updated", id, "event_subscription")

	twincore.JSON(w, http.StatusOK, sub)
}

// ListEventSubscriptions handles GET /event_subscriptions.
func (h *Handler) ListEventSubscriptions(w http.ResponseWriter, r *http.Request) {
	cursor, limit := paginationParams(r)

	page := h.store.EventSubscriptions.Paginate(cursor, limit)

	data := make([]any, len(page.Data))
	for i, sub := range page.Data {
		data[i] = sub
	}

	resp := listResponse(data, page.Cursor)
	twincore.JSON(w, http.StatusOK, resp)
}
