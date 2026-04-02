package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/search"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// ListIndices handles GET /1/indexes
func (h *Handler) ListIndices(w http.ResponseWriter, r *http.Request) {
	indices := h.store.Engine.ListIndices()
	items := make([]map[string]any, 0, len(indices))
	for _, idx := range indices {
		items = append(items, map[string]any{
			"name":         idx.Name,
			"entries":      idx.RecordCount,
			"dataSize":     0,
			"fileSize":     0,
			"lastBuildTimeS": 0,
			"numberOfPendingTasks": 0,
			"pendingTask":  false,
			"updatedAt":    h.store.Now(),
			"createdAt":    h.store.Now(),
		})
	}
	twincore.JSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"nbPages": 1,
	})
}

// DeleteIndex handles DELETE /1/indexes/{indexName}
func (h *Handler) DeleteIndex(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	if err := h.store.Engine.DeleteIndex(indexName); err != nil {
		algoliaError(w, http.StatusNotFound, err.Error())
		return
	}

	twincore.JSON(w, http.StatusOK, h.store.TaskResult(indexName))
}

// OperateIndex handles POST /1/indexes/{indexName}/operation — copy or move index.
func (h *Handler) OperateIndex(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	var body struct {
		Operation   string `json:"operation"`
		Destination string `json:"destination"`
	}
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	if body.Destination == "" {
		algoliaError(w, http.StatusBadRequest, "destination is required")
		return
	}

	// Browse source records.
	records, err := h.store.Engine.Browse(indexName)
	if err != nil {
		algoliaError(w, http.StatusNotFound, err.Error())
		return
	}

	// Get source settings.
	srcIdx, err := h.store.Engine.GetIndex(indexName)
	if err != nil {
		algoliaError(w, http.StatusNotFound, err.Error())
		return
	}

	// Create or overwrite destination.
	h.store.Engine.DeleteIndex(body.Destination) //nolint:errcheck
	if _, err := h.store.Engine.CreateIndex(body.Destination); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Copy settings.
	if _, err := h.store.Engine.SetSettings(body.Destination, *srcIdx); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Copy records.
	for _, rec := range records {
		cp := &search.Record{
			ObjectID:   rec.ObjectID,
			Attributes: rec.Attributes,
		}
		if err := h.store.Engine.SaveRecord(r.Context(), body.Destination, cp); err != nil {
			algoliaError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	// If move, delete source.
	if body.Operation == "move" {
		h.store.Engine.DeleteIndex(indexName) //nolint:errcheck
	}

	twincore.JSON(w, http.StatusOK, h.store.TaskResult(body.Destination))
}

// GetTaskStatus handles GET /1/indexes/{indexName}/task/{taskID}
func (h *Handler) GetTaskStatus(w http.ResponseWriter, r *http.Request) {
	taskIDStr := chi.URLParam(r, "taskID")
	taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
	if err != nil {
		algoliaError(w, http.StatusBadRequest, "invalid taskID")
		return
	}

	// All tasks in the twin complete immediately.
	twincore.JSON(w, http.StatusOK, map[string]any{
		"status":    "published",
		"taskID":    taskID,
		"updatedAt": h.store.Now(),
	})
}

// GetSettings handles GET /1/indexes/{indexName}/settings
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	idx, err := h.store.Engine.GetIndex(indexName)
	if err != nil {
		algoliaError(w, http.StatusNotFound, err.Error())
		return
	}

	settings := map[string]any{
		"searchableAttributes":  idx.SearchableAttributes,
		"attributesForFaceting": idx.AttributesForFaceting,
		"customRanking":         idx.CustomRanking,
	}
	twincore.JSON(w, http.StatusOK, settings)
}

// SetSettings handles PUT /1/indexes/{indexName}/settings
func (h *Handler) SetSettings(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	var body struct {
		SearchableAttributes  []string `json:"searchableAttributes,omitempty"`
		AttributesForFaceting []string `json:"attributesForFaceting,omitempty"`
		CustomRanking         []string `json:"customRanking,omitempty"`
	}
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	settings := search.Index{
		Name:                  indexName,
		SearchableAttributes:  body.SearchableAttributes,
		AttributesForFaceting: body.AttributesForFaceting,
		CustomRanking:         body.CustomRanking,
	}

	// Auto-create index if it doesn't exist.
	if _, err := h.store.Engine.GetIndex(indexName); err != nil {
		h.store.Engine.CreateIndex(indexName) //nolint:errcheck
	}

	if _, err := h.store.Engine.SetSettings(indexName, settings); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp := h.store.TaskResult(indexName)
	twincore.JSON(w, http.StatusOK, resp)
}
