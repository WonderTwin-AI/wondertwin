package store

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// testItem is a simple struct used throughout store tests.
type testItem struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

// ---------------------------------------------------------------------------
// Store[T] – basic CRUD
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	s := New[testItem]("acct")
	if s == nil {
		t.Fatal("expected non-nil store")
	}
	if s.Count() != 0 {
		t.Errorf("expected empty store, got count %d", s.Count())
	}
}

func TestNextID(t *testing.T) {
	s := New[testItem]("acct")
	id1 := s.NextID()
	id2 := s.NextID()

	if id1 != "acct_000001" {
		t.Errorf("expected acct_000001, got %s", id1)
	}
	if id2 != "acct_000002" {
		t.Errorf("expected acct_000002, got %s", id2)
	}
}

func TestSetAndGet(t *testing.T) {
	s := New[testItem]("item")
	item := testItem{Name: "alpha", Value: 1}
	s.Set("item_000001", item)

	got, ok := s.Get("item_000001")
	if !ok {
		t.Fatal("expected item to be found")
	}
	if got.Name != "alpha" || got.Value != 1 {
		t.Errorf("unexpected item: %+v", got)
	}
}

func TestGetMissing(t *testing.T) {
	s := New[testItem]("item")
	_, ok := s.Get("nonexistent")
	if ok {
		t.Error("expected ok=false for missing item")
	}
}

func TestSetOverwrite(t *testing.T) {
	s := New[testItem]("item")
	s.Set("id1", testItem{Name: "first", Value: 1})
	s.Set("id1", testItem{Name: "second", Value: 2})

	got, _ := s.Get("id1")
	if got.Name != "second" {
		t.Errorf("expected overwritten item, got %+v", got)
	}
	// Overwrite should not add a duplicate entry to order.
	if s.Count() != 1 {
		t.Errorf("expected count 1 after overwrite, got %d", s.Count())
	}
}

func TestDelete(t *testing.T) {
	s := New[testItem]("item")
	s.Set("id1", testItem{Name: "a", Value: 1})

	if !s.Delete("id1") {
		t.Error("expected Delete to return true for existing item")
	}
	if s.Delete("id1") {
		t.Error("expected Delete to return false for already-deleted item")
	}
	if s.Count() != 0 {
		t.Errorf("expected empty store after delete, got count %d", s.Count())
	}
}

func TestDeleteNonExistent(t *testing.T) {
	s := New[testItem]("item")
	if s.Delete("nope") {
		t.Error("expected Delete to return false for non-existent item")
	}
}

// ---------------------------------------------------------------------------
// Listing
// ---------------------------------------------------------------------------

func TestListAndListIDs(t *testing.T) {
	s := New[testItem]("item")
	s.Set("a", testItem{Name: "alpha", Value: 1})
	s.Set("b", testItem{Name: "beta", Value: 2})
	s.Set("c", testItem{Name: "gamma", Value: 3})

	items := s.List()
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	// insertion order
	if items[0].Name != "alpha" || items[1].Name != "beta" || items[2].Name != "gamma" {
		t.Errorf("unexpected list order: %+v", items)
	}

	ids := s.ListIDs()
	if len(ids) != 3 {
		t.Fatalf("expected 3 ids, got %d", len(ids))
	}
	if ids[0] != "a" || ids[1] != "b" || ids[2] != "c" {
		t.Errorf("unexpected id order: %v", ids)
	}
}

func TestListIDsReturnsCopy(t *testing.T) {
	s := New[testItem]("item")
	s.Set("a", testItem{Name: "a", Value: 1})

	ids := s.ListIDs()
	ids[0] = "mutated"

	original := s.ListIDs()
	if original[0] != "a" {
		t.Error("ListIDs did not return a copy; mutation affected the store")
	}
}

// ---------------------------------------------------------------------------
// Pagination
// ---------------------------------------------------------------------------

func TestPaginateAll(t *testing.T) {
	s := New[testItem]("item")
	for i := 0; i < 5; i++ {
		s.Set(s.NextID(), testItem{Name: "item", Value: i})
	}

	page := s.Paginate("", 0)
	if len(page.Data) != 5 {
		t.Errorf("expected 5 items, got %d", len(page.Data))
	}
	if page.HasMore {
		t.Error("expected HasMore=false when returning all")
	}
	if page.Total != 5 {
		t.Errorf("expected Total=5, got %d", page.Total)
	}
}

