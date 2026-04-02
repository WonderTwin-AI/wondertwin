package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-shopify/internal/store"
)

// --- Custom Collections ---

func (h *Handler) ListCustomCollections(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"custom_collections": h.store.CustomCollections.List()})
}

func (h *Handler) GetCustomCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := h.store.CustomCollections.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"custom_collection": c})
}

func (h *Handler) CreateCustomCollection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CustomCollection store.Collection `json:"custom_collection"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	c := req.CustomCollection
	if c.Title == "" {
		shopifyValidationError(w, "title", []string{"can't be blank"})
		return
	}
	c.ID = h.store.NextID()
	now := h.store.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	c.IsSmart = false
	if c.Handle == "" {
		c.Handle = strings.ToLower(strings.ReplaceAll(c.Title, " ", "-"))
	}
	if c.Published {
		c.PublishedAt = now
	}

	h.store.CustomCollections.Set(fmt.Sprintf("%d", c.ID), c)
	shopifyJSON(w, http.StatusCreated, map[string]any{"custom_collection": c})
}

func (h *Handler) UpdateCustomCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := h.store.CustomCollections.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		CustomCollection store.Collection `json:"custom_collection"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.CustomCollection.Title != "" {
		c.Title = req.CustomCollection.Title
	}
	if req.CustomCollection.BodyHTML != "" {
		c.BodyHTML = req.CustomCollection.BodyHTML
	}
	if req.CustomCollection.SortOrder != "" {
		c.SortOrder = req.CustomCollection.SortOrder
	}
	c.UpdatedAt = h.store.Now()
	h.store.CustomCollections.Set(id, c)
	shopifyJSON(w, http.StatusOK, map[string]any{"custom_collection": c})
}

func (h *Handler) DeleteCustomCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.CustomCollections.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- Smart Collections ---

func (h *Handler) ListSmartCollections(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"smart_collections": h.store.SmartCollections.List()})
}

func (h *Handler) GetSmartCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := h.store.SmartCollections.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"smart_collection": c})
}

func (h *Handler) CreateSmartCollection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SmartCollection store.Collection `json:"smart_collection"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	c := req.SmartCollection
	if c.Title == "" {
		shopifyValidationError(w, "title", []string{"can't be blank"})
		return
	}
	c.ID = h.store.NextID()
	now := h.store.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	c.IsSmart = true
	if c.Handle == "" {
		c.Handle = strings.ToLower(strings.ReplaceAll(c.Title, " ", "-"))
	}
	if c.Published {
		c.PublishedAt = now
	}

	h.store.SmartCollections.Set(fmt.Sprintf("%d", c.ID), c)
	shopifyJSON(w, http.StatusCreated, map[string]any{"smart_collection": c})
}

func (h *Handler) UpdateSmartCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := h.store.SmartCollections.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		SmartCollection store.Collection `json:"smart_collection"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.SmartCollection.Title != "" {
		c.Title = req.SmartCollection.Title
	}
	if req.SmartCollection.BodyHTML != "" {
		c.BodyHTML = req.SmartCollection.BodyHTML
	}
	if req.SmartCollection.SortOrder != "" {
		c.SortOrder = req.SmartCollection.SortOrder
	}
	c.UpdatedAt = h.store.Now()
	h.store.SmartCollections.Set(id, c)
	shopifyJSON(w, http.StatusOK, map[string]any{"smart_collection": c})
}

func (h *Handler) DeleteSmartCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.SmartCollections.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- Collects ---

func (h *Handler) ListCollects(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"collects": h.store.Collects.List()})
}

func (h *Handler) CreateCollect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Collect store.Collect `json:"collect"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	c := req.Collect
	if c.ProductID == 0 || c.CollectionID == 0 {
		shopifyValidationError(w, "collect", []string{"product_id and collection_id are required"})
		return
	}
	c.ID = h.store.NextID()
	c.CreatedAt = h.store.Now()

	h.store.Collects.Set(fmt.Sprintf("%d", c.ID), c)
	shopifyJSON(w, http.StatusCreated, map[string]any{"collect": c})
}

func (h *Handler) DeleteCollect(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.Collects.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}
