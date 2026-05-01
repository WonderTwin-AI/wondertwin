package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// ListAPIKeys handles GET /api-keys
func (h *Handler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys := h.store.APIKeys.Filter(func(_ string, _ store.APIKey) bool { return true })
	// Strip tokens from list response
	stripped := make([]map[string]any, len(keys))
	for i, k := range keys {
		stripped[i] = map[string]any{"id": k.ID, "name": k.Name, "created_at": k.CreatedAt}
	}
	twincore.JSON(w, http.StatusOK, map[string]any{"data": stripped})
}

// CreateAPIKey handles POST /api-keys
func (h *Handler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"statusCode": 400, "message": "name is required", "name": "validation_error",
		})
		return
	}

	id := h.resendID()
	token := "re_" + h.resendID() + "_" + h.resendID()
	key := store.APIKey{
		ID:        id,
		Name:      req.Name,
		Token:     token,
		CreatedAt: h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z"),
	}

	h.store.APIKeys.Set(id, key)
	// Token is only returned on create
	twincore.JSON(w, http.StatusCreated, map[string]any{
		"id": key.ID, "token": key.Token,
	})
}

// DeleteAPIKey handles DELETE /api-keys/{id}
func (h *Handler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := h.store.APIKeys.Get(id); !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "API key not found", "name": "not_found",
		})
		return
	}
	h.store.APIKeys.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}