func TestPaginateWithLimit(t *testing.T) {
	s := New[testItem]("item")
	for i := 0; i < 5; i++ {
		s.Set(s.NextID(), testItem{Name: "item", Value: i})
	}

	page1 := s.Paginate("", 2)
	if len(page1.Data) != 2 {
		t.Fatalf("expected 2 items in first page, got %d", len(page1.Data))
	}
	if !page1.HasMore {
		t.Error("expected HasMore=true on first page")
	}
	if page1.Cursor == "" {
		t.Error("expected non-empty cursor")
	}

	page2 := s.Paginate(page1.Cursor, 2)
	if len(page2.Data) != 2 {
		t.Fatalf("expected 2 items in second page, got %d", len(page2.Data))
	}
	if !page2.HasMore {
		t.Error("expected HasMore=true on second page")
	}

	page3 := s.Paginate(page2.Cursor, 2)
	if len(page3.Data) != 1 {
		t.Fatalf("expected 1 item in third page, got %d", len(page3.Data))
	}
	if page3.HasMore {
		t.Error("expected HasMore=false on last page")
	}
}

func TestPaginateEmptyStore(t *testing.T) {
	s := New[testItem]("item")
	page := s.Paginate("", 10)
	if len(page.Data) != 0 {
		t.Errorf("expected 0 items, got %d", len(page.Data))
	}
	if page.HasMore {
		t.Error("expected HasMore=false for empty store")
	}
}

// ---------------------------------------------------------------------------
// Filter / FilterWithIDs
// ---------------------------------------------------------------------------

func TestFilter(t *testing.T) {
	s := New[testItem]("item")
	s.Set("a", testItem{Name: "alpha", Value: 10})
	s.Set("b", testItem{Name: "beta", Value: 20})
	s.Set("c", testItem{Name: "gamma", Value: 30})

	result := s.Filter(func(id string, item testItem) bool {
		return item.Value >= 20
	})
	if len(result) != 2 {
		t.Fatalf("expected 2 filtered items, got %d", len(result))
	}
	if result[0].Name != "beta" || result[1].Name != "gamma" {
		t.Errorf("unexpected filter result: %+v", result)
	}
}

func TestFilterNoMatch(t *testing.T) {
	s := New[testItem]("item")
	s.Set("a", testItem{Name: "alpha", Value: 1})

	result := s.Filter(func(id string, item testItem) bool {
		return item.Value > 100
	})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
}

func TestFilterWithIDs(t *testing.T) {
	s := New[testItem]("item")
	s.Set("a", testItem{Name: "alpha", Value: 10})
	s.Set("b", testItem{Name: "beta", Value: 20})

	ids, items := s.FilterWithIDs(func(id string, item testItem) bool {
		return id == "a"
	})
	if len(ids) != 1 || ids[0] != "a" {
		t.Errorf("expected [a], got %v", ids)
	}
	if len(items) != 1 || items[0].Name != "alpha" {
		t.Errorf("expected [alpha], got %+v", items)
	}
}

// ---------------------------------------------------------------------------
// Snapshot / LoadSnapshot
// ---------------------------------------------------------------------------

func TestSnapshotAndLoadSnapshot(t *testing.T) {
	s := New[testItem]("item")
	s.Set("b", testItem{Name: "beta", Value: 2})
	s.Set("a", testItem{Name: "alpha", Value: 1})

	snap := s.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 items in snapshot, got %d", len(snap))
	}

	s2 := New[testItem]("item")
	s2.LoadSnapshot(snap)
	if s2.Count() != 2 {
		t.Fatalf("expected 2 items after LoadSnapshot, got %d", s2.Count())
	}

	// LoadSnapshot sorts IDs, so order should be deterministic.
	ids := s2.ListIDs()
	if ids[0] != "a" || ids[1] != "b" {
		t.Errorf("expected sorted IDs after LoadSnapshot, got %v", ids)
	}
}

func TestLoadSnapshotReplacesExisting(t *testing.T) {
	s := New[testItem]("item")
	s.Set("old", testItem{Name: "old", Value: 0})

	snap := map[string]testItem{
		"new": {Name: "new", Value: 99},
	}
	s.LoadSnapshot(snap)

	if s.Count() != 1 {
		t.Fatalf("expected 1 item, got %d", s.Count())
	}
	_, ok := s.Get("old")
	if ok {
		t.Error("old item should have been replaced")
	}
	got, ok := s.Get("new")
	if !ok {
		t.Fatal("new item not found")
	}
	if got.Value != 99 {
		t.Errorf("expected Value=99, got %d", got.Value)
	}
}

