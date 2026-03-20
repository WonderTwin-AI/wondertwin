package search

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// Engine manages search indices, records, and queries.
type Engine struct {
	mu      sync.RWMutex
	indices map[string]*indexData
	hooks   SearchHooks
	clock   *state.Clock
}

// indexData holds the internal state of an index.
type indexData struct {
	config  Index
	records map[string]*Record // objectID → record
	order   []string           // insertion order
}

// Option configures the Engine.
type Option func(*Engine)

// WithHooks sets the search hooks implementation.
func WithHooks(h SearchHooks) Option {
	return func(e *Engine) { e.hooks = h }
}

// WithClock sets the clock for timestamps.
func WithClock(c *state.Clock) Option {
	return func(e *Engine) { e.clock = c }
}

// NewEngine creates a new search engine with the given options.
func NewEngine(opts ...Option) *Engine {
	e := &Engine{
		indices: make(map[string]*indexData),
	}
	for _, opt := range opts {
		opt(e)
	}
	if e.clock == nil {
		e.clock = state.NewClock()
	}
	return e
}

// --- Index Management ---

// CreateIndex creates a new search index.
func (e *Engine) CreateIndex(name string) (*Index, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("index name is required")
	}
	if _, ok := e.indices[name]; ok {
		return nil, fmt.Errorf("index %q already exists", name)
	}

	idx := &indexData{
		config:  Index{Name: name},
		records: make(map[string]*Record),
		order:   make([]string, 0),
	}
	e.indices[name] = idx
	return &idx.config, nil
}

// GetIndex returns an index by name.
func (e *Engine) GetIndex(name string) (*Index, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	idx, ok := e.indices[name]
	if !ok {
		return nil, fmt.Errorf("index %q not found", name)
	}
	cfg := idx.config
	cfg.RecordCount = len(idx.records)
	return &cfg, nil
}

// DeleteIndex removes an index and all its records.
func (e *Engine) DeleteIndex(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.indices[name]; !ok {
		return fmt.Errorf("index %q not found", name)
	}
	delete(e.indices, name)
	return nil
}

// ListIndices returns all indices.
func (e *Engine) ListIndices() []Index {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]Index, 0, len(e.indices))
	for _, idx := range e.indices {
		cfg := idx.config
		cfg.RecordCount = len(idx.records)
		result = append(result, cfg)
	}
	return result
}

// SetSettings updates the configuration for an index.
func (e *Engine) SetSettings(name string, settings Index) (*Index, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	idx, ok := e.indices[name]
	if !ok {
		return nil, fmt.Errorf("index %q not found", name)
	}
	if len(settings.SearchableAttributes) > 0 {
		idx.config.SearchableAttributes = settings.SearchableAttributes
	}
	if len(settings.AttributesForFaceting) > 0 {
		idx.config.AttributesForFaceting = settings.AttributesForFaceting
	}
	if len(settings.CustomRanking) > 0 {
		idx.config.CustomRanking = settings.CustomRanking
	}
	cfg := idx.config
	cfg.RecordCount = len(idx.records)
	return &cfg, nil
}

// --- Record Operations ---

// SaveRecord adds or updates a record in an index.
func (e *Engine) SaveRecord(ctx context.Context, indexName string, record *Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	idx, ok := e.indices[indexName]
	if !ok {
		// Auto-create index (Algolia behavior).
		idx = &indexData{
			config:  Index{Name: indexName},
			records: make(map[string]*Record),
			order:   make([]string, 0),
		}
		e.indices[indexName] = idx
	}

	if record.ObjectID == "" {
		return fmt.Errorf("objectID is required")
	}
	if record.Attributes == nil {
		record.Attributes = make(map[string]any)
	}

	// Validate via hooks.
	if e.hooks != nil {
		if err := e.hooks.ValidateRecord(ctx, indexName, record); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}

	if _, exists := idx.records[record.ObjectID]; !exists {
		idx.order = append(idx.order, record.ObjectID)
	}
	idx.records[record.ObjectID] = record

	if e.hooks != nil {
		if err := e.hooks.OnRecordSaved(ctx, indexName, record); err != nil {
			return fmt.Errorf("hook OnRecordSaved: %w", err)
		}
	}

	return nil
}

