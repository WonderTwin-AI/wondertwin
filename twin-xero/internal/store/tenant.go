package store

import (
	"sync"

	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// Tenanted keeps one store of T per Xero tenant. Every Accounting record
// belongs to the organisation named by the xero-tenant-id header, so a record
// of one tenant must never be listed, read or changed through another.
type Tenanted[T any] struct {
	mu       sync.Mutex
	byTenant map[string]*pkgstate.Store[T]
}

// NewTenanted creates an empty per-tenant store.
func NewTenanted[T any]() *Tenanted[T] {
	return &Tenanted[T]{byTenant: map[string]*pkgstate.Store[T]{}}
}

// For returns the store of one tenant, creating it on first use.
func (t *Tenanted[T]) For(tenantID string) *pkgstate.Store[T] {
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.byTenant[tenantID]
	if !ok {
		st = pkgstate.New[T]("")
		t.byTenant[tenantID] = st
	}
	return st
}

// Snapshot returns every tenant's records, keyed by tenant ID and record ID.
func (t *Tenanted[T]) Snapshot() map[string]map[string]T {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]map[string]T, len(t.byTenant))
	for id, st := range t.byTenant {
		out[id] = st.Snapshot()
	}
	return out
}

// Load replaces every tenant's records with a snapshot.
func (t *Tenanted[T]) Load(snap map[string]map[string]T) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.byTenant = make(map[string]*pkgstate.Store[T], len(snap))
	for id, records := range snap {
		st := pkgstate.New[T]("")
		st.LoadSnapshot(records)
		t.byTenant[id] = st
	}
}

// Reset drops every tenant's records.
func (t *Tenanted[T]) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.byTenant = map[string]*pkgstate.Store[T]{}
}
