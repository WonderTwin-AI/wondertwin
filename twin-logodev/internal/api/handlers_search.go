package api

import (
	"net/http"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-logodev/internal/store"
)

// SearchBrands handles GET /api/v1/search?q=query — searches seeded brands by name/domain/ticker.
func (h *Handler) SearchBrands(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	if q == "" {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"error":   "bad_request",
			"message": "Query parameter 'q' is required.",
		})
		return
	}

	brands := h.store.Brands.List()
	var results []store.Brand
	for _, b := range brands {
		if strings.Contains(strings.ToLower(b.Name), q) ||
			strings.Contains(strings.ToLower(b.Domain), q) ||
			strings.Contains(strings.ToLower(b.Ticker), q) {
			results = append(results, b)
		}
	}

	twincore.JSON(w, http.StatusOK, results)
}
