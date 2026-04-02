package api

import (
	"net/http"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-logodev/internal/store"
)

// SearchBrands handles GET /api/v1/search?q=query — searches seeded brands.
// Supports strategy param: "typeahead" (default, prefix matching) or "match" (contains).
func (h *Handler) SearchBrands(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	if q == "" {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"error":   "bad_request",
			"message": "Query parameter 'q' is required.",
		})
		return
	}

	strategy := r.URL.Query().Get("strategy")
	if strategy == "" {
		strategy = "typeahead"
	}

	brands := h.store.Brands.List()
	var results []store.Brand

	for _, b := range brands {
		fields := []string{
			strings.ToLower(b.Name),
			strings.ToLower(b.Domain),
			strings.ToLower(b.Ticker),
			strings.ToLower(b.Crypto),
			strings.ToLower(b.ISIN),
		}

		var matched bool
		for _, f := range fields {
			if f == "" {
				continue
			}
			if strategy == "typeahead" {
				matched = matched || strings.HasPrefix(f, q)
			} else {
				matched = matched || strings.Contains(f, q)
			}
		}

		if matched {
			results = append(results, b)
		}
	}

	// Logo.dev returns max 10 results
	if len(results) > 10 {
		results = results[:10]
	}

	twincore.JSON(w, http.StatusOK, results)
}
