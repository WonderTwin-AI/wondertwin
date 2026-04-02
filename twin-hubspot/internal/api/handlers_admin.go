package api

import (
	"net/http"
)

// AdminListObjects handles GET /admin/objects
func (h *Handler) AdminListObjects(w http.ResponseWriter, r *http.Request) {
	all := h.store.Objects.List()
	hsJSON(w, 200, map[string]any{
		"results": all,
		"total":   len(all),
	})
}
