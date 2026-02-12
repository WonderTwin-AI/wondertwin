// Package store provides a generic, thread-safe, in-memory key-value store
// for use by WonderTwin twins. It supports CRUD operations, listing with cursor-based
// pagination, and deterministic ID generation.
package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Store is a generic, thread-safe, in-memory store for objects of type T.
// T must be a struct that can be marshaled/unmarshaled to JSON.
type Store[T any] struct {
	mu      sync.RWMutex
	items   map[string]T
	order   []string // insertion order for deterministic listing
	prefix  string
	counter atomic.Uint64
}

// New creates a new Store with the given ID prefix (e.g., "acct", "msg", "evt").
func New[T any](prefix string) *Store[T] {
	return &Store[T]{
		items:  make(map[string]T),
		order:  make([]string, 0),
		prefix: prefix,
	}
}

// NextID generates a deterministic ID with the store's prefix.
// IDs are of the form "{prefix}_{counter}" e.g., "acct_000001".
func (s *Store[T]) NextID() string {
	n := s.counter.Add(1)
	return fmt.Sprintf("%s_%06d", s.prefix, n)
}

// Set stores an item with the given ID. If the ID already exists, it is overwritten
// but its position in the insertion order is preserved.
func (s *Store[T]) Set(id string, item T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[id]; !exists {
		s.order = append(s.order, id)
	}
	s.items[id] = item
}

// Get retrieves an item by ID. Returns the item and true if found, zero value and false otherwise.
func (s *Store[T]) Get(id string) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	return item, ok
}

// Delete removes an item by ID. Returns true if the item existed.
func (s *Store[T]) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[id]; !exists {
		return false
	}
	delete(s.items, id)
	for i, oid := range s.order {
		if oid == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	return true
}

// List returns all items in insertion order.
func (s *Store[T]) List() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]T, 0, len(s.order))
	for _, id := range s.order {
		result = append(result, s.items[id])
	}
	return result
}

// ListIDs returns all IDs in insertion order.
func (s *Store[T]) ListIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.order))
	copy(out, s.order)
	return out
}

// Page represents a paginated result set.
type Page[T any] struct {
	Data    []T    `json:"data"`
	HasMore bool   `json:"has_more"`
	Cursor  string `json:"cursor,omitempty"`
	Total   int    `json:"total"`
}

// Paginate returns a page of items using cursor-based pagination.
// The cursor is the last ID seen. An empty cursor starts from the beginning.
// Limit controls the page size (0 means return all).
func (s *Store[T]) Paginate(cursor string, limit int) Page[T] {
	s.mu.RLock()
	defer s.mu.RUnlock()

	startIdx := 0
	if cursor != "" {
		for i, id := range s.order {
			if id == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	if limit <= 0 {
		limit = len(s.order)
	}

	endIdx := startIdx + limit
	hasMore := false
	if endIdx > len(s.order) {
		endIdx = len(s.order)
	} else if endIdx < len(s.order) {
		hasMore = true
	}

	data := make([]T, 0, endIdx-startIdx)
	var lastCursor string
	for i := startIdx; i < endIdx; i++ {
		data = append(data, s.items[s.order[i]])
		lastCursor = s.order[i]
	}

	return Page[T]{
		Data:    data,
		HasMore: hasMore,
		Cursor:  lastCursor,
		Total:   len(s.order),
	}
}

// Count returns the number of items in the store.
func (s *Store[T]) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

// Filter returns items that match the given predicate, in insertion order.
func (s *Store[T]) Filter(predicate func(id string, item T) bool) []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []T
	for _, id := range s.order {
		if predicate(id, s.items[id]) {
			result = append(result, s.items[id])
		}
	}
	return result
}

// FilterWithIDs returns items and their IDs that match the given predicate.
func (s *Store[T]) FilterWithIDs(predicate func(id string, item T) bool) ([]string, []T) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ids []string
	var items []T
	for _, id := range s.order {
		if predicate(id, s.items[id]) {
			ids = append(ids, id)
			items = append(items, s.items[id])
		}
	}
	return ids, items
}

// Reset clears all items and resets the ID counter.
func (s *Store[T]) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = make(map[string]T)
	s.order = make([]string, 0)
	s.counter.Store(0)
}

// Snapshot returns all items as a JSON-serializable map.
func (s *Store[T]) Snapshot() map[string]T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := make(map[string]T, len(s.items))
	for k, v := range s.items {
		snapshot[k] = v
	}
	return snapshot
}

// LoadSnapshot replaces all items from a JSON-serializable map.
// Existing items are cleared. IDs are sorted to maintain deterministic order.
func (s *Store[T]) LoadSnapshot(snapshot map[string]T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = make(map[string]T, len(snapshot))
	s.order = make([]string, 0, len(snapshot))
	for k, v := range snapshot {
		s.items[k] = v
		s.order = append(s.order, k)
	}
	sort.Strings(s.order)
}

// MarshalJSON serializes the store to JSON (the items map).
func (s *Store[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Snapshot())
}

// UnmarshalJSON deserializes JSON into the store, replacing existing items.
func (s *Store[T]) UnmarshalJSON(data []byte) error {
	var snapshot map[string]T
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	s.LoadSnapshot(snapshot)
	return nil
}

// Clock provides a simulated clock for time-dependent twin behavior.
type Clock struct {
	mu     sync.RWMutex
	offset time.Duration
}

// NewClock creates a new simulated clock with no offset.
func NewClock() *Clock {
	return &Clock{}
}

// Now returns the current simulated time.
func (c *Clock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Now().Add(c.offset)
}

// Advance moves the simulated clock forward by the given duration.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offset += d
}

// Reset resets the clock offset to zero.
func (c *Clock) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offset = 0
}

// Offset returns the current clock offset.
func (c *Clock) Offset() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.offset
}
