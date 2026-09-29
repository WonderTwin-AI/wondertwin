package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// emitEvent creates a Stripe event and optionally enqueues a webhook.
func (h *Handler) emitEvent(eventType string, objectData map[string]any) {
	id := h.store.Events.NextID()
	evt := store.Event{
		ID:              id,
		Object:          "event",
		Type:            eventType,
		Data:            store.EventData{Object: objectData},
		APIVersion:      h.DefaultVersion(),
		Created:         h.store.Now(),
		Livemode:        false,
		PendingWebhooks: 1,
	}
	h.store.Events.Set(id, evt)

	// Deliver to matching webhook endpoints
	if h.dispatcher != nil {
		h.dispatcher.Publish(mapFromJSON(evt))
	}
}

// GetEvent handles GET /v1/events/{id}.
func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	evt, ok := h.store.Events.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such event: '"+id+"'")
		return
	}

	twincore.JSON(w, http.StatusOK, evt)
}

// ListEvents handles GET /v1/events.
func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	eventType := r.URL.Query().Get("type")
	_ = r.ParseForm()
	types := r.Form["types[]"]
	limit := parseLimit(r, 10)
	page := paginate(r, h.store.Events, limit, func(evt store.Event) bool {
		if eventType != "" && !eventTypeMatches(eventType, evt.Type) {
			return false
		}
		if len(types) > 0 {
			for _, t := range types {
				if t == evt.Type {
					return true
				}
			}
			return false
		}
		return true
	})
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/events",
		"data":     page.Data,
		"has_more": page.HasMore,
	})
}

// eventTypeMatches reports whether an event type matches a type filter,
// which may end in a wildcard ("customer.*").
func eventTypeMatches(filter, eventType string) bool {
	if strings.HasSuffix(filter, "*") {
		return strings.HasPrefix(eventType, strings.TrimSuffix(filter, "*"))
	}
	return filter == eventType
}
