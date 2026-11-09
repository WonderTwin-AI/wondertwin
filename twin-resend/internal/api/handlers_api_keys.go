package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// ListAPIKeys handles GET /api-keys, newest first; every key when limit is
// absent (quirk pagination-002). Tokens are never listed (quirk auth-005).
func (h *Handler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	pq, ok := readPageQuery(w, r)
	if !ok {
		return
	}
	keys := h.store.APIKeys.List()
	stripped := make([]map[string]any, len(keys))
	for i, k := range keys {
		stripped[i] = map[string]any{"id": k.ID, "name": k.Name, "created_at": k.CreatedAt, "last_used_at": k.LastUsedAt}
	}
	twincore.JSON(w, http.StatusOK, paginate(stripped, pq, 0, func(k map[string]any) string { return k["id"].(string) }))
}

// CreateAPIKey handles POST /api-keys. It answers 201 {id, object, token};
// the token is returned only here (quirks auth-005, envelope-005).
func (h *Handler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Permission string `json:"permission"`
		DomainID   string `json:"domain_id"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		missingField(w, "name")
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
		CreatedAt:  h.store.Clock.Now().UTC().Format(pgTimestamp),
		Permission: req.Permission,
		DomainID:   req.DomainID,
	}

	h.store.APIKeys.Set(id, key)
	twincore.JSON(w, http.StatusCreated, map[string]any{
		"id": key.ID, "object": "api_key", "token": key.Token,
	})
}

// UpdateAPIKey handles PATCH /api-keys/{id}, which renames a key and
// answers {object, id} (probes n02, n08 of 2026-11-08).
func (h *Handler) UpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	key, ok := h.store.APIKeys.Get(id)
	if !ok {
		apiKeyNotFound(w)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		missingField(w, "name")
		return
	}
	key.Name = req.Name
	h.store.APIKeys.Set(id, key)
	twincore.JSON(w, http.StatusOK, map[string]any{"object": "api_key", "id": id})
}

func apiKeyNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "not_found", "API key not found")
}

// DeleteAPIKey handles DELETE /api-keys/{id}, answering 200
// {object, id, deleted} (quirk envelope-005).
func (h *Handler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := h.store.APIKeys.Get(id); !ok {
		apiKeyNotFound(w)
		return
	}
	h.store.APIKeys.Delete(id)
	twincore.JSON(w, http.StatusOK, map[string]any{"object": "api_key", "id": id, "deleted": true})
}

// unverifiedKeyDomainMsg is Resend's answer to a send from the domain a
// sending key is bound to while that domain is unverified (quirk auth-007).
const unverifiedKeyDomainMsg = "The associated domain with your API key is not verified. Please, create a new API key with full access or with a verified domain."

// keyDomainRefused reports whether r's key is bound to a domain that is
// not verified and from names that domain. Resend answers such a send 400
// with no error name, and still lets the key send from other domains such
// as onboarding@resend.dev (quirk auth-007, probes n06, n07 of 2026-11-08).
func (h *Handler) keyDomainRefused(r *http.Request, from string) bool {
	key, ok := h.issuedKey(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if !ok || key.DomainID == "" {
		return false
	}
	d, ok := h.store.Domains.Get(key.DomainID)
	return ok && d.Status != store.DomainStatusVerified && strings.EqualFold(d.Name, senderDomain(from))
}

// refuseKeyDomain writes Resend's answer for keyDomainRefused.
func refuseKeyDomain(w http.ResponseWriter) {
	twincore.JSON(w, http.StatusBadRequest, map[string]any{
		"message": unverifiedKeyDomainMsg, "statusCode": http.StatusBadRequest,
	})
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
