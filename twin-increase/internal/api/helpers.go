package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/wondertwin-ai/wondertwin/twin-increase/internal/store"
)

// parseJSONBody decodes JSON from request body into target.
func parseJSONBody(r *http.Request, target any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

// paginationParams extracts Increase-style pagination parameters.
// Increase uses cursor and limit query params.
func paginationParams(r *http.Request) (cursor string, limit int) {
	cursor = r.URL.Query().Get("cursor")
	limit = 100 // Increase default and max
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	return
}

// listResponse builds an Increase-style paginated list response.
func listResponse(data []any, nextCursor string) map[string]any {
	if data == nil {
		data = []any{}
	}
	result := map[string]any{
		"data": data,
	}
	if nextCursor != "" {
		result["cursor"] = nextCursor
	}
	return result
}

// emitEvent creates an event and dispatches it via webhook.
func (h *Handler) emitEvent(category string, associatedObjectID, associatedObjectType string) {
	id := h.store.Events.NextID()
	event := store.Event{
		ID:                   id,
		Type:                 "event",
		Category:             category,
		AssociatedObjectID:   associatedObjectID,
		AssociatedObjectType: associatedObjectType,
		CreatedAt:            h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z"),
	}
	h.store.Events.Set(id, event)

	// Dispatch to all active subscriptions
	subs := h.store.EventSubscriptions.Filter(func(id string, sub store.EventSubscription) bool {
		if sub.Status != store.EventSubscriptionStatusActive {
			return false
		}
		// If no categories selected, send all events
		if len(sub.SelectedEventCategories) == 0 {
			return true
		}
		// Otherwise check if category matches
		for _, cat := range sub.SelectedEventCategories {
			if cat == category {
				return true
			}
		}
		return false
	})

	if len(subs) > 0 {
		// Enqueue webhook event
		payload := map[string]any{
			"id":                     event.ID,
			"type":                   event.Type,
			"category":               event.Category,
			"associated_object_id":   event.AssociatedObjectID,
			"associated_object_type": event.AssociatedObjectType,
			"created_at":             event.CreatedAt,
		}

		h.dispatcher.Enqueue(event.Category, payload)

		// Flush webhooks immediately for simulation
		h.dispatcher.Flush()
	}
}

// signWebhook generates the Increase-Webhook-Signature header.
// Format: t=<timestamp>,v1=<signature>
// For simulation, we use a simple HMAC-SHA256 with a fixed secret.
func (h *Handler) signWebhook(payload []byte) string {
	// In a real implementation, this would use crypto/hmac with a webhook secret
	// For the twin, we'll return a simple format
	timestamp := h.store.Clock.Now().Unix()
	return fmt.Sprintf("t=%d,v1=simulated_signature", timestamp)
}