// SaveRecords adds or updates multiple records in batch.
func (e *Engine) SaveRecords(ctx context.Context, indexName string, records []*Record) error {
	for _, r := range records {
		if err := e.SaveRecord(ctx, indexName, r); err != nil {
			return err
		}
	}
	return nil
}

// GetRecord returns a record by objectID.
func (e *Engine) GetRecord(indexName, objectID string) (*Record, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	idx, ok := e.indices[indexName]
	if !ok {
		return nil, fmt.Errorf("index %q not found", indexName)
	}
	r, ok := idx.records[objectID]
	if !ok {
		return nil, fmt.Errorf("record %q not found in index %q", objectID, indexName)
	}
	return r, nil
}

// DeleteRecord removes a record by objectID.
func (e *Engine) DeleteRecord(ctx context.Context, indexName, objectID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	idx, ok := e.indices[indexName]
	if !ok {
		return fmt.Errorf("index %q not found", indexName)
	}
	if _, ok := idx.records[objectID]; !ok {
		return fmt.Errorf("record %q not found in index %q", objectID, indexName)
	}
	delete(idx.records, objectID)
	for i, id := range idx.order {
		if id == objectID {
			idx.order = append(idx.order[:i], idx.order[i+1:]...)
			break
		}
	}

	if e.hooks != nil {
		if err := e.hooks.OnRecordDeleted(ctx, indexName, objectID); err != nil {
			return fmt.Errorf("hook OnRecordDeleted: %w", err)
		}
	}

	return nil
}

// ClearIndex removes all records from an index without deleting the index.
func (e *Engine) ClearIndex(indexName string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	idx, ok := e.indices[indexName]
	if !ok {
		return fmt.Errorf("index %q not found", indexName)
	}
	idx.records = make(map[string]*Record)
	idx.order = make([]string, 0)
	return nil
}

// Browse returns all records in an index (for full iteration).
func (e *Engine) Browse(indexName string) ([]*Record, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	idx, ok := e.indices[indexName]
	if !ok {
		return nil, fmt.Errorf("index %q not found", indexName)
	}
	result := make([]*Record, 0, len(idx.order))
	for _, id := range idx.order {
		result = append(result, idx.records[id])
	}
	return result, nil
}

// --- Search ---

// scored pairs a record with its relevance score and matched words.
type scored struct {
	record       *Record
	score        float64
	matchedWords []string
}

