package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/search"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// algoliaHit converts a search.Hit into a flat Algolia-style hit map.
// Attributes are flattened to the top level alongside objectID and _highlightResult.
func algoliaHit(hit search.Hit) map[string]any {
	m := make(map[string]any, len(hit.Attributes)+2)
	for k, v := range hit.Attributes {
		m[k] = v
	}
	m["objectID"] = hit.ObjectID
	if len(hit.HighlightResult) > 0 {
		m["_highlightResult"] = hit.HighlightResult
	}
	return m
}

// algoliaHits converts a slice of search hits to Algolia format.
func algoliaHits(hits []search.Hit) []map[string]any {
	out := make([]map[string]any, len(hits))
	for i, h := range hits {
		out[i] = algoliaHit(h)
	}
	return out
}

// algoliaSearchResponse builds the full Algolia search response map.
func algoliaSearchResponse(result *search.SearchResult, params string, elapsed time.Duration) map[string]any {
	resp := map[string]any{
		"hits":             algoliaHits(result.Hits),
		"nbHits":           result.NbHits,
		"page":             result.Page,
		"nbPages":          result.NbPages,
		"hitsPerPage":      result.HitsPerPage,
		"query":            result.Query,
		"processingTimeMS": int(elapsed.Milliseconds()),
		"params":           params,
	}
	if result.Facets != nil {
		resp["facets"] = result.Facets
	}
	return resp
}

// buildParams encodes search parameters as an Algolia-style query string.
func buildParams(sp *search.SearchParams) string {
	var parts []string
	if sp.Query != "" {
		parts = append(parts, "query="+sp.Query)
	}
	if sp.HitsPerPage > 0 {
		parts = append(parts, fmt.Sprintf("hitsPerPage=%d", sp.HitsPerPage))
	}
	if sp.Page > 0 {
		parts = append(parts, fmt.Sprintf("page=%d", sp.Page))
	}
	return strings.Join(parts, "&")
}

// Search handles POST /1/indexes/{indexName}/query
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	var body struct {
		Params               string   `json:"params"`
		Query                string   `json:"query"`
		Filters              string   `json:"filters,omitempty"`
		FacetFilters         []string `json:"facetFilters,omitempty"`
		Facets               []string `json:"facets,omitempty"`
		Page                 int      `json:"page"`
		HitsPerPage          int      `json:"hitsPerPage"`
		AttributesToRetrieve []string `json:"attributesToRetrieve,omitempty"`
	}
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	sp := &search.SearchParams{
		Query:                body.Query,
		Filters:              body.Filters,
		FacetFilters:         body.FacetFilters,
		Facets:               body.Facets,
		Page:                 body.Page,
		HitsPerPage:          body.HitsPerPage,
		AttributesToRetrieve: body.AttributesToRetrieve,
	}

	// Algolia clients sometimes encode params as a query string in the "params" field.
	if body.Params != "" && sp.Query == "" {
		sp.Query = parseParamQuery(body.Params)
	}

	start := time.Now()
	result, err := h.store.Engine.Search(r.Context(), indexName, sp)
	elapsed := time.Since(start)
	if err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	twincore.JSON(w, http.StatusOK, algoliaSearchResponse(result, buildParams(sp), elapsed))
}

// MultiSearch handles POST /1/indexes/*/queries
func (h *Handler) MultiSearch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Requests []struct {
			IndexName            string   `json:"indexName"`
			Params               string   `json:"params,omitempty"`
			Query                string   `json:"query,omitempty"`
			Filters              string   `json:"filters,omitempty"`
			FacetFilters         []string `json:"facetFilters,omitempty"`
			Facets               []string `json:"facets,omitempty"`
			Page                 int      `json:"page"`
			HitsPerPage          int      `json:"hitsPerPage"`
			AttributesToRetrieve []string `json:"attributesToRetrieve,omitempty"`
		} `json:"requests"`
	}
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	results := make([]map[string]any, 0, len(body.Requests))
	for _, req := range body.Requests {
		sp := &search.SearchParams{
			Query:                req.Query,
			Filters:              req.Filters,
			FacetFilters:         req.FacetFilters,
			Facets:               req.Facets,
			Page:                 req.Page,
			HitsPerPage:          req.HitsPerPage,
			AttributesToRetrieve: req.AttributesToRetrieve,
		}
		if req.Params != "" && sp.Query == "" {
			sp.Query = parseParamQuery(req.Params)
		}

		start := time.Now()
		result, err := h.store.Engine.Search(r.Context(), req.IndexName, sp)
		elapsed := time.Since(start)
		if err != nil {
			// Algolia returns per-query errors inline.
			results = append(results, map[string]any{
				"index":   req.IndexName,
				"message": err.Error(),
				"status":  400,
			})
			continue
		}
		resp := algoliaSearchResponse(result, buildParams(sp), elapsed)
		resp["index"] = req.IndexName
		results = append(results, resp)
	}

	twincore.JSON(w, http.StatusOK, map[string]any{"results": results})
}

// SearchFacets handles POST /1/indexes/{indexName}/facets/{facetName}/query
func (h *Handler) SearchFacets(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")
	facetName := chi.URLParam(r, "facetName")

	var body struct {
		Params       string `json:"params,omitempty"`
		FacetQuery   string `json:"facetQuery,omitempty"`
		MaxFacetHits int    `json:"maxFacetHits,omitempty"`
	}
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Search all records in the index to compute facet values.
	sp := &search.SearchParams{
		Facets: []string{facetName},
	}
	result, err := h.store.Engine.Search(r.Context(), indexName, sp)
	if err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	facetQuery := strings.ToLower(body.FacetQuery)
	maxHits := body.MaxFacetHits
	if maxHits <= 0 {
		maxHits = 10
	}

	var facetHits []map[string]any
	if result.Facets != nil {
		if counts, ok := result.Facets[facetName]; ok {
			for value, count := range counts {
				if facetQuery != "" && !strings.Contains(strings.ToLower(value), facetQuery) {
					continue
				}
				facetHits = append(facetHits, map[string]any{
					"value":     value,
					"count":     count,
					"highlighted": value,
				})
				if len(facetHits) >= maxHits {
					break
				}
			}
		}
	}
	if facetHits == nil {
		facetHits = []map[string]any{}
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"facetHits":        facetHits,
		"exhaustiveFacetsCount": true,
		"processingTimeMS": 1,
	})
}

// Browse handles POST /1/indexes/{indexName}/browse
func (h *Handler) Browse(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	records, err := h.store.Engine.Browse(indexName)
	if err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	hits := make([]map[string]any, 0, len(records))
	for _, rec := range records {
		m := make(map[string]any, len(rec.Attributes)+1)
		for k, v := range rec.Attributes {
			m[k] = v
		}
		m["objectID"] = rec.ObjectID
		hits = append(hits, m)
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"hits":             hits,
		"nbHits":           len(hits),
		"page":             0,
		"nbPages":          1,
		"hitsPerPage":      1000,
		"processingTimeMS": 1,
		"query":            "",
		"params":           "",
	})
}

// parseParamQuery extracts the "query" value from an Algolia-style params string.
func parseParamQuery(params string) string {
	for _, part := range strings.Split(params, "&") {
		if strings.HasPrefix(part, "query=") {
			return strings.TrimPrefix(part, "query=")
		}
	}
	return ""
}
