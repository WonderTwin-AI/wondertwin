package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/search"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// AddRecord handles POST /1/indexes/{indexName} — adds a record with auto-generated objectID.
func (h *Handler) AddRecord(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	objectID, _ := body["objectID"].(string)
	if objectID == "" {
		objectID = h.store.Clock.Now().UTC().Format("20060102150405") + fmt.Sprintf("%d", h.store.NextTaskID())
	}
	delete(body, "objectID")

	rec := &search.Record{
		ObjectID:   objectID,
		Attributes: body,
	}
	if err := h.store.Engine.SaveRecord(r.Context(), indexName, rec); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp := h.store.TaskResult(indexName)
	resp["objectID"] = objectID
	twincore.JSON(w, http.StatusCreated, resp)
}

// SaveRecord handles PUT /1/indexes/{indexName}/{objectID} — add or replace a record.
func (h *Handler) SaveRecord(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")
	objectID := chi.URLParam(r, "objectID")

	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}
	delete(body, "objectID")

	rec := &search.Record{
		ObjectID:   objectID,
		Attributes: body,
	}
	if err := h.store.Engine.SaveRecord(r.Context(), indexName, rec); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp := h.store.TaskResult(indexName)
	resp["objectID"] = objectID
	twincore.JSON(w, http.StatusOK, resp)
}

// PartialUpdate handles POST /1/indexes/{indexName}/{objectID}/partial — partial attribute update.
func (h *Handler) PartialUpdate(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")
	objectID := chi.URLParam(r, "objectID")

	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}
	delete(body, "objectID")

	// Try to get existing record; if not found, create new.
	existing, err := h.store.Engine.GetRecord(indexName, objectID)
	if err != nil {
		// Record doesn't exist, create with partial attributes.
		existing = &search.Record{
			ObjectID:   objectID,
			Attributes: make(map[string]any),
		}
	}

	// Merge new attributes into existing.
	for k, v := range body {
		existing.Attributes[k] = v
	}

	if err := h.store.Engine.SaveRecord(r.Context(), indexName, existing); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp := h.store.TaskResult(indexName)
	resp["objectID"] = objectID
	twincore.JSON(w, http.StatusOK, resp)
}

// DeleteRecord handles DELETE /1/indexes/{indexName}/{objectID}
func (h *Handler) DeleteRecord(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")
	objectID := chi.URLParam(r, "objectID")

	if err := h.store.Engine.DeleteRecord(r.Context(), indexName, objectID); err != nil {
		algoliaError(w, http.StatusNotFound, err.Error())
		return
	}

	resp := h.store.TaskResult(indexName)
	twincore.JSON(w, http.StatusOK, resp)
}

// GetRecord handles GET /1/indexes/{indexName}/{objectID}
func (h *Handler) GetRecord(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")
	objectID := chi.URLParam(r, "objectID")

	rec, err := h.store.Engine.GetRecord(indexName, objectID)
	if err != nil {
		algoliaError(w, http.StatusNotFound, err.Error())
		return
	}

	// Flatten attributes to top level.
	resp := make(map[string]any, len(rec.Attributes)+1)
	for k, v := range rec.Attributes {
		resp[k] = v
	}
	resp["objectID"] = rec.ObjectID

	twincore.JSON(w, http.StatusOK, resp)
}

// BatchWrite handles POST /1/indexes/{indexName}/batch
func (h *Handler) BatchWrite(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	var body struct {
		Requests []batchAction `json:"requests"`
	}
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	objectIDs := make([]string, 0, len(body.Requests))
	for _, action := range body.Requests {
		oid, err := h.executeBatchAction(r, indexName, action)
		if err != nil {
			algoliaError(w, http.StatusBadRequest, err.Error())
			return
		}
		objectIDs = append(objectIDs, oid)
	}

	resp := h.store.TaskResult(indexName)
	resp["objectIDs"] = objectIDs
	twincore.JSON(w, http.StatusOK, resp)
}

// MultiBatchWrite handles POST /1/indexes/*/batch
func (h *Handler) MultiBatchWrite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Requests []struct {
			Action    string         `json:"action"`
			IndexName string         `json:"indexName"`
			Body      map[string]any `json:"body"`
		} `json:"requests"`
	}
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	objectIDs := make([]string, 0, len(body.Requests))
	for _, req := range body.Requests {
		action := batchAction{
			Action: req.Action,
			Body:   req.Body,
		}
		oid, err := h.executeBatchAction(r, req.IndexName, action)
		if err != nil {
			algoliaError(w, http.StatusBadRequest, err.Error())
			return
		}
		objectIDs = append(objectIDs, oid)
	}

	resp := h.store.TaskResult("")
	resp["objectIDs"] = objectIDs
	twincore.JSON(w, http.StatusOK, resp)
}

