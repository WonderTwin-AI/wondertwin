// Package api implements the Algolia-compatible HTTP API handlers for the twin.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/quirks"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"

	"github.com/wondertwin-ai/wondertwin/twin-algolia/internal/store"
)

// Handler holds all API handler state.
type Handler struct {
	store   *store.MemoryStore
	mw      *twincore.Middleware
	emitter *telemetry.Emitter
	quirks  *quirks.Engine
}

// NewHandler creates a new API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware, em *telemetry.Emitter, qe *quirks.Engine) *Handler {
	return &Handler{store: s, mw: mw, emitter: em, quirks: qe}
}

// Routes mounts all Algolia API routes and admin extras.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.mw.FaultInjection)
		r.Use(quirks.Middleware(h.quirks))
		r.Use(telemetry.Middleware(h.emitter))
		r.Use(h.authMiddleware)

		// Search
		r.Post("/1/indexes/{indexName}/query", h.Search)
		r.Post("/1/indexes/_multi/queries", h.MultiSearch)
		r.Post("/1/indexes/{indexName}/facets/{facetName}/query", h.SearchFacets)
		r.Post("/1/indexes/{indexName}/browse", h.Browse)

		// Objects
		r.Post("/1/indexes/{indexName}", h.AddRecord)
		r.Put("/1/indexes/{indexName}/{objectID}", h.SaveRecord)
		r.Post("/1/indexes/{indexName}/{objectID}/partial", h.PartialUpdate)
		r.Delete("/1/indexes/{indexName}/{objectID}", h.DeleteRecord)
		r.Get("/1/indexes/{indexName}/{objectID}", h.GetRecord)
		r.Post("/1/indexes/{indexName}/batch", h.BatchWrite)
		r.Post("/1/indexes/_multi/batch", h.MultiBatchWrite)
		r.Post("/1/indexes/{indexName}/deleteByQuery", h.DeleteByQuery)
		r.Post("/1/indexes/{indexName}/clear", h.ClearIndex)
		r.Post("/1/indexes/_multi/objects", h.GetMultipleRecords)

		// Index management
		r.Get("/1/indexes", h.ListIndices)
		r.Delete("/1/indexes/{indexName}", h.DeleteIndex)
		r.Post("/1/indexes/{indexName}/operation", h.OperateIndex)
		r.Get("/1/indexes/{indexName}/task/{taskID}", h.GetTaskStatus)

		// Settings
		r.Get("/1/indexes/{indexName}/settings", h.GetSettings)
		r.Put("/1/indexes/{indexName}/settings", h.SetSettings)

		// Synonyms
		r.Put("/1/indexes/{indexName}/synonyms/{synonymID}", h.SaveSynonym)
		r.Post("/1/indexes/{indexName}/synonyms/batch", h.BatchSynonyms)
		r.Get("/1/indexes/{indexName}/synonyms/{synonymID}", h.GetSynonym)
		r.Post("/1/indexes/{indexName}/synonyms/search", h.SearchSynonyms)
		r.Delete("/1/indexes/{indexName}/synonyms/{synonymID}", h.DeleteSynonym)
		r.Post("/1/indexes/{indexName}/synonyms/clear", h.ClearSynonyms)

		// Rules
		r.Put("/1/indexes/{indexName}/rules/{ruleID}", h.SaveRule)
		r.Post("/1/indexes/{indexName}/rules/batch", h.BatchRules)
		r.Get("/1/indexes/{indexName}/rules/{ruleID}", h.GetRule)
		r.Post("/1/indexes/{indexName}/rules/search", h.SearchRules)
		r.Delete("/1/indexes/{indexName}/rules/{ruleID}", h.DeleteRule)
		r.Post("/1/indexes/{indexName}/rules/clear", h.ClearRules)

		// API Keys
		r.Get("/1/keys", h.ListKeys)
		r.Get("/1/keys/{keyID}", h.GetKey)
		r.Post("/1/keys", h.CreateKey)
		r.Put("/1/keys/{keyID}", h.UpdateKey)
		r.Delete("/1/keys/{keyID}", h.DeleteKey)
		r.Post("/1/keys/{keyID}/restore", h.RestoreKey)

		// Logs
		r.Get("/1/logs", h.GetLogs)
	})

	// Admin extras (no auth)
	r.Get("/admin/indices", h.AdminListIndices)
}

// authMiddleware validates Algolia authentication headers.
func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appID := r.Header.Get("X-Algolia-Application-Id")
		apiKey := r.Header.Get("X-Algolia-API-Key")
		if appID == "" || apiKey == "" {
			algoliaError(w, http.StatusForbidden, "Invalid Application-ID or API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// algoliaError writes an Algolia-style JSON error response.
func algoliaError(w http.ResponseWriter, status int, message string) {
	twincore.JSON(w, status, map[string]any{
		"message": message,
		"status":  status,
	})
}

// readJSON decodes a JSON request body into dst.
func readJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return fmt.Errorf("missing request body")
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}