// ---------------------------------------------------------------------------
// JSON marshaling
// ---------------------------------------------------------------------------

func TestMarshalJSON(t *testing.T) {
	s := New[testItem]("item")
	s.Set("id1", testItem{Name: "alpha", Value: 1})

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	var m map[string]testItem
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if m["id1"].Name != "alpha" {
		t.Errorf("unexpected marshaled value: %+v", m)
	}
}

func TestUnmarshalJSON(t *testing.T) {
	data := []byte(`{"x":{"name":"x-item","value":42}}`)

	s := New[testItem]("item")
	if err := json.Unmarshal(data, s); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}

	got, ok := s.Get("x")
	if !ok {
		t.Fatal("expected item x")
	}
	if got.Name != "x-item" || got.Value != 42 {
		t.Errorf("unexpected item: %+v", got)
	}
}

func TestUnmarshalJSONInvalid(t *testing.T) {
	s := New[testItem]("item")
	if err := json.Unmarshal([]byte(`{bad json`), s); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// ---------------------------------------------------------------------------
// Reset
// ---------------------------------------------------------------------------

func TestReset(t *testing.T) {
	s := New[testItem]("item")
	s.Set("a", testItem{Name: "a", Value: 1})
	_ = s.NextID()
	_ = s.NextID()

	s.Reset()

	if s.Count() != 0 {
		t.Errorf("expected 0 items after reset, got %d", s.Count())
	}
	// Counter should be reset, so next ID starts at 1 again.
	if id := s.NextID(); id != "item_000001" {
		t.Errorf("expected item_000001 after reset, got %s", id)
	}
}

// ---------------------------------------------------------------------------
// Count
// ---------------------------------------------------------------------------

func TestCount(t *testing.T) {
	s := New[testItem]("item")
	if s.Count() != 0 {
		t.Errorf("expected 0, got %d", s.Count())
	}
	s.Set("a", testItem{})
	s.Set("b", testItem{})
	if s.Count() != 2 {
		t.Errorf("expected 2, got %d", s.Count())
	}
	s.Delete("a")
	if s.Count() != 1 {
		t.Errorf("expected 1, got %d", s.Count())
	}
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

func TestConcurrentAccess(t *testing.T) {
	s := New[testItem]("item")
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := s.NextID()
			s.Set(id, testItem{Name: "concurrent", Value: i})
			s.Get(id)
			s.List()
			s.Count()
		}(i)
	}
	wg.Wait()

	if s.Count() != 100 {
		t.Errorf("expected 100, got %d", s.Count())
	}
}

// ---------------------------------------------------------------------------
// Clock
// ---------------------------------------------------------------------------

func TestNewClock(t *testing.T) {
	c := NewClock()
	if c == nil {
		t.Fatal("expected non-nil clock")
	}
	if c.Offset() != 0 {
		t.Errorf("expected zero offset, got %v", c.Offset())
	}
}

func TestClockNow(t *testing.T) {
	c := NewClock()
	before := time.Now()
	now := c.Now()
	after := time.Now()

	if now.Before(before) || now.After(after) {
		t.Errorf("clock.Now() outside expected range: before=%v now=%v after=%v", before, now, after)
	}
}

func TestClockAdvance(t *testing.T) {
	c := NewClock()
	c.Advance(24 * time.Hour)

	if c.Offset() != 24*time.Hour {
		t.Errorf("expected 24h offset, got %v", c.Offset())
	}

	now := c.Now()
	realNow := time.Now()
	diff := now.Sub(realNow)

	// The simulated time should be roughly 24h ahead (within a few ms tolerance).
	if diff < 23*time.Hour+59*time.Minute || diff > 24*time.Hour+1*time.Minute {
		t.Errorf("expected ~24h offset, got %v", diff)
	}
}

func TestClockAdvanceCumulative(t *testing.T) {
	c := NewClock()
	c.Advance(1 * time.Hour)
	c.Advance(2 * time.Hour)

	if c.Offset() != 3*time.Hour {
		t.Errorf("expected 3h cumulative offset, got %v", c.Offset())
	}
}

func TestClockReset(t *testing.T) {
	c := NewClock()
	c.Advance(10 * time.Hour)
	c.Reset()

	if c.Offset() != 0 {
		t.Errorf("expected zero offset after reset, got %v", c.Offset())
	}
}
