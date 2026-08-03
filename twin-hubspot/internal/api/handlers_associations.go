package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-hubspot/internal/store"
)

// ListAssociations handles GET /crm/v4/objects/{fromObjectType}/{objectId}/associations/{toObjectType}
func (h *Handler) ListAssociations(w http.ResponseWriter, r *http.Request) {
	fromType := chi.URLParam(r, "fromObjectType")
	objectID := chi.URLParam(r, "objectId")
	toType := chi.URLParam(r, "toObjectType")

	assocs := h.store.ListAssociations(fromType, objectID, toType)

	results := make([]map[string]any, 0, len(assocs))
	for _, a := range assocs {
		results = append(results, map[string]any{
			"toObjectId": a.ToObjectID,
			"associationTypes": []map[string]any{
				{
					"category": a.AssociationCategory,
					"typeId":   a.AssociationTypeID,
					"label":    nil,
				},
			},
		})
	}
	hsJSON(w, 200, map[string]any{"results": results})
}

// CreateDefaultAssociation handles PUT /crm/v4/objects/{fromObjectType}/{objectId}/associations/default/{toObjectType}/{toObjectId}
func (h *Handler) CreateDefaultAssociation(w http.ResponseWriter, r *http.Request) {
	fromType := chi.URLParam(r, "fromObjectType")
	objectID := chi.URLParam(r, "objectId")
	toType := chi.URLParam(r, "toObjectType")
	toObjectID := chi.URLParam(r, "toObjectId")

	id := h.store.NextID()
	assoc := store.Association{
		ID:                  id,
		FromObjectType:      fromType,
		FromObjectID:        objectID,
		ToObjectType:        toType,
		ToObjectID:          toObjectID,
		AssociationCategory: "HUBSPOT_DEFINED",
		AssociationTypeID:   0,
	}
	h.store.Associations.Set(id, assoc)

	// Create the reverse association as well
	reverseID := h.store.NextID()
	reverse := store.Association{
		ID:                  reverseID,
		FromObjectType:      toType,
		FromObjectID:        toObjectID,
		ToObjectType:        fromType,
		ToObjectID:          objectID,
		AssociationCategory: "HUBSPOT_DEFINED",
		AssociationTypeID:   0,
	}
	h.store.Associations.Set(reverseID, reverse)

	hsJSON(w, 200, map[string]any{
		"fromObjectTypeId": fromType,
		"fromObjectId":     objectID,
		"toObjectTypeId":   toType,
		"toObjectId":       toObjectID,
		"labels":           []any{},
	})
}

// DeleteAssociation handles DELETE /crm/v4/objects/{fromObjectType}/{objectId}/associations/{toObjectType}/{toObjectId}
func (h *Handler) DeleteAssociation(w http.ResponseWriter, r *http.Request) {
	fromType := chi.URLParam(r, "fromObjectType")
	objectID := chi.URLParam(r, "objectId")
	toType := chi.URLParam(r, "toObjectType")
	toObjectID := chi.URLParam(r, "toObjectId")

	// Delete forward associations
	ids, _ := h.store.Associations.FilterWithIDs(func(_ string, a store.Association) bool {
		return a.FromObjectType == fromType && a.FromObjectID == objectID &&
			a.ToObjectType == toType && a.ToObjectID == toObjectID
	})
	for _, id := range ids {
		h.store.Associations.Delete(id)
	}

	// Delete reverse associations
	reverseIDs, _ := h.store.Associations.FilterWithIDs(func(_ string, a store.Association) bool {
		return a.FromObjectType == toType && a.FromObjectID == toObjectID &&
			a.ToObjectType == fromType && a.ToObjectID == objectID
	})
	for _, id := range reverseIDs {
		h.store.Associations.Delete(id)
	}

	w.WriteHeader(204)
}