// Search executes a search query against an index.
func (e *Engine) Search(ctx context.Context, indexName string, params *SearchParams) (*SearchResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	idx, ok := e.indices[indexName]
	if !ok {
		return nil, fmt.Errorf("index %q not found", indexName)
	}

	// Defaults.
	hitsPerPage := params.HitsPerPage
	if hitsPerPage <= 0 {
		hitsPerPage = 20
	}
	page := params.Page
	if page < 0 {
		page = 0
	}

	query := strings.ToLower(strings.TrimSpace(params.Query))
	queryWords := strings.Fields(query)

	// Determine searchable attributes.
	searchableAttrs := idx.config.SearchableAttributes

	// Score and filter all records.
	var candidates []scored

	for _, id := range idx.order {
		record := idx.records[id]

		// Apply facet filters.
		if !matchesFacetFilters(record, params.FacetFilters) {
			continue
		}

		// Apply filter string.
		if params.Filters != "" && !matchesFilterString(record, params.Filters) {
			continue
		}

		// Compute text relevance score.
		if query == "" {
			// Empty query returns all (filtered) records.
			candidates = append(candidates, scored{record: record, score: 0})
			continue
		}

		score, matched := computeRelevance(record, queryWords, searchableAttrs)
		if score > 0 {
			candidates = append(candidates, scored{record: record, score: score, matchedWords: matched})
		}
	}

	// Sort by score descending.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	totalHits := len(candidates)
	nbPages := 1
	if hitsPerPage > 0 {
		nbPages = int(math.Ceil(float64(totalHits) / float64(hitsPerPage)))
	}

	// Paginate.
	start := page * hitsPerPage
	end := start + hitsPerPage
	if start > len(candidates) {
		start = len(candidates)
	}
	if end > len(candidates) {
		end = len(candidates)
	}
	paginated := candidates[start:end]

	// Build hits with highlight info.
	hits := make([]Hit, 0, len(paginated))
	for _, c := range paginated {
		hit := Hit{
			ObjectID:   c.record.ObjectID,
			Attributes: filterAttributes(c.record.Attributes, params.AttributesToRetrieve),
		}
		if len(c.matchedWords) > 0 {
			hit.HighlightResult = buildHighlight(c.record, c.matchedWords, searchableAttrs)
		}
		hits = append(hits, hit)
	}

	// Compute facets.
	var facets map[string]map[string]int
	if len(params.Facets) > 0 {
		facets = computeFacets(candidates, params.Facets)
	}

	result := &SearchResult{
		Hits:        hits,
		NbHits:      totalHits,
		Page:        page,
		NbPages:     nbPages,
		HitsPerPage: hitsPerPage,
		Facets:      facets,
		Query:       params.Query,
	}

	if e.hooks != nil {
		if err := e.hooks.OnSearch(ctx, indexName, params, result); err != nil {
			return nil, fmt.Errorf("hook OnSearch: %w", err)
		}
	}

	return result, nil
}

// --- Internal helpers ---

// computeRelevance scores a record against query words.
// Returns 0 if no match.
func computeRelevance(record *Record, queryWords, searchableAttrs []string) (float64, []string) {
	if len(queryWords) == 0 {
		return 0, nil
	}

	var totalScore float64
	var matchedWords []string
	seen := make(map[string]bool)

	for _, word := range queryWords {
		wordMatched := false
		for _, attr := range getSearchableValues(record, searchableAttrs) {
			lowerVal := strings.ToLower(fmt.Sprintf("%v", attr))
			if strings.Contains(lowerVal, word) {
				totalScore += 1.0
				wordMatched = true
				// Exact word match scores higher.
				for _, valWord := range strings.Fields(lowerVal) {
					if valWord == word {
						totalScore += 1.0
						break
					}
				}
				break // only count each query word once per record
			}
		}
		if wordMatched && !seen[word] {
			matchedWords = append(matchedWords, word)
			seen[word] = true
		}
	}

	// Bonus: all query words matched.
	if len(matchedWords) == len(queryWords) {
		totalScore += 2.0
	}

	return totalScore, matchedWords
}

// getSearchableValues extracts string values from a record for search.
// If searchableAttrs is configured, only those are searched.
// Otherwise all string-valued attributes are searched.
func getSearchableValues(record *Record, searchableAttrs []string) []any {
	if len(searchableAttrs) > 0 {
		var vals []any
		for _, attr := range searchableAttrs {
			if v, ok := record.Attributes[attr]; ok {
				vals = append(vals, v)
			}
		}
		return vals
	}
	// Search all attributes.
	vals := make([]any, 0, len(record.Attributes))
	for _, v := range record.Attributes {
		vals = append(vals, v)
	}
	return vals
}

// matchesFacetFilters checks if a record matches all facet filters.
// Facet filters are in the format "attribute:value".
func matchesFacetFilters(record *Record, filters []string) bool {
	for _, f := range filters {
		parts := strings.SplitN(f, ":", 2)
		if len(parts) != 2 {
			continue
		}
		attr, val := parts[0], parts[1]
		recVal, ok := record.Attributes[attr]
		if !ok || fmt.Sprintf("%v", recVal) != val {
			return false
		}
	}
	return true
}

