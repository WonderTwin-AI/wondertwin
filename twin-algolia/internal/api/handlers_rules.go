package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"

	"github.com/wondertwin-ai/wondertwin/twin-algolia/internal/store"
)

// SaveRule handles PUT /1/indexes/{indexName}/rules/{ruleID}
func (h *Handler) SaveRule(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")
	ruleID := chi.URLParam(r, "ruleID")

	var rule store.Rule
	if err := readJSON(r, &rule); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}
	rule.ObjectID = ruleID

	h.store.Rules.Set(store.RuleKey(indexName, ruleID), rule)

	resp := h.store.TaskResult(indexName)
	resp["id"] = ruleID
	twincore.JSON(w, http.StatusOK, resp)
}

// BatchRules handles POST /1/indexes/{indexName}/rules/batch
func (h *Handler) BatchRules(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	var rules []store.Rule
	if err := readJSON(r, &rules); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	for _, rule := range rules {
		if rule.ObjectID == "" {
			algoliaError(w, http.StatusBadRequest, "objectID is required for each rule")
			return
		}
		h.store.Rules.Set(store.RuleKey(indexName, rule.ObjectID), rule)
	}

	twincore.JSON(w, http.StatusOK, h.store.TaskResult(indexName))
}

// GetRule handles GET /1/indexes/{indexName}/rules/{ruleID}
func (h *Handler) GetRule(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")
	ruleID := chi.URLParam(r, "ruleID")

	rule, ok := h.store.Rules.Get(store.RuleKey(indexName, ruleID))
	if !ok {
		algoliaError(w, http.StatusNotFound, "Rule not found")
		return
	}

	twincore.JSON(w, http.StatusOK, rule)
}

// SearchRules handles POST /1/indexes/{indexName}/rules/search
func (h *Handler) SearchRules(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	var body struct {
		Query       string `json:"query"`
		Page        int    `json:"page"`
		HitsPerPage int    `json:"hitsPerPage"`
	}
	if err := readJSON(r, &body); err != nil {
		algoliaError(w, http.StatusBadRequest, err.Error())
		return
	}

	all := h.store.ListIndexRules(indexName)
	query := strings.ToLower(body.Query)

	var hits []store.Rule
	for _, rule := range all {
		if query != "" && !ruleMatches(rule, query) {
			continue
		}
		hits = append(hits, rule)
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

// DeleteRule handles DELETE /1/indexes/{indexName}/rules/{ruleID}
func (h *Handler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")
	ruleID := chi.URLParam(r, "ruleID")

	key := store.RuleKey(indexName, ruleID)
	if !h.store.Rules.Delete(key) {
		algoliaError(w, http.StatusNotFound, "Rule not found")
		return
	}

	twincore.JSON(w, http.StatusOK, h.store.TaskResult(indexName))
}

// ClearRules handles POST /1/indexes/{indexName}/rules/clear
func (h *Handler) ClearRules(w http.ResponseWriter, r *http.Request) {
	indexName := chi.URLParam(r, "indexName")

	rules := h.store.ListIndexRules(indexName)
	for _, rule := range rules {
		h.store.Rules.Delete(store.RuleKey(indexName, rule.ObjectID))
	}

	twincore.JSON(w, http.StatusOK, h.store.TaskResult(indexName))
}

// ruleMatches checks if a rule matches a search query.
func ruleMatches(rule store.Rule, query string) bool {
	if strings.Contains(strings.ToLower(rule.ObjectID), query) {
		return true
	}
	if strings.Contains(strings.ToLower(rule.Description), query) {
		return true
	}
	for _, cond := range rule.Conditions {
		if strings.Contains(strings.ToLower(cond.Pattern), query) {
			return true
		}
	}
	return false
}
