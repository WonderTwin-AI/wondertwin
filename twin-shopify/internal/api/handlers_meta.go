package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-shopify/internal/store"
)

// --- Shop ---

func (h *Handler) GetShop(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"shop": h.store.Shop})
}

// --- Metafields ---

func (h *Handler) ListMetafields(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"metafields": h.store.Metafields.List()})
}

func (h *Handler) GetMetafield(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	mf, ok := h.store.Metafields.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"metafield": mf})
}

func (h *Handler) CreateMetafield(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Metafield store.Metafield `json:"metafield"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	mf := req.Metafield
	if mf.Namespace == "" {
		shopifyValidationError(w, "namespace", []string{"can't be blank"})
		return
	}
	if mf.Key == "" {
		shopifyValidationError(w, "key", []string{"can't be blank"})
		return
	}
	mf.ID = h.store.NextID()
	now := h.store.Now()
	mf.CreatedAt = now
	mf.UpdatedAt = now
	if mf.Type == "" {
		mf.Type = "single_line_text_field"
	}
	if mf.OwnerResource == "" {
		mf.OwnerResource = "shop"
		mf.OwnerID = h.store.Shop.ID
	}

	h.store.Metafields.Set(fmt.Sprintf("%d", mf.ID), mf)
	shopifyJSON(w, http.StatusCreated, map[string]any{"metafield": mf})
}

func (h *Handler) UpdateMetafield(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	mf, ok := h.store.Metafields.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Metafield store.Metafield `json:"metafield"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Metafield.Value != "" {
		mf.Value = req.Metafield.Value
	}
	if req.Metafield.Type != "" {
		mf.Type = req.Metafield.Type
	}
	mf.UpdatedAt = h.store.Now()
	h.store.Metafields.Set(id, mf)
	shopifyJSON(w, http.StatusOK, map[string]any{"metafield": mf})
}

func (h *Handler) DeleteMetafield(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.Metafields.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- Access Scopes ---

func (h *Handler) ListAccessScopes(w http.ResponseWriter, r *http.Request) {
	scopes := []map[string]string{
		{"handle": "read_products"},
		{"handle": "write_products"},
		{"handle": "read_orders"},
		{"handle": "write_orders"},
		{"handle": "read_customers"},
		{"handle": "write_customers"},
		{"handle": "read_inventory"},
		{"handle": "write_inventory"},
		{"handle": "read_fulfillments"},
		{"handle": "write_fulfillments"},
		{"handle": "read_draft_orders"},
		{"handle": "write_draft_orders"},
		{"handle": "read_content"},
		{"handle": "write_content"},
		{"handle": "read_themes"},
		{"handle": "write_themes"},
		{"handle": "read_script_tags"},
		{"handle": "write_script_tags"},
		{"handle": "read_price_rules"},
		{"handle": "write_price_rules"},
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"access_scopes": scopes})
}