// DeleteByQuery handles POST /1/indexes/{indexName}/deleteByQuery
func (h *Handler) DeleteByQuery(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	var body struct {
		Params       string   `json:"params,omitempty"`
		Query        string   `json:"query,omitempty"`
		Filters      string   `json:"filters,omitempty"`
		FacetFilters []string `json:"facetFilters,omitempty"`
	}
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	sp := &search.SearchParams{
		Query:        body.Query,
		Filters:      body.Filters,
		FacetFilters: body.FacetFilters,
		HitsPerPage:  1000,
	}
	if body.Params != "" && sp.Query == "" {
		sp.Query = parseParamQuery(body.Params)
	}

	result, err := h.store.Engine.Search(r.Context(), indexName, sp)
	if err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	for _, hit := range result.Hits {
		if err := h.store.Engine.DeleteRecord(r.Context(), indexName, hit.ObjectID); err != nil {
			algoliaError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	twincore.JSON(w, http.StatusOK, h.store.TaskResult(indexName))
}

// ClearIndex handles POST /1/indexes/{indexName}/clear
func (h *Handler) ClearIndex(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	if err := h.store.Engine.ClearIndex(indexName); err != nil {
		algoliaError(w, http.StatusNotFound, err.Error())
		return
	}

	twincore.JSON(w, http.StatusOK, h.store.TaskResult(indexName))
}

// GetMultipleRecords handles POST /1/indexes/*/objects
func (h *Handler) GetMultipleRecords(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Requests []struct {
			IndexName            string   `json:"indexName"`
			ObjectID             string   `json:"objectID"`
			AttributesToRetrieve []string `json:"attributesToRetrieve,omitempty"`
		} `json:"requests"`
	}
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	results := make([]map[string]any, 0, len(body.Requests))
	for _, req := range body.Requests {
		rec, err := h.store.Engine.GetRecord(req.IndexName, req.ObjectID)
		if err != nil {
			// Algolia returns null for missing records in multi-get.
			results = append(results, nil)
			continue
		}

		attrs := rec.Attributes
		if len(req.AttributesToRetrieve) > 0 {
			filtered := make(map[string]any, len(req.AttributesToRetrieve))
			for _, key := range req.AttributesToRetrieve {
				if v, ok := attrs[key]; ok {
					filtered[key] = v
				}
			}
			attrs = filtered
		}

		m := make(map[string]any, len(attrs)+1)
		for k, v := range attrs {
			m[k] = v
		}
		m["objectID"] = rec.ObjectID
		results = append(results, m)
	}

	twincore.JSON(w, http.StatusOK, map[string]any{"results": results})
}

// batchAction represents a single action in a batch request.
type batchAction struct {
	Action string         `json:"action"`
	Body   map[string]any `json:"body"`
}

// executeBatchAction executes a single batch action and returns the objectID.
func (h *Handler) executeBatchAction(r *http.Request, indexName string, action batchAction) (string, error) {
	objectID, _ := action.Body["objectID"].(string)

	switch strings.ToLower(action.Action) {
	case "addobject":
		if objectID == "" {
			objectID = h.store.Clock.Now().UTC().Format("20060102150405") + fmt.Sprintf("%d", h.store.NextTaskID())
		}
		attrs := copyWithout(action.Body, "objectID")
		rec := &search.Record{ObjectID: objectID, Attributes: attrs}
		return objectID, h.store.Engine.SaveRecord(r.Context(), indexName, rec)

	case "updateobject":
		if objectID == "" {
			return "", fmt.Errorf("objectID is required for updateObject")
		}
		attrs := copyWithout(action.Body, "objectID")
		rec := &search.Record{ObjectID: objectID, Attributes: attrs}
		return objectID, h.store.Engine.SaveRecord(r.Context(), indexName, rec)

	case "partialupdateobject", "partialupdateobjectnocreate":
		if objectID == "" {
			return "", fmt.Errorf("objectID is required for partialUpdateObject")
		}
		existing, err := h.store.Engine.GetRecord(indexName, objectID)
		if err != nil {
			if strings.ToLower(action.Action) == "partialupdateobjectnocreate" {
				return objectID, nil // skip if doesn't exist
			}
			existing = &search.Record{ObjectID: objectID, Attributes: make(map[string]any)}
		}
		for k, v := range action.Body {
			if k != "objectID" {
				existing.Attributes[k] = v
			}
		}
		return objectID, h.store.Engine.SaveRecord(r.Context(), indexName, existing)

	case "deleteobject":
		if objectID == "" {
			return "", fmt.Errorf("objectID is required for deleteObject")
		}
		return objectID, h.store.Engine.DeleteRecord(r.Context(), indexName, objectID)

	case "clear":
		return "", h.store.Engine.ClearIndex(indexName)

	default:
		return "", fmt.Errorf("unknown batch action: %s", action.Action)
	}
}

// copyWithout returns a shallow copy of the map without the given key.
func copyWithout(m map[string]any, key string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if k != key {
			out[k] = v
		}
	}
	return out
}
