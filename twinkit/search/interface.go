// Package search provides a search index engine for twins that simulate
// search platforms like Algolia, Elasticsearch, and Typesense. It handles
// index management, record CRUD, full-text search with relevance ranking,
// and faceted filtering.
package search

// Index represents a search index with its configuration.
type Index struct {
	Name                 string   `json:"name"`
	SearchableAttributes []string `json:"searchableAttributes,omitempty"`
	AttributesForFaceting []string `json:"attributesForFaceting,omitempty"`
	CustomRanking        []string `json:"customRanking,omitempty"`
	RecordCount          int      `json:"recordCount"`
}

// Record is a single document in a search index.
// ObjectID is the unique identifier. All other fields are stored
// in the Attributes map.
type Record struct {
	ObjectID   string         `json:"objectID"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// SearchParams defines the parameters for a search query.
type SearchParams struct {
	Query                string   `json:"query"`
	Filters              string   `json:"filters,omitempty"`             // filter expression: "category:electronics AND price < 100"
	FacetFilters         []string `json:"facetFilters,omitempty"`        // ["category:electronics", "brand:apple"]
	Facets               []string `json:"facets,omitempty"`              // attributes to compute facet counts for
	Page                 int      `json:"page"`
	HitsPerPage          int      `json:"hitsPerPage"`
	AttributesToRetrieve []string `json:"attributesToRetrieve,omitempty"` // limit returned attributes
}

// SearchResult is the response from a search query.
type SearchResult struct {
	Hits        []Hit                     `json:"hits"`
	NbHits      int                       `json:"nbHits"`
	Page        int                       `json:"page"`
	NbPages     int                       `json:"nbPages"`
	HitsPerPage int                       `json:"hitsPerPage"`
	Facets      map[string]map[string]int `json:"facets,omitempty"` // attribute → value → count
	Query       string                    `json:"query"`
}

// Hit is a single search result with highlighting metadata.
type Hit struct {
	ObjectID        string                    `json:"objectID"`
	Attributes      map[string]any            `json:"attributes,omitempty"`
	HighlightResult map[string]HighlightField `json:"_highlightResult,omitempty"`
}

// HighlightField contains the highlighted value for a single attribute.
type HighlightField struct {
	Value        string   `json:"value"`
	MatchLevel   string   `json:"matchLevel"` // "none", "partial", "full"
	MatchedWords []string `json:"matchedWords,omitempty"`
}
