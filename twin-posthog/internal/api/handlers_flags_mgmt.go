package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// ListFeatureFlags handles GET /api/projects/{project_id}/feature_flags/
func (h *Handler) ListFeatureFlags(w http.ResponseWriter, r *http.Request) {
	flags := h.store.GetFeatureFlags()

	results := make([]map[string]any, 0, len(flags))
	for _, flag := range flags {
		results = append(results, flagToAPI(flag))
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"count":    len(results),
		"next":     nil,
		"previous": nil,
		"results":  results,
	})
}

// GetFeatureFlag handles GET /api/projects/{project_id}/feature_flags/{id}/
func (h *Handler) GetFeatureFlag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "flag_id")
	flags := h.store.GetFeatureFlags()

	// Find by key or numeric ID
	for _, flag := range flags {
		if flag.Key == id {
			twincore.JSON(w, http.StatusOK, flagToAPI(flag))
			return
		}
	}

	twincore.JSON(w, http.StatusNotFound, map[string]any{
		"type":   "invalid_request",
		"code":   "not_found",
		"detail": "Feature flag not found.",
	})
}

// CreateFeatureFlag handles POST /api/projects/{project_id}/feature_flags/
func (h *Handler) CreateFeatureFlag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key                string `json:"key"`
		Name               string `json:"name"`
		Active             *bool  `json:"active"`
		RolloutPercentage  *int   `json:"rollout_percentage"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"type":   "validation_error",
			"code":   "required",
			"detail": "key is required",
		})
		return
	}

	enabled := true
	if req.Active != nil {
		enabled = *req.Active
	}

	flag := store.FeatureFlag{
		Key:     req.Key,
		Enabled: enabled,
	}

	h.store.SetFeatureFlag(flag)
	twincore.JSON(w, http.StatusCreated, flagToAPI(flag))
}

// UpdateFeatureFlag handles PATCH /api/projects/{project_id}/feature_flags/{id}/
func (h *Handler) UpdateFeatureFlag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "flag_id")
	flags := h.store.GetFeatureFlags()

	flag, found := flags[id]
	if !found {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"type":   "invalid_request",
			"code":   "not_found",
			"detail": "Feature flag not found.",
		})
		return
	}

	var req struct {
		Active  *bool  `json:"active,omitempty"`
		Name    string `json:"name,omitempty"`
		Variant string `json:"variant,omitempty"`
		Payload string `json:"payload,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
		if req.Active != nil {
			flag.Enabled = *req.Active
		}
		if req.Variant != "" {
			flag.Variant = req.Variant
		}
		if req.Payload != "" {
			flag.Payload = req.Payload
		}
	}

	h.store.SetFeatureFlag(flag)
	twincore.JSON(w, http.StatusOK, flagToAPI(flag))
}

// DeleteFeatureFlag handles DELETE /api/projects/{project_id}/feature_flags/{id}/
func (h *Handler) DeleteFeatureFlag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "flag_id")
	flags := h.store.GetFeatureFlags()

	if _, found := flags[id]; !found {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"type":   "invalid_request",
			"code":   "not_found",
			"detail": "Feature flag not found.",
		})
		return
	}

	h.store.DeleteFeatureFlag(id)
	w.WriteHeader(http.StatusNoContent)
}

// ListEarlyAccessFeatures handles GET /api/early_access_features/
func (h *Handler) ListEarlyAccessFeatures(w http.ResponseWriter, r *http.Request) {
	// Early access features are flags with a specific convention
	// Return empty list for now — features are managed through flags
	twincore.JSON(w, http.StatusOK, map[string]any{
		"earlyAccessFeatures": []any{},
	})
}

// flagToAPI converts a FeatureFlag to the PostHog API response format.
func flagToAPI(flag store.FeatureFlag) map[string]any {
	result := map[string]any{
		"id":                   flag.Key, // use key as ID in sim
		"key":                  flag.Key,
		"name":                 flag.Key,
		"active":               flag.Enabled,
		"is_simple_flag":       flag.Variant == "",
		"rollout_percentage":   100,
		"ensure_experience_continuity": false,
		"filters": map[string]any{
			"groups": []map[string]any{
				{"rollout_percentage": 100},
			},
		},
	}
	if flag.Variant != "" {
		result["filters"].(map[string]any)["multivariate"] = map[string]any{
			"variants": []map[string]any{
				{"key": flag.Variant, "rollout_percentage": 100},
			},
		}
	}
	return result
}

// ensure imports are used
var _ = fmt.Sprintf
var _ = strconv.Itoa
