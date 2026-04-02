package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-hubspot/internal/store"
)

// ListObjects handles GET /crm/v3/objects/{objectType}
func (h *Handler) ListObjects(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	limit := intQuery(r, "limit", 10)
	after := r.URL.Query().Get("after")

	all := h.store.ListObjectsByType(objectType)

	startIdx := 0
	if after != "" {
		for i, obj := range all {
			if obj.ID == after {
				startIdx = i + 1
				break
			}
		}
	}

	endIdx := startIdx + limit
	hasMore := false
	if endIdx >= len(all) {
		endIdx = len(all)
	} else {
		hasMore = true
	}

	results := all[startIdx:endIdx]

	resp := map[string]any{
		"results": results,
	}
	if hasMore {
		resp["paging"] = map[string]any{
			"next": map[string]any{
				"after": results[len(results)-1].ID,
			},
		}
	}
	hsJSON(w, 200, resp)
}

// CreateObject handles POST /crm/v3/objects/{objectType}
func (h *Handler) CreateObject(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")

	var body struct {
		Properties map[string]string `json:"properties"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}
	if body.Properties == nil {
		body.Properties = make(map[string]string)
	}

	now := h.store.Now()
	id := h.store.NextID()
	obj := store.CRMObject{
		ID:         id,
		Properties: body.Properties,
		CreatedAt:  now,
		UpdatedAt:  now,
		ObjectType: objectType,
	}
	h.store.Objects.Set(id, obj)
	hsJSON(w, 201, obj)
}

// GetObject handles GET /crm/v3/objects/{objectType}/{objectId}
func (h *Handler) GetObject(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	objectID := chi.URLParam(r, "objectId")

	obj, ok := h.store.GetObjectByID(objectType, objectID)
	if !ok {
		hsError(w, 404, "OBJECT_NOT_FOUND", objectType+" with id "+objectID+" not found")
		return
	}
	if obj.Archived {
		hsError(w, 404, "OBJECT_NOT_FOUND", objectType+" with id "+objectID+" not found")
		return
	}
	hsJSON(w, 200, obj)
}

// UpdateObject handles PATCH /crm/v3/objects/{objectType}/{objectId}
func (h *Handler) UpdateObject(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	objectID := chi.URLParam(r, "objectId")

	obj, ok := h.store.GetObjectByID(objectType, objectID)
	if !ok || obj.Archived {
		hsError(w, 404, "OBJECT_NOT_FOUND", objectType+" with id "+objectID+" not found")
		return
	}

	var body struct {
		Properties map[string]string `json:"properties"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	for k, v := range body.Properties {
		obj.Properties[k] = v
	}
	obj.UpdatedAt = h.store.Now()
	h.store.Objects.Set(objectID, *obj)
	hsJSON(w, 200, obj)
}

// ArchiveObject handles DELETE /crm/v3/objects/{objectType}/{objectId}
func (h *Handler) ArchiveObject(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")
	objectID := chi.URLParam(r, "objectId")

	obj, ok := h.store.GetObjectByID(objectType, objectID)
	if !ok || obj.Archived {
		hsError(w, 404, "OBJECT_NOT_FOUND", objectType+" with id "+objectID+" not found")
		return
	}

	obj.Archived = true
	obj.UpdatedAt = h.store.Now()
	h.store.Objects.Set(objectID, *obj)
	w.WriteHeader(204)
}

// BatchCreateObjects handles POST /crm/v3/objects/{objectType}/batch/create
func (h *Handler) BatchCreateObjects(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")

	var body struct {
		Inputs []struct {
			Properties map[string]string `json:"properties"`
		} `json:"inputs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	startedAt := h.store.Now()
	results := make([]store.CRMObject, 0, len(body.Inputs))
	for _, input := range body.Inputs {
		props := input.Properties
		if props == nil {
			props = make(map[string]string)
		}
		now := h.store.Now()
		id := h.store.NextID()
		obj := store.CRMObject{
			ID:         id,
			Properties: props,
			CreatedAt:  now,
			UpdatedAt:  now,
			ObjectType: objectType,
		}
		h.store.Objects.Set(id, obj)
		results = append(results, obj)
	}

	hsJSON(w, 201, map[string]any{
		"status":      "COMPLETE",
		"results":     results,
		"startedAt":   startedAt,
		"completedAt": h.store.Now(),
	})
}

// BatchReadObjects handles POST /crm/v3/objects/{objectType}/batch/read
func (h *Handler) BatchReadObjects(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")

	var body struct {
		Inputs []struct {
			ID string `json:"id"`
		} `json:"inputs"`
		Properties []string `json:"properties"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	startedAt := h.store.Now()
	results := make([]store.CRMObject, 0, len(body.Inputs))
	for _, input := range body.Inputs {
		obj, ok := h.store.GetObjectByID(objectType, input.ID)
		if !ok || obj.Archived {
			continue
		}
		if len(body.Properties) > 0 {
			filtered := make(map[string]string, len(body.Properties))
			for _, p := range body.Properties {
				if v, exists := obj.Properties[p]; exists {
					filtered[p] = v
				}
			}
			obj.Properties = filtered
		}
		results = append(results, *obj)
	}

	hsJSON(w, 200, map[string]any{
		"status":      "COMPLETE",
		"results":     results,
		"startedAt":   startedAt,
		"completedAt": h.store.Now(),
	})
}

// BatchUpdateObjects handles POST /crm/v3/objects/{objectType}/batch/update
func (h *Handler) BatchUpdateObjects(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")

	var body struct {
		Inputs []struct {
			ID         string            `json:"id"`
			Properties map[string]string `json:"properties"`
		} `json:"inputs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	startedAt := h.store.Now()
	results := make([]store.CRMObject, 0, len(body.Inputs))
	for _, input := range body.Inputs {
		obj, ok := h.store.GetObjectByID(objectType, input.ID)
		if !ok || obj.Archived {
			continue
		}
		for k, v := range input.Properties {
			obj.Properties[k] = v
		}
		obj.UpdatedAt = h.store.Now()
		h.store.Objects.Set(input.ID, *obj)
		results = append(results, *obj)
	}

	hsJSON(w, 200, map[string]any{
		"status":      "COMPLETE",
		"results":     results,
		"startedAt":   startedAt,
		"completedAt": h.store.Now(),
	})
}

// BatchUpsertObjects handles POST /crm/v3/objects/{objectType}/batch/upsert
func (h *Handler) BatchUpsertObjects(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")

	var body struct {
		Inputs []struct {
			ID         string            `json:"id,omitempty"`
			IDProperty string            `json:"idProperty,omitempty"`
			Properties map[string]string `json:"properties"`
		} `json:"inputs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	startedAt := h.store.Now()
	results := make([]store.CRMObject, 0, len(body.Inputs))
	for _, input := range body.Inputs {
		props := input.Properties
		if props == nil {
			props = make(map[string]string)
		}

		// Try to find existing object by ID or idProperty
		var existing *store.CRMObject
		if input.ID != "" {
			existing, _ = h.store.GetObjectByID(objectType, input.ID)
		} else if input.IDProperty != "" {
			// Search by unique property
			propVal := props[input.IDProperty]
			if propVal != "" {
				objs := h.store.ListObjectsByType(objectType)
				for i := range objs {
					if objs[i].Properties[input.IDProperty] == propVal {
						existing = &objs[i]
						break
					}
				}
			}
		}

		if existing != nil && !existing.Archived {
			for k, v := range props {
				existing.Properties[k] = v
			}
			existing.UpdatedAt = h.store.Now()
			h.store.Objects.Set(existing.ID, *existing)
			results = append(results, *existing)
		} else {
			now := h.store.Now()
			id := h.store.NextID()
			obj := store.CRMObject{
				ID:         id,
				Properties: props,
				CreatedAt:  now,
				UpdatedAt:  now,
				ObjectType: objectType,
			}
			h.store.Objects.Set(id, obj)
			results = append(results, obj)
		}
	}

	hsJSON(w, 200, map[string]any{
		"status":      "COMPLETE",
		"results":     results,
		"startedAt":   startedAt,
		"completedAt": h.store.Now(),
	})
}

// BatchArchiveObjects handles POST /crm/v3/objects/{objectType}/batch/archive
func (h *Handler) BatchArchiveObjects(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")

	var body struct {
		Inputs []struct {
			ID string `json:"id"`
		} `json:"inputs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	for _, input := range body.Inputs {
		obj, ok := h.store.GetObjectByID(objectType, input.ID)
		if ok && !obj.Archived {
			obj.Archived = true
			obj.UpdatedAt = h.store.Now()
			h.store.Objects.Set(input.ID, *obj)
		}
	}
	w.WriteHeader(204)
}

// SearchObjects handles POST /crm/v3/objects/{objectType}/search
func (h *Handler) SearchObjects(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "objectType")

	var body struct {
		FilterGroups []struct {
			Filters []store.SearchFilter `json:"filters"`
		} `json:"filterGroups"`
		Sorts      []any    `json:"sorts"`
		Properties []string `json:"properties"`
		Limit      int      `json:"limit"`
		After      string   `json:"after"`
		Query      string   `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	if body.Limit <= 0 {
		body.Limit = 10
	}

	// Flatten filters from all filter groups (OR between groups, AND within group)
	// For simplicity, collect unique results across all filter groups.
	seen := make(map[string]bool)
	var allResults []store.CRMObject

	if len(body.FilterGroups) == 0 {
		allResults = h.store.SearchObjects(objectType, body.Query, nil)
	} else {
		for _, fg := range body.FilterGroups {
			results := h.store.SearchObjects(objectType, body.Query, fg.Filters)
			for _, obj := range results {
				if !seen[obj.ID] {
					seen[obj.ID] = true
					allResults = append(allResults, obj)
				}
			}
		}
	}

	// Apply after cursor
	startIdx := 0
	if body.After != "" {
		for i, obj := range allResults {
			if obj.ID == body.After {
				startIdx = i + 1
				break
			}
		}
	}

	endIdx := startIdx + body.Limit
	hasMore := false
	if endIdx >= len(allResults) {
		endIdx = len(allResults)
	} else {
		hasMore = true
	}

	page := allResults[startIdx:endIdx]

	// Filter properties if requested
	if len(body.Properties) > 0 {
		for i := range page {
			filtered := make(map[string]string, len(body.Properties))
			for _, p := range body.Properties {
				if v, exists := page[i].Properties[p]; exists {
					filtered[p] = v
				}
			}
			page[i].Properties = filtered
		}
	}

	resp := map[string]any{
		"total":   len(allResults),
		"results": page,
	}
	if hasMore {
		resp["paging"] = map[string]any{
			"next": map[string]any{
				"after": page[len(page)-1].ID,
			},
		}
	}
	hsJSON(w, 200, resp)
}

// intQuery parses an integer query parameter with a default value.
func intQuery(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