// BatchCreateAssociations handles POST /crm/v4/associations/{fromObjectType}/{toObjectType}/batch/create
func (h *Handler) BatchCreateAssociations(w http.ResponseWriter, r *http.Request) {
	fromType := chi.URLParam(r, "fromObjectType")
	toType := chi.URLParam(r, "toObjectType")

	var body struct {
		Inputs []struct {
			From struct {
				ID string `json:"id"`
			} `json:"from"`
			To struct {
				ID string `json:"id"`
			} `json:"to"`
			Types []struct {
				AssociationCategory string `json:"associationCategory"`
				AssociationTypeID   int    `json:"associationTypeId"`
			} `json:"types"`
		} `json:"inputs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	startedAt := h.store.Now()
	results := make([]map[string]any, 0, len(body.Inputs))
	for _, input := range body.Inputs {
		category := "HUBSPOT_DEFINED"
		typeID := 0
		if len(input.Types) > 0 {
			category = input.Types[0].AssociationCategory
			typeID = input.Types[0].AssociationTypeID
		}

		id := h.store.NextID()
		assoc := store.Association{
			ID:                  id,
			FromObjectType:      fromType,
			FromObjectID:        input.From.ID,
			ToObjectType:        toType,
			ToObjectID:          input.To.ID,
			AssociationCategory: category,
			AssociationTypeID:   typeID,
		}
		h.store.Associations.Set(id, assoc)

		results = append(results, map[string]any{
			"from":                map[string]any{"id": input.From.ID},
			"to":                  map[string]any{"id": input.To.ID},
			"associationCategory": category,
			"associationTypeId":   typeID,
		})
	}

	hsJSON(w, 201, map[string]any{
		"status":      "COMPLETE",
		"results":     results,
		"startedAt":   startedAt,
		"completedAt": h.store.Now(),
	})
}

// BatchReadAssociations handles POST /crm/v4/associations/{fromObjectType}/{toObjectType}/batch/read
func (h *Handler) BatchReadAssociations(w http.ResponseWriter, r *http.Request) {
	fromType := chi.URLParam(r, "fromObjectType")
	toType := chi.URLParam(r, "toObjectType")

	var body struct {
		Inputs []struct {
			ID string `json:"id"`
		} `json:"inputs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	startedAt := h.store.Now()
	results := make([]map[string]any, 0, len(body.Inputs))
	for _, input := range body.Inputs {
		assocs := h.store.ListAssociations(fromType, input.ID, toType)
		toList := make([]map[string]any, 0, len(assocs))
		for _, a := range assocs {
			toList = append(toList, map[string]any{
				"toObjectId": a.ToObjectID,
				"associationTypes": []map[string]any{
					{
						"category": a.AssociationCategory,
						"typeId":   a.AssociationTypeID,
						"label":    nil,
					},
				},
			})
		}
		results = append(results, map[string]any{
			"from": map[string]any{"id": input.ID},
			"to":   toList,
		})
	}

	hsJSON(w, 200, map[string]any{
		"status":      "COMPLETE",
		"results":     results,
		"startedAt":   startedAt,
		"completedAt": h.store.Now(),
	})
}

// BatchArchiveAssociations handles POST /crm/v4/associations/{fromObjectType}/{toObjectType}/batch/archive
func (h *Handler) BatchArchiveAssociations(w http.ResponseWriter, r *http.Request) {
	fromType := chi.URLParam(r, "fromObjectType")
	toType := chi.URLParam(r, "toObjectType")

	var body struct {
		Inputs []struct {
			From struct {
				ID string `json:"id"`
			} `json:"from"`
			To struct {
				ID string `json:"id"`
			} `json:"to"`
		} `json:"inputs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		hsError(w, 400, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	for _, input := range body.Inputs {
		ids, _ := h.store.Associations.FilterWithIDs(func(_ string, a store.Association) bool {
			return a.FromObjectType == fromType && a.FromObjectID == input.From.ID &&
				a.ToObjectType == toType && a.ToObjectID == input.To.ID
		})
		for _, id := range ids {
			h.store.Associations.Delete(id)
		}
	}
	w.WriteHeader(204)
}

// ListAssociationLabels handles GET /crm/v4/associations/{fromObjectType}/{toObjectType}/labels
func (h *Handler) ListAssociationLabels(w http.ResponseWriter, r *http.Request) {
	fromType := chi.URLParam(r, "fromObjectType")
	toType := chi.URLParam(r, "toObjectType")

	// Return default HubSpot-defined association label
	hsJSON(w, 200, map[string]any{
		"results": []map[string]any{
			{
				"category": "HUBSPOT_DEFINED",
				"typeId":   0,
				"label":    fromType + "_to_" + toType,
			},
		},
	})
}
