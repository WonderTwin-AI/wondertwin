package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/store"
)

// captureRequest represents a PostHog capture request body.
type captureRequest struct {
	APIKey     string         `json:"api_key"`
	Event      string         `json:"event"`
	DistinctID string         `json:"distinct_id"`
	Properties map[string]any `json:"properties,omitempty"`
	Timestamp  string         `json:"timestamp,omitempty"`
}

// batchRequest represents a PostHog batch capture request body.
type batchRequest struct {
	APIKey string           `json:"api_key"`
	Batch  []captureRequest `json:"batch"`
}

// decideRequest represents a PostHog /decide request body.
type decideRequest struct {
	APIKey     string `json:"api_key"`
	DistinctID string `json:"distinct_id"`
	Token      string `json:"token"`
}

// CaptureEvent handles POST /capture, POST /e
func (h *Handler) CaptureEvent(w http.ResponseWriter, r *http.Request) {
	var req captureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"status": 0,
			"error":  "Invalid request body: " + err.Error(),
		})
		return
	}

	// Check for API key in body or header
	apiKey := req.APIKey
	if apiKey == "" {
		apiKey = apiKeyFromRequest(r)
	}
	// Also check properties.$token (JS SDK)
	if apiKey == "" && req.Properties != nil {
		if token, ok := req.Properties["$token"].(string); ok {
			apiKey = token
		}
	}

	if req.Event == "" {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"status": 0,
			"error":  "event field is required",
		})
		return
	}

	h.storeEvent(req)

	twincore.JSON(w, http.StatusOK, map[string]any{
		"status": 1,
	})
}

// BatchCapture handles POST /batch
func (h *Handler) BatchCapture(w http.ResponseWriter, r *http.Request) {
	var req batchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"status": 0,
			"error":  "Invalid request body: " + err.Error(),
		})
		return
	}

	for _, event := range req.Batch {
		if event.APIKey == "" {
			event.APIKey = req.APIKey
		}
		h.storeEvent(event)
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"status": 1,
	})
}

// Decide handles POST /decide/?v=3 (feature flag evaluation)
func (h *Handler) Decide(w http.ResponseWriter, r *http.Request) {
	var req decideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"status": 0,
			"error":  "Invalid request body: " + err.Error(),
		})
		return
	}

	flags := h.store.GetFeatureFlags()

	// Build feature flag response
	featureFlags := make(map[string]any, len(flags))
	featureFlagPayloads := make(map[string]any, len(flags))

	for key, flag := range flags {
		if flag.Enabled {
			if flag.Variant != "" {
				featureFlags[key] = flag.Variant
			} else {
				featureFlags[key] = true
			}
		} else {
			featureFlags[key] = false
		}
		featureFlagPayloads[key] = nil
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"featureFlags":        featureFlags,
		"featureFlagPayloads": featureFlagPayloads,
		"errorsWhileComputingFlags": false,
	})
}

// storeEvent saves a captured event to the store.
func (h *Handler) storeEvent(req captureRequest) {
	now := h.store.Clock.Now()

	ts := req.Timestamp
	if ts == "" {
		ts = now.Format(time.RFC3339)
	}

	id := h.store.Events.NextID()

	evt := store.CapturedEvent{
		UUID:       id,
		Event:      req.Event,
		DistinctID: req.DistinctID,
		Properties: req.Properties,
		Timestamp:  ts,
	}

	h.store.Events.Set(id, evt)
}

// AdminListEvents handles GET /admin/events
// Supports ?event={name} and ?distinct_id={id} query parameters.
func (h *Handler) AdminListEvents(w http.ResponseWriter, r *http.Request) {
	eventFilter := r.URL.Query().Get("event")
	distinctIDFilter := r.URL.Query().Get("distinct_id")

	events := h.store.Events.List()

	if eventFilter != "" || distinctIDFilter != "" {
		var filtered []store.CapturedEvent
		for _, evt := range events {
			if eventFilter != "" && evt.Event != eventFilter {
				continue
			}
			if distinctIDFilter != "" && evt.DistinctID != distinctIDFilter {
				continue
			}
			filtered = append(filtered, evt)
		}
		events = filtered
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"events": events,
		"total":  len(events),
	})
}

// AdminSetFeatureFlags handles POST /admin/feature-flags
func (h *Handler) AdminSetFeatureFlags(w http.ResponseWriter, r *http.Request) {
	var flags []store.FeatureFlag
	if err := json.NewDecoder(r.Body).Decode(&flags); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	h.store.SetFeatureFlags(flags)

	twincore.JSON(w, http.StatusOK, map[string]any{
		"status": "set",
		"flags":  h.store.GetFeatureFlags(),
	})
}

// AdminGetFeatureFlags handles GET /admin/feature-flags
func (h *Handler) AdminGetFeatureFlags(w http.ResponseWriter, r *http.Request) {
	twincore.JSON(w, http.StatusOK, h.store.GetFeatureFlags())
}
