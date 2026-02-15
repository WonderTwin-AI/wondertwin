package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// emitEvent creates a Stripe event and optionally enqueues a webhook.
func (h *Handler) emitEvent(eventType string, objectData map[string]any) {
	id := h.store.Events.NextID()
	evt := store.Event{
		ID:              id,
		Object:          "event",
		Type:            eventType,
		Data:            store.EventData{Object: objectData},
		APIVersion:      "2024-04-10",
		Created:         store.Now(),
		Livemode:        false,
		PendingWebhooks: 1,
	}
	h.store.Events.Set(id, evt)

	// Enqueue webhook delivery
	if h.dispatcher != nil {
		h.dispatcher.Enqueue(eventType, map[string]any{
			"id":               evt.ID,
			"object":           "event",
			"type":             evt.Type,
			"data":             evt.Data,
			"api_version":      evt.APIVersion,
			"created":          evt.Created,
			"livemode":         evt.Livemode,
			"pending_webhooks": evt.PendingWebhooks,
		})
	}
}

// GetEvent handles GET /v1/events/{id}.
func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	evt, ok := h.store.Events.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such event: '"+id+"'")
		return
	}

	twincore.JSON(w, http.StatusOK, evt)
}

// ListEvents handles GET /v1/events.
func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	eventType := r.URL.Query().Get("type")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	if eventType != "" {
		// Filter by event type
		filtered := h.store.Events.Filter(func(id string, evt store.Event) bool {
			return evt.Type == eventType
		})
		data := filtered
		if len(data) > limit {
			data = data[:limit]
		}
		twincore.JSON(w, http.StatusOK, map[string]any{
			"object":   "list",
			"url":      "/v1/events",
			"data":     data,
			"has_more": len(filtered) > limit,
		})
		return
	}

	page := h.store.Events.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/events",
		"data":     page.Data,
		"has_more": page.HasMore,
	})
}
