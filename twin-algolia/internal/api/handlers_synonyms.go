package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"

	"github.com/wondertwin-ai/wondertwin/twin-algolia/internal/store"
)

// SaveSynonym handles PUT /1/indexes/{indexName}/synonyms/{synonymID}
func (h *Handler) SaveSynonym(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")
	synonymID := chi.URLParam(r, "synonymID")

	var syn store.Synonym
	if err := readJSON(r, &syn); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}
	syn.ObjectID = synonymID

	h.store.Synonyms.Set(store.SynonymKey(indexName, synonymID), syn)

	resp := h.store.TaskResult(indexName)
	resp["id"] = synonymID
	twincore.JSON(w, http.StatusOK, resp)
}

// BatchSynonyms handles POST /1/indexes/{indexName}/synonyms/batch
func (h *Handler) BatchSynonyms(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	var synonyms []store.Synonym
	if err := readJSON(r, &synonyms); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	for _, syn := range synonyms {
		if syn.ObjectID == "" {
			algoliaError(w, http.StatusBadRequest, "objectID is required for each synonym")
			return
		}
		h.store.Synonyms.Set(store.SynonymKey(indexName, syn.ObjectID), syn)
	}

	twincore.JSON(w, http.StatusOK, h.store.TaskResult(indexName))
}

// GetSynonym handles GET /1/indexes/{indexName}/synonyms/{synonymID}
func (h *Handler) GetSynonym(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")
	synonymID := chi.URLParam(r, "synonymID")

	syn, ok := h.store.Synonyms.Get(store.SynonymKey(indexName, synonymID))
	if !ok {
		algoliaError(w, http.StatusNotFound, "Synonym not found")
		return
	}

	twincore.JSON(w, http.StatusOK, syn)
}

// SearchSynonyms handles POST /1/indexes/{indexName}/synonyms/search
func (h *Handler) SearchSynonyms(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	var body struct {
		Query       string `json:"query"`
		Type        string `json:"type,omitempty"`
		Page        int    `json:"page"`
		HitsPerPage int    `json:"hitsPerPage"`
	}
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	all := h.store.ListIndexSynonyms(indexName)
	query := strings.ToLower(body.Query)

	var hits []store.Synonym
	for _, syn := range all {
		if body.Type != "" && syn.Type != body.Type {
			continue
		}
		if query != "" && !synonymMatches(syn, query) {
			continue
		}
		hits = append(hits, syn)
	}

	hitsPerPage := body.HitsPerPage
	if hitsPerPage <= 0 {
		hitsPerPage = 20
	}

	total := len(hits)
	start := body.Page * hitsPerPage
	end := start + hitsPerPage
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"hits":    hits[start:end],
		"nbHits":  total,
		"page":    body.Page,
		"nbPages": (total + hitsPerPage - 1) / max(hitsPerPage, 1),
	})
}

// DeleteSynonym handles DELETE /1/indexes/{indexName}/synonyms/{synonymID}
func (h *Handler) DeleteSynonym(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")
	synonymID := chi.URLParam(r, "synonymID")

	key := store.SynonymKey(indexName, synonymID)
	if !h.store.Synonyms.Delete(key) {
		algoliaError(w, http.StatusNotFound, "Synonym not found")
		return
	}

	twincore.JSON(w, http.StatusOK, h.store.TaskResult(indexName))
}

// ClearSynonyms handles POST /1/indexes/{indexName}/synonyms/clear
func (h *Handler) ClearSynonyms(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	// Delete all synonyms for this index.
	syns := h.store.ListIndexSynonyms(indexName)
	for _, syn := range syns {
		h.store.Synonyms.Delete(store.SynonymKey(indexName, syn.ObjectID))
	}

	twincore.JSON(w, http.StatusOK, h.store.TaskResult(indexName))
}

// synonymMatches checks if a synonym matches a search query.
func synonymMatches(syn store.Synonym, query string) bool {
	if strings.Contains(strings.ToLower(syn.ObjectID), query) {
		return true
	}
	for _, s := range syn.Synonyms {
		if strings.Contains(strings.ToLower(s), query) {
			return true
		}
	}
	if strings.Contains(strings.ToLower(syn.Input), query) {
		return true
	}
	if strings.Contains(strings.ToLower(syn.Word), query) {
		return true
	}
	return false
}
