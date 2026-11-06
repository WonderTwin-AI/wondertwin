package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// ListAPIKeys handles GET /api-keys. Tokens are never listed (quirk auth-005).
func (h *Handler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys := h.store.APIKeys.Filter(func(_ string, _ store.APIKey) bool { return true })
	stripped := make([]map[string]any, len(keys))
	for i, k := range keys {
		stripped[i] = map[string]any{"id": k.ID, "name": k.Name, "created_at": k.CreatedAt, "last_used_at": k.LastUsedAt}
	}
	twincore.JSON(w, http.StatusOK, listBody(stripped))
}

// CreateAPIKey handles POST /api-keys. It answers 201 {id, object, token};
// the token is returned only here (quirks auth-005, envelope-005).
func (h *Handler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Permission string `json:"permission"`
		DomainID   string `json:"domain_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"statusCode": 400, "message": "name is required", "name": "validation_error",
		})
		return
	}
	if req.Permission == "" {
		req.Permission = store.PermissionFullAccess
	}

	id := h.resendID()
	token := "re_" + h.resendID() + "_" + h.resendID()
	key := store.APIKey{
		ID:         id,
		Name:       req.Name,
		Token:      token,
		CreatedAt:  h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z"),
		Permission: req.Permission,
		DomainID:   req.DomainID,
	}

	h.store.APIKeys.Set(id, key)
	twincore.JSON(w, http.StatusCreated, map[string]any{
		"id": key.ID, "object": "api_key", "token": key.Token,
	})
}

// DeleteAPIKey handles DELETE /api-keys/{id}, answering 200
// {object, id, deleted} (quirk envelope-005).
func (h *Handler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := h.store.APIKeys.Get(id); !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "API key not found", "name": "not_found",
		})
		return
	}
	h.store.APIKeys.Delete(id)
	twincore.JSON(w, http.StatusOK, map[string]any{"object": "api_key", "id": id, "deleted": true})
}

// issuedKey returns the stored key whose token is token, if the emulator
// issued it.
func (h *Handler) issuedKey(token string) (store.APIKey, bool) {
	for _, k := range h.store.APIKeys.List() {
		if k.Token == token {
			return k, true
		}
	}
	return store.APIKey{}, false
}
