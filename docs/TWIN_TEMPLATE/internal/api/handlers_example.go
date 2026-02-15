package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-TEMPLATE/internal/store"
)

// createResourceRequest matches the real service's create request body.
// Use the exact same JSON field names and structure as the real API.
type createResourceRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// updateResourceRequest matches the real service's update request body.
type updateResourceRequest struct {
	Name        *string           `json:"name,omitempty"`
	Description *string           `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// CreateResource handles POST /v1/resources
func (h *Handler) CreateResource(w http.ResponseWriter, r *http.Request) {
	var req createResourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Return errors in the same format the real service uses.
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]any{
				"message": "Invalid request body: " + err.Error(),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Validate required fields.
	if req.Name == "" {
		twincore.JSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error": map[string]any{
				"message": "The 'name' field is required.",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Create the resource in the store.
	now := h.store.Clock.Now()
	id := h.store.Resources.NextID()

	resource := store.Resource{
		ID:          id,
		Object:      "resource",
		Name:        req.Name,
		Description: req.Description,
		Metadata:    req.Metadata,
		CreatedAt:   now.Unix(),
	}

	h.store.Resources.Set(id, resource)

	twincore.JSON(w, http.StatusCreated, resource)
}

// GetResource handles GET /v1/resources/{id}
func (h *Handler) GetResource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	resource, ok := h.store.Resources.Get(id)
	if !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"error": map[string]any{
				"message": "No such resource: " + id,
				"type":    "invalid_request_error",
			},
		})
		return
	}

	twincore.JSON(w, http.StatusOK, resource)
}

// ListResources handles GET /v1/resources
func (h *Handler) ListResources(w http.ResponseWriter, r *http.Request) {
	resources := h.store.Resources.List()

	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   resources,
	})
}

// UpdateResource handles PATCH /v1/resources/{id}
func (h *Handler) UpdateResource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	resource, ok := h.store.Resources.Get(id)
	if !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"error": map[string]any{
				"message": "No such resource: " + id,
				"type":    "invalid_request_error",
			},
		})
		return
	}

	var req updateResourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]any{
				"message": "Invalid request body: " + err.Error(),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Apply partial updates.
	if req.Name != nil {
		resource.Name = *req.Name
	}
	if req.Description != nil {
		resource.Description = *req.Description
	}
	if req.Metadata != nil {
		resource.Metadata = req.Metadata
	}

	h.store.Resources.Set(id, resource)

	twincore.JSON(w, http.StatusOK, resource)
}

// DeleteResource handles DELETE /v1/resources/{id}
func (h *Handler) DeleteResource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	_, ok := h.store.Resources.Get(id)
	if !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"error": map[string]any{
				"message": "No such resource: " + id,
				"type":    "invalid_request_error",
			},
		})
		return
	}

	h.store.Resources.Delete(id)

	// Return the same deletion response format as the real service.
	twincore.JSON(w, http.StatusOK, map[string]any{
		"id":      id,
		"object":  "resource",
		"deleted": true,
	})
}
