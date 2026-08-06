package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/store"
	pkgevents "github.com/wondertwin-ai/wondertwin/twinkit/events"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
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
	body, err := readCaptureBody(r)
	if err != nil {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"status": 0,
			"error":  "Invalid request body: " + err.Error(),
		})
		return
	}
	var req captureRequest
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&req); err != nil {
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
	body, err := readCaptureBody(r)
	if err != nil {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"status": 0,
			"error":  "Invalid request body: " + err.Error(),
		})
		return
	}
	var req batchRequest
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&req); err != nil {
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
		if flag.Payload != "" {
			featureFlagPayloads[key] = flag.Payload
		} else {
			featureFlagPayloads[key] = nil
		}
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"featureFlags":              featureFlags,
		"featureFlagPayloads":       featureFlagPayloads,
		"errorsWhileComputingFlags": false,
		"requestId":                 h.store.Events.NextID(),
		"quotaLimited":              []string{},
	})
}

// storeEvent saves a captured event to the store and processes special event types.
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

	// Feed through events engine for lifecycle hooks + telemetry.
	if h.eventsEngine != nil {
		engineEvt := &pkgevents.Event{
			ID:         id,
			EventName:  req.Event,
			DistinctID: req.DistinctID,
			Properties: req.Properties,
		}
		if parsedTime, err := time.Parse(time.RFC3339, ts); err == nil {
			engineEvt.Timestamp = parsedTime
		}
		h.eventsEngine.Track(context.Background(), engineEvt)

		// For $identify events, also call Identify on the engine.
		if req.Event == "$identify" {
			setProps := make(map[string]any)
			if req.Properties != nil {
				if sp, ok := req.Properties["$set"].(map[string]any); ok {
					setProps = sp
				}
			}
			h.eventsEngine.Identify(context.Background(), &pkgevents.IdentifyCall{
				DistinctID: req.DistinctID,
				Properties: setProps,
			})
		}
	}

	// Process special event types (store-level processing for API responses).
	switch req.Event {
	case "$identify":
		h.processIdentify(req, ts)
	case "$create_alias":
		h.processAlias(req, ts)
	case "$groupidentify":
		h.processGroupIdentify(req, ts)
	}
}

// processIdentify upserts a Person from an $identify event.
func (h *Handler) processIdentify(req captureRequest, ts string) {
	props := make(map[string]any)

	// Merge $set properties
	if req.Properties != nil {
		if setProps, ok := req.Properties["$set"].(map[string]any); ok {
			for k, v := range setProps {
				props[k] = v
			}
		}
	}

	// Check if person already exists and merge
	existing := h.store.Persons.Filter(func(_ string, p store.Person) bool {
		return p.DistinctID == req.DistinctID
	})

	if len(existing) > 0 {
		// Merge with existing properties
		for k, v := range existing[0].Properties {
			if _, exists := props[k]; !exists {
				props[k] = v
			}
		}
		// Find and update the existing record
		ids, _ := h.store.Persons.FilterWithIDs(func(_ string, p store.Person) bool {
			return p.DistinctID == req.DistinctID
		})
		if len(ids) > 0 {
			h.store.Persons.Set(ids[0], store.Person{
				DistinctID: req.DistinctID,
				Properties: props,
				CreatedAt:  existing[0].CreatedAt,
			})
			return
		}
	}

	id := h.store.Persons.NextID()
	h.store.Persons.Set(id, store.Person{
		DistinctID: req.DistinctID,
		Properties: props,
		CreatedAt:  ts,
	})
}

// processAlias stores an AliasMapping from a $create_alias event.
func (h *Handler) processAlias(req captureRequest, ts string) {
	alias := ""
	if req.Properties != nil {
		if a, ok := req.Properties["alias"].(string); ok {
			alias = a
		}
	}
	if alias == "" {
		return
	}

	id := h.store.Aliases.NextID()
	h.store.Aliases.Set(id, store.AliasMapping{
		Alias:      alias,
		DistinctID: req.DistinctID,
		CreatedAt:  ts,
	})
}

// processGroupIdentify upserts a Group from a $groupidentify event.
func (h *Handler) processGroupIdentify(req captureRequest, ts string) {
	if req.Properties == nil {
		return
	}

	groupType, _ := req.Properties["$group_type"].(string)
	groupKey, _ := req.Properties["$group_key"].(string)
	if groupType == "" || groupKey == "" {
		return
	}

	props := make(map[string]any)
	if setProps, ok := req.Properties["$group_set"].(map[string]any); ok {
		for k, v := range setProps {
			props[k] = v
		}
	}

	// Check if group already exists
	ids, existing := h.store.Groups.FilterWithIDs(func(_ string, g store.Group) bool {
		return g.Type == groupType && g.Key == groupKey
	})

	if len(existing) > 0 {
		// Merge properties
		for k, v := range existing[0].Properties {
			if _, exists := props[k]; !exists {
				props[k] = v
			}
		}
		h.store.Groups.Set(ids[0], store.Group{
			Type:       groupType,
			Key:        groupKey,
			Properties: props,
			CreatedAt:  existing[0].CreatedAt,
		})
		return
	}

	id := h.store.Groups.NextID()
	h.store.Groups.Set(id, store.Group{
		Type:       groupType,
		Key:        groupKey,
		Properties: props,
		CreatedAt:  ts,
	})
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

// AdminListPersons handles GET /admin/persons
// Supports ?distinct_id={id} query parameter.
func (h *Handler) AdminListPersons(w http.ResponseWriter, r *http.Request) {
	distinctIDFilter := r.URL.Query().Get("distinct_id")

	persons := h.store.Persons.List()

	if distinctIDFilter != "" {
		var filtered []store.Person
		for _, p := range persons {
			if p.DistinctID == distinctIDFilter {
				filtered = append(filtered, p)
			}
		}
		persons = filtered
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"persons": persons,
		"total":   len(persons),
	})
}

// AdminListAliases handles GET /admin/aliases
func (h *Handler) AdminListAliases(w http.ResponseWriter, r *http.Request) {
	aliases := h.store.Aliases.List()

	twincore.JSON(w, http.StatusOK, map[string]any{
		"aliases": aliases,
		"total":   len(aliases),
	})
}

// AdminListGroups handles GET /admin/groups
// Supports ?type={groupType} query parameter.
func (h *Handler) AdminListGroups(w http.ResponseWriter, r *http.Request) {
	typeFilter := r.URL.Query().Get("type")

	groups := h.store.Groups.List()

	if typeFilter != "" {
		var filtered []store.Group
		for _, g := range groups {
			if g.Type == typeFilter {
				filtered = append(filtered, g)
			}
		}
		groups = filtered
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"groups": groups,
		"total":  len(groups),
	})
}