// matchesFilterString evaluates a simple filter expression.
// Supports: "attribute:value", "attribute > N", "attribute < N", "AND" combinations.
func matchesFilterString(record *Record, filter string) bool {
	// Split on AND.
	clauses := strings.Split(filter, " AND ")
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}

		// Try "attr:value" format.
		if strings.Contains(clause, ":") && !strings.Contains(clause, " ") {
			parts := strings.SplitN(clause, ":", 2)
			attr, val := parts[0], parts[1]
			recVal, ok := record.Attributes[attr]
			if !ok || fmt.Sprintf("%v", recVal) != val {
				return false
			}
			continue
		}

		// Try "attr > N" or "attr < N" etc.
		for _, op := range []string{">=", "<=", ">", "<", "="} {
			if strings.Contains(clause, " "+op+" ") {
				parts := strings.SplitN(clause, " "+op+" ", 2)
				if len(parts) == 2 {
					attr := strings.TrimSpace(parts[0])
					valStr := strings.TrimSpace(parts[1])
					recVal, ok := record.Attributes[attr]
					if !ok {
						return false
					}
					rf, rok := toFloat(recVal)
					vf, vok := toFloat(valStr)
					if rok && vok {
						switch op {
						case ">":
							if !(rf > vf) {
								return false
							}
						case "<":
							if !(rf < vf) {
								return false
							}
						case ">=":
							if !(rf >= vf) {
								return false
							}
						case "<=":
							if !(rf <= vf) {
								return false
							}
						case "=":
							if rf != vf {
								return false
							}
						}
					} else if op == "=" {
						if fmt.Sprintf("%v", recVal) != valStr {
							return false
						}
					} else {
						return false
					}
				}
				break
			}
		}
	}
	return true
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		var f float64
		_, err := fmt.Sscanf(n, "%f", &f)
		return f, err == nil
	default:
		return 0, false
	}
}

// computeFacets counts attribute values across matched records.
func computeFacets(candidates []scored, facetAttrs []string) map[string]map[string]int {
	facets := make(map[string]map[string]int)
	for _, attr := range facetAttrs {
		facets[attr] = make(map[string]int)
	}
	for _, c := range candidates {
		for _, attr := range facetAttrs {
			if val, ok := c.record.Attributes[attr]; ok {
				facets[attr][fmt.Sprintf("%v", val)]++
			}
		}
	}
	return facets
}

// filterAttributes limits returned attributes if specified.
func filterAttributes(attrs map[string]any, retrieve []string) map[string]any {
	if len(retrieve) == 0 {
		return attrs
	}
	result := make(map[string]any, len(retrieve))
	for _, key := range retrieve {
		if v, ok := attrs[key]; ok {
			result[key] = v
		}
	}
	return result
}

// buildHighlight creates highlight results for matched attributes.
func buildHighlight(record *Record, matchedWords, searchableAttrs []string) map[string]HighlightField {
	result := make(map[string]HighlightField)
	attrs := searchableAttrs
	if len(attrs) == 0 {
		attrs = make([]string, 0, len(record.Attributes))
		for k := range record.Attributes {
			attrs = append(attrs, k)
		}
	}
	for _, attr := range attrs {
		val, ok := record.Attributes[attr]
		if !ok {
			continue
		}
		strVal := fmt.Sprintf("%v", val)
		lowerVal := strings.ToLower(strVal)
		var hits []string
		for _, w := range matchedWords {
			if strings.Contains(lowerVal, w) {
				hits = append(hits, w)
			}
		}
		if len(hits) > 0 {
			matchLevel := "partial"
			if len(hits) == len(matchedWords) {
				matchLevel = "full"
			}
			result[attr] = HighlightField{
				Value:        strVal,
				MatchLevel:   matchLevel,
				MatchedWords: hits,
			}
		}
	}
	return result
}
