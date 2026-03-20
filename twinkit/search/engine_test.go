package search

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/state"
)

func newTestEngine(opts ...Option) *Engine {
	return NewEngine(append([]Option{WithClock(state.NewClock())}, opts...)...)
}

func ctx() context.Context {
	return context.Background()
}

func seedProducts(t *testing.T, e *Engine) {
	t.Helper()
	e.CreateIndex("products")
	e.SetSettings("products", Index{
		SearchableAttributes:  []string{"name", "description"},
		AttributesForFaceting: []string{"category", "brand"},
	})
	records := []*Record{
		{ObjectID: "1", Attributes: map[string]any{"name": "iPhone 15", "description": "Latest Apple smartphone", "category": "electronics", "brand": "apple", "price": 999}},
		{ObjectID: "2", Attributes: map[string]any{"name": "Galaxy S24", "description": "Samsung flagship phone", "category": "electronics", "brand": "samsung", "price": 899}},
		{ObjectID: "3", Attributes: map[string]any{"name": "MacBook Pro", "description": "Apple laptop for professionals", "category": "computers", "brand": "apple", "price": 2499}},
		{ObjectID: "4", Attributes: map[string]any{"name": "ThinkPad X1", "description": "Lenovo business laptop", "category": "computers", "brand": "lenovo", "price": 1499}},
		{ObjectID: "5", Attributes: map[string]any{"name": "AirPods Pro", "description": "Apple wireless earbuds", "category": "audio", "brand": "apple", "price": 249}},
	}
	e.SaveRecords(ctx(), "products", records)
}

// --- Index Management ---

func TestCreateIndex(t *testing.T) {
	e := newTestEngine()
	idx, err := e.CreateIndex("products")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if idx.Name != "products" {
		t.Errorf("name = %q, want products", idx.Name)
	}
}

func TestCreateIndex_Duplicate(t *testing.T) {
	e := newTestEngine()
	e.CreateIndex("products")
	_, err := e.CreateIndex("products")
	if err == nil {
		t.Error("expected error for duplicate index")
	}
}

