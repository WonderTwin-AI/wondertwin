package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// AdminListIndices handles GET /admin/indices — returns all indices with their record counts.
func (h *Handler) AdminListIndices(w http.ResponseWriter, r *http.Request) {
	indices := h.store.Engine.ListIndices()
	items := make([]map[string]any, 0, len(indices))
	for _, idx := range indices {
		items = append(items, map[string]any{
			"name":                 idx.Name,
			"entries":              idx.RecordCount,
			"searchableAttributes": idx.SearchableAttributes,
			"attributesForFaceting": idx.AttributesForFaceting,
			"customRanking":        idx.CustomRanking,
		})
	}
	twincore.JSON(w, http.StatusOK, map[string]any{"indices": items})
}

// GetLogs handles GET /1/logs — returns an empty log list (twin does not persist logs).
func (h *Handler) GetLogs(w http.ResponseWriter, r *http.Request) {
	twincore.JSON(w, http.StatusOK, map[string]any{
		"logs": []any{},
	})
}
