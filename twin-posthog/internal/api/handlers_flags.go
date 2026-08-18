package api

import (
	"encoding/json"
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// Flags handles POST /flags/?v=2 (feature flag evaluation v2 path)
func (h *Handler) Flags(w http.ResponseWriter, r *http.Request) {
	// /flags is the v2 successor to /decide and PostHog gates it on the project
	// token identically. Decode leniently: unlike /decide the body is optional
	// here, so a key supplied by header or query param still satisfies the gate.
	var req decideRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	flagsKey := req.APIKey
	if flagsKey == "" {
		flagsKey = req.Token
	}
	if resolveAPIKey(r, flagsKey, nil) == "" {
		noKeyError(w)
		return
	}

	flags := h.store.GetFeatureFlags()

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

// LocalEvaluation handles GET /api/feature_flag/local_evaluation
// Returns flag definitions for local evaluation by the SDK.
func (h *Handler) LocalEvaluation(w http.ResponseWriter, r *http.Request) {
	// Check for personal API key in Authorization header
	auth := r.Header.Get("Authorization")
	if auth == "" {
		twincore.JSON(w, http.StatusUnauthorized, map[string]any{
			"type":   "authentication_error",
			"code":   "unauthorized",
			"detail": "Personal API key required. Pass as Authorization: Bearer <key>.",
		})
		return
	}

	flags := h.store.GetFeatureFlags()

	// Build flag definitions for local evaluation
	flagDefs := make([]map[string]any, 0, len(flags))
	for _, flag := range flags {
		def := map[string]any{
			"id":                 flag.Key,
			"key":                flag.Key,
			"name":               flag.Key,
			"active":             flag.Enabled,
			"is_simple_flag":     true,
			"rollout_percentage": 100,
			"filters": map[string]any{
				"groups": []map[string]any{
					{
						"rollout_percentage": 100,
					},
				},
			},
			"ensure_experience_continuity": false,
		}
		if flag.Variant != "" {
			def["filters"].(map[string]any)["multivariate"] = map[string]any{
				"variants": []map[string]any{
					{
						"key":                flag.Variant,
						"rollout_percentage": 100,
					},
				},
			}
		}
		flagDefs = append(flagDefs, def)
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"flags":              flagDefs,
		"group_type_mapping": map[string]any{},
	})
}