func TestDeleteIndex(t *testing.T) {
	e := newTestEngine()
	e.CreateIndex("products")
	if err := e.DeleteIndex("products"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := e.GetIndex("products")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestListIndices(t *testing.T) {
	e := newTestEngine()
	e.CreateIndex("products")
	e.CreateIndex("users")
	indices := e.ListIndices()
	if len(indices) != 2 {
		t.Errorf("expected 2 indices, got %d", len(indices))
	}
}

func TestSetSettings(t *testing.T) {
	e := newTestEngine()
	e.CreateIndex("products")
	idx, err := e.SetSettings("products", Index{
		SearchableAttributes:  []string{"name", "description"},
		AttributesForFaceting: []string{"category"},
	})
	if err != nil {
		t.Fatalf("set settings: %v", err)
	}
	if len(idx.SearchableAttributes) != 2 {
		t.Errorf("searchable = %d, want 2", len(idx.SearchableAttributes))
	}
}

// --- Record Operations ---

func TestSaveAndGetRecord(t *testing.T) {
	e := newTestEngine()
	e.CreateIndex("products")
	err := e.SaveRecord(ctx(), "products", &Record{
		ObjectID:   "1",
		Attributes: map[string]any{"name": "Widget", "price": 9.99},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	r, err := e.GetRecord("products", "1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if r.Attributes["name"] != "Widget" {
		t.Errorf("name = %v, want Widget", r.Attributes["name"])
	}
}

func TestSaveRecord_AutoCreatesIndex(t *testing.T) {
	e := newTestEngine()
	// Save to non-existent index — should auto-create.
	err := e.SaveRecord(ctx(), "new_index", &Record{ObjectID: "1", Attributes: map[string]any{"x": 1}})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	_, err = e.GetIndex("new_index")
	if err != nil {
		t.Error("index should be auto-created")
	}
}

func TestDeleteRecord(t *testing.T) {
	e := newTestEngine()
	e.CreateIndex("products")
	e.SaveRecord(ctx(), "products", &Record{ObjectID: "1", Attributes: map[string]any{"name": "X"}})

	if err := e.DeleteRecord(ctx(), "products", "1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := e.GetRecord("products", "1")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestClearIndex(t *testing.T) {
	e := newTestEngine()
	seedProducts(t, e)

	e.ClearIndex("products")
	records, _ := e.Browse("products")
	if len(records) != 0 {
		t.Errorf("expected 0 records after clear, got %d", len(records))
	}

	// Index itself should still exist.
	_, err := e.GetIndex("products")
	if err != nil {
		t.Error("index should still exist after clear")
	}
}

func TestBrowse(t *testing.T) {
	e := newTestEngine()
	seedProducts(t, e)

	records, err := e.Browse("products")
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	if len(records) != 5 {
		t.Errorf("expected 5 records, got %d", len(records))
	}
}

// --- Search: Text Matching ---

func TestSearch_BasicTextMatch(t *testing.T) {
	e := newTestEngine()
	seedProducts(t, e)

	result, err := e.Search(ctx(), "products", &SearchParams{Query: "apple"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.NbHits != 3 {
		t.Errorf("expected 3 hits for 'apple', got %d", result.NbHits)
	}
}

func TestSearch_MultiWordQuery(t *testing.T) {
	e := newTestEngine()
	seedProducts(t, e)

	result, err := e.Search(ctx(), "products", &SearchParams{Query: "apple laptop"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	// MacBook Pro matches both words, should be first.
	if len(result.Hits) == 0 {
		t.Fatal("expected hits")
	}
	if result.Hits[0].ObjectID != "3" {
		t.Errorf("top hit = %q, want '3' (MacBook Pro matches both 'apple' and 'laptop')", result.Hits[0].ObjectID)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	e := newTestEngine()
	seedProducts(t, e)

	result, err := e.Search(ctx(), "products", &SearchParams{Query: ""})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.NbHits != 5 {
		t.Errorf("empty query should return all, got %d", result.NbHits)
	}
}

func TestSearch_NoMatch(t *testing.T) {
	e := newTestEngine()
	seedProducts(t, e)

	result, _ := e.Search(ctx(), "products", &SearchParams{Query: "nonexistent"})
	if result.NbHits != 0 {
		t.Errorf("expected 0 hits, got %d", result.NbHits)
	}
}

// --- Search: Facet Filters ---

func TestSearch_FacetFilter(t *testing.T) {
	e := newTestEngine()
	seedProducts(t, e)

	result, _ := e.Search(ctx(), "products", &SearchParams{
		Query:        "",
		FacetFilters: []string{"category:electronics"},
	})
	if result.NbHits != 2 {
		t.Errorf("expected 2 electronics, got %d", result.NbHits)
	}
}

func TestSearch_MultipleFacetFilters(t *testing.T) {
	e := newTestEngine()
	seedProducts(t, e)

	result, _ := e.Search(ctx(), "products", &SearchParams{
		Query:        "",
		FacetFilters: []string{"category:electronics", "brand:apple"},
	})
	if result.NbHits != 1 {
		t.Errorf("expected 1 apple electronics, got %d", result.NbHits)
	}
}

// --- Search: Filter String ---

func TestSearch_FilterString(t *testing.T) {
	e := newTestEngine()
	seedProducts(t, e)

	result, _ := e.Search(ctx(), "products", &SearchParams{
		Query:   "",
		Filters: "price < 1000",
	})
	if result.NbHits != 3 {
		t.Errorf("expected 3 products under 1000, got %d", result.NbHits)
	}
}

func TestSearch_FilterStringAND(t *testing.T) {
	e := newTestEngine()
	seedProducts(t, e)

	result, _ := e.Search(ctx(), "products", &SearchParams{
		Query:   "",
		Filters: "category:electronics AND price < 950",
	})
	if result.NbHits != 1 {
		t.Errorf("expected 1 cheap electronics, got %d", result.NbHits)
	}
}

// --- Search: Facet Counts ---

func TestSearch_FacetCounts(t *testing.T) {
	e := newTestEngine()
	seedProducts(t, e)

	result, _ := e.Search(ctx(), "products", &SearchParams{
		Query:  "",
		Facets: []string{"category", "brand"},
	})
	if result.Facets == nil {
		t.Fatal("expected facets")
	}
	if result.Facets["category"]["electronics"] != 2 {
		t.Errorf("electronics count = %d, want 2", result.Facets["category"]["electronics"])
	}
	if result.Facets["brand"]["apple"] != 3 {
		t.Errorf("apple count = %d, want 3", result.Facets["brand"]["apple"])
	}
}

// --- Search: Pagination ---

func TestSearch_Pagination(t *testing.T) {
	e := newTestEngine()
	seedProducts(t, e)

	// Page 0, 2 per page.
	result, _ := e.Search(ctx(), "products", &SearchParams{Query: "", HitsPerPage: 2, Page: 0})
	if len(result.Hits) != 2 {
		t.Errorf("page 0: expected 2 hits, got %d", len(result.Hits))
	}
	if result.NbPages != 3 {
		t.Errorf("nbPages = %d, want 3", result.NbPages)
	}

	// Page 2 (last page).
	result, _ = e.Search(ctx(), "products", &SearchParams{Query: "", HitsPerPage: 2, Page: 2})
	if len(result.Hits) != 1 {
		t.Errorf("page 2: expected 1 hit, got %d", len(result.Hits))
	}

	// Beyond last page.
	result, _ = e.Search(ctx(), "products", &SearchParams{Query: "", HitsPerPage: 2, Page: 10})
	if len(result.Hits) != 0 {
		t.Errorf("beyond last page: expected 0 hits, got %d", len(result.Hits))
	}
}

// --- Search: Attribute Retrieval ---

func TestSearch_AttributesToRetrieve(t *testing.T) {
	e := newTestEngine()
	seedProducts(t, e)

	result, _ := e.Search(ctx(), "products", &SearchParams{
		Query:                "",
		AttributesToRetrieve: []string{"name", "price"},
		HitsPerPage:          1,
	})
	if len(result.Hits) == 0 {
		t.Fatal("expected hits")
	}
	hit := result.Hits[0]
	if _, ok := hit.Attributes["name"]; !ok {
		t.Error("name should be in attributes")
	}
	if _, ok := hit.Attributes["description"]; ok {
		t.Error("description should not be in attributes (not requested)")
	}
}

// --- Search: Highlight ---

func TestSearch_Highlight(t *testing.T) {
	e := newTestEngine()
	seedProducts(t, e)

	result, _ := e.Search(ctx(), "products", &SearchParams{Query: "apple"})
	if len(result.Hits) == 0 {
		t.Fatal("expected hits")
	}
	// At least one hit should have highlight results.
	foundHighlight := false
	for _, hit := range result.Hits {
		if len(hit.HighlightResult) > 0 {
			foundHighlight = true
			for _, hl := range hit.HighlightResult {
				if len(hl.MatchedWords) == 0 {
					t.Error("MatchedWords should not be empty in highlight")
				}
			}
		}
	}
	if !foundHighlight {
		t.Error("expected at least one hit with highlight results")
	}
}

// --- Hooks ---

type testHooks struct {
	NoOpSearchHooks
	saved    int
	deleted  int
	searched int
}

func (h *testHooks) OnRecordSaved(_ context.Context, _ string, _ *Record) error {
	h.saved++
	return nil
}
func (h *testHooks) OnRecordDeleted(_ context.Context, _, _ string) error {
	h.deleted++
	return nil
}
func (h *testHooks) OnSearch(_ context.Context, _ string, _ *SearchParams, _ *SearchResult) error {
	h.searched++
	return nil
}

func TestHooks_CalledOnOperations(t *testing.T) {
	hooks := &testHooks{}
	e := newTestEngine(WithHooks(hooks))
	e.CreateIndex("test")

	e.SaveRecord(ctx(), "test", &Record{ObjectID: "1", Attributes: map[string]any{"x": 1}})
	if hooks.saved != 1 {
		t.Errorf("saved = %d, want 1", hooks.saved)
	}

	e.Search(ctx(), "test", &SearchParams{Query: "x"})
	if hooks.searched != 1 {
		t.Errorf("searched = %d, want 1", hooks.searched)
	}

	e.DeleteRecord(ctx(), "test", "1")
	if hooks.deleted != 1 {
		t.Errorf("deleted = %d, want 1", hooks.deleted)
	}
}

// --- Record Count ---

func TestGetIndex_RecordCount(t *testing.T) {
	e := newTestEngine()
	seedProducts(t, e)

	idx, _ := e.GetIndex("products")
	if idx.RecordCount != 5 {
		t.Errorf("record count = %d, want 5", idx.RecordCount)
	}
}

// --- Thread Safety ---

func TestConcurrentSaveAndSearch(t *testing.T) {
	e := newTestEngine()
	e.CreateIndex("test")

	var wg sync.WaitGroup
	// Concurrent writes.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			e.SaveRecord(ctx(), "test", &Record{
				ObjectID:   fmt.Sprintf("r_%d", n),
				Attributes: map[string]any{"name": fmt.Sprintf("item %d", n)},
			})
		}(i)
	}
	// Concurrent reads.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.Search(ctx(), "test", &SearchParams{Query: "item"})
		}()
	}
	wg.Wait()

	records, _ := e.Browse("test")
	if len(records) != 50 {
		t.Errorf("expected 50 records, got %d", len(records))
	}
}
