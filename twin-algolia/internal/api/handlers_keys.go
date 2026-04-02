package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"

	"github.com/wondertwin-ai/wondertwin/twin-algolia/internal/store"
)

// ListKeys handles GET /1/keys
func (h *Handler) ListKeys(w http.ResponseWriter, r *http.Request) {
	keys := h.store.APIKeys.List()
	twincore.JSON(w, http.StatusOK, map[string]any{
		"keys": keys,
	})
}

// GetKey handles GET /1/keys/{keyID}
func (h *Handler) GetKey(w http.ResponseWriter, r *http.Request) {
	keyID := chi.URLParam(r, "keyID")

	key, ok := h.store.APIKeys.Get(keyID)
	if !ok {
		algoliaError(w, http.StatusNotFound, "Key does not exist")
		return
	}

	twincore.JSON(w, http.StatusOK, key)
}

// CreateKey handles POST /1/keys
func (h *Handler) CreateKey(w http.ResponseWriter, r *http.Request) {
	var body store.APIKey
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	if body.Value == "" {
		body.Value = generateKeyValue()
	}
	body.CreatedAt = time.Now().Unix()

	h.store.APIKeys.Set(body.Value, body)

	twincore.JSON(w, http.StatusCreated, map[string]any{
		"key":       body.Value,
		"createdAt": h.store.Now(),
	})
}

// UpdateKey handles PUT /1/keys/{keyID}
func (h *Handler) UpdateKey(w http.ResponseWriter, r *http.Request) {
	keyID := chi.URLParam(r, "keyID")

	existing, ok := h.store.APIKeys.Get(keyID)
	if !ok {
		algoliaError(w, http.StatusNotFound, "Key does not exist")
		return
	}

	var body store.APIKey
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Merge: keep value, update provided fields.
	if len(body.ACL) > 0 {
		existing.ACL = body.ACL
	}
	if body.Description != "" {
		existing.Description = body.Description
	}
	if len(body.Indexes) > 0 {
		existing.Indexes = body.Indexes
	}
	if body.MaxHitsPerQuery > 0 {
		existing.MaxHitsPerQuery = body.MaxHitsPerQuery
	}
	if body.MaxQueriesPerIPPerHour > 0 {
		existing.MaxQueriesPerIPPerHour = body.MaxQueriesPerIPPerHour
	}
	if body.Validity > 0 {
		existing.Validity = body.Validity
	}

	h.store.APIKeys.Set(keyID, existing)

	twincore.JSON(w, http.StatusOK, map[string]any{
		"key":       keyID,
		"updatedAt": h.store.Now(),
	})
}

// DeleteKey handles DELETE /1/keys/{keyID}
func (h *Handler) DeleteKey(w http.ResponseWriter, r *http.Request) {
	keyID := chi.URLParam(r, "keyID")

	if !h.store.APIKeys.Delete(keyID) {
		algoliaError(w, http.StatusNotFound, "Key does not exist")
		return
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"deletedAt": h.store.Now(),
	})
}

// RestoreKey handles POST /1/keys/{keyID}/restore
func (h *Handler) RestoreKey(w http.ResponseWriter, r *http.Request) {
	keyID := chi.URLParam(r, "keyID")

	// In the twin, we don't keep deleted keys. Restore creates a minimal key.
	key := store.APIKey{
		Value:     keyID,
		ACL:       []string{"search"},
		CreatedAt: time.Now().Unix(),
	}
	h.store.APIKeys.Set(keyID, key)

	twincore.JSON(w, http.StatusOK, map[string]any{
		"createdAt": h.store.Now(),
	})
}

// generateKeyValue produces a random hex API key.
func generateKeyValue() string {
	b := make([]byte, 16)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}
