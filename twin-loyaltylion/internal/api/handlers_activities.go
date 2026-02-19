package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-loyaltylion/internal/store"
)

// RecordActivity handles POST /v2/activities.
func (h *Handler) RecordActivity(w http.ResponseWriter, r *http.Request) {
	apiKey := getAPIKey(r)

	var req struct {
		Name       string            `json:"name"`
		MerchantID string            `json:"merchant_id"`
		Properties map[string]string `json:"properties,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Name == "" {
		twincore.Error(w, http.StatusUnprocessableEntity, "name is required")
		return
	}
	if req.MerchantID == "" {
		twincore.Error(w, http.StatusUnprocessableEntity, "merchant_id is required")
		return
	}

	now := h.store.Clock.Now().Format(time.RFC3339)
	id := h.store.NextActivityID()
	activity := store.Activity{
		ID:         id,
		Name:       req.Name,
		MerchantID: req.MerchantID,
		Properties: req.Properties,
		Timestamp:  now,
		APIKey:     apiKey,
	}

	h.store.Activities.Set(fmt.Sprintf("%d", id), activity)
	twincore.JSON(w, http.StatusCreated, activity)
}
