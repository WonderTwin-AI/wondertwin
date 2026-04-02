package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-hubspot/internal/store"
)

// ListProperties handles GET /crm/v3/properties/{objectType}
func (h *Handler) ListProperties(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	props := h.store.ListPropertiesByType(objectType)
	if props == nil {
		props = []store.Property{}
	}
	hsJSON(w, 200, map[string]any{"results": props})
}

// CreateProperty handles POST /crm/v3/properties/{objectType}
func (h *Handler) CreateProperty(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")

	var prop store.Property
	if err := json.NewDecoder(r.Body).Decode(&prop); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}
	if prop.Name == "" {
		hsError(w, 400, "VALIDATION_ERROR", "Property name is required")
		return
	}

	// Check for duplicates
	key := objectType + ":" + prop.Name
	if _, ok := h.store.Properties.Get(key); ok {
		hsError(w, 409, "CONFLICT", "Property "+prop.Name+" already exists for "+objectType)
		return
	}

	prop.ObjectType = objectType
	h.store.Properties.Set(key, prop)
	hsJSON(w, 201, prop)
}

// GetProperty handles GET /crm/v3/properties/{objectType}/{propertyName}
func (h *Handler) GetProperty(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	propertyName := chi.URLParam(r, "propertyName")

	key := objectType + ":" + propertyName
	prop, ok := h.store.Properties.Get(key)
	if !ok {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Property "+propertyName+" not found for "+objectType)
		return
	}
	hsJSON(w, 200, prop)
}

// UpdateProperty handles PATCH /crm/v3/properties/{objectType}/{propertyName}
func (h *Handler) UpdateProperty(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	propertyName := chi.URLParam(r, "propertyName")

	key := objectType + ":" + propertyName
	prop, ok := h.store.Properties.Get(key)
	if !ok {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Property "+propertyName+" not found for "+objectType)
		return
	}

	var body struct {
		Label       *string           `json:"label,omitempty"`
		Type        *string           `json:"type,omitempty"`
		FieldType   *string           `json:"fieldType,omitempty"`
		GroupName   *string           `json:"groupName,omitempty"`
		Description *string           `json:"description,omitempty"`
		Options     []store.PropertyOption `json:"options,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	if body.Label != nil {
		prop.Label = *body.Label
	}
	if body.Type != nil {
		prop.Type = *body.Type
	}
	if body.FieldType != nil {
		prop.FieldType = *body.FieldType
	}
	if body.GroupName != nil {
		prop.GroupName = *body.GroupName
	}
	if body.Description != nil {
		prop.Description = *body.Description
	}
	if body.Options != nil {
		prop.Options = body.Options
	}

	h.store.Properties.Set(key, prop)
	hsJSON(w, 200, prop)
}

// DeleteProperty handles DELETE /crm/v3/properties/{objectType}/{propertyName}
func (h *Handler) DeleteProperty(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	propertyName := chi.URLParam(r, "propertyName")

	key := objectType + ":" + propertyName
	if !h.store.Properties.Delete(key) {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Property "+propertyName+" not found for "+objectType)
		return
	}
	w.WriteHeader(204)
}

// ListPropertyGroups handles GET /crm/v3/properties/{objectType}/groups
func (h *Handler) ListPropertyGroups(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	groups := h.store.PropertyGroups.Filter(func(_ string, pg store.PropertyGroup) bool {
		return pg.ObjectType == objectType
	})
	if groups == nil {
		groups = []store.PropertyGroup{}
	}
	hsJSON(w, 200, map[string]any{"results": groups})
}

// CreatePropertyGroup handles POST /crm/v3/properties/{objectType}/groups
func (h *Handler) CreatePropertyGroup(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")

	var pg store.PropertyGroup
	if err := json.NewDecoder(r.Body).Decode(&pg); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}
	if pg.Name == "" {
		hsError(w, 400, "VALIDATION_ERROR", "Property group name is required")
		return
	}

	key := objectType + ":" + pg.Name
	if _, ok := h.store.PropertyGroups.Get(key); ok {
		hsError(w, 409, "CONFLICT", "Property group "+pg.Name+" already exists for "+objectType)
		return
	}

	pg.ObjectType = objectType
	h.store.PropertyGroups.Set(key, pg)
	hsJSON(w, 201, pg)
}

// GetPropertyGroup handles GET /crm/v3/properties/{objectType}/groups/{groupName}
func (h *Handler) GetPropertyGroup(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	groupName := chi.URLParam(r, "groupName")

	key := objectType + ":" + groupName
	pg, ok := h.store.PropertyGroups.Get(key)
	if !ok {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Property group "+groupName+" not found for "+objectType)
		return
	}
	hsJSON(w, 200, pg)
}

// UpdatePropertyGroup handles PATCH /crm/v3/properties/{objectType}/groups/{groupName}
func (h *Handler) UpdatePropertyGroup(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	groupName := chi.URLParam(r, "groupName")

	key := objectType + ":" + groupName
	pg, ok := h.store.PropertyGroups.Get(key)
	if !ok {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Property group "+groupName+" not found for "+objectType)
		return
	}

	var body struct {
		Label *string `json:"label,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}
	if body.Label != nil {
		pg.Label = *body.Label
	}

	h.store.PropertyGroups.Set(key, pg)
	hsJSON(w, 200, pg)
}

// DeletePropertyGroup handles DELETE /crm/v3/properties/{objectType}/groups/{groupName}
func (h *Handler) DeletePropertyGroup(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	groupName := chi.URLParam(r, "groupName")

	key := objectType + ":" + groupName
	if !h.store.PropertyGroups.Delete(key) {
		hsError(w, 404, "OBJECT_NOT_FOUND", "Property group "+groupName+" not found for "+objectType)
		return
	}
	w.WriteHeader(204)
}
