// Package sim provides deterministic simulation primitives for WonderTwin
// twins. All randomness is seeded for reproducibility — the same seed
// produces the same sequence of outcomes, IDs, and delays.
package sim

import (
	"math/rand"
	"sync"
)

// Rand is a deterministic random source. Thread-safe.
type Rand struct {
	mu  sync.Mutex
	rng *rand.Rand
}

// NewRand creates a new deterministic random source with the given seed.
func NewRand(seed int64) *Rand {
	return &Rand{rng: rand.New(rand.NewSource(seed))}
}

// DefaultRand creates a Rand with seed 42 (the answer to everything).
func DefaultRand() *Rand {
	return NewRand(42)
}

// Reseed replaces the underlying random source with a new one seeded
// at the given value. Reseeding while requests are in flight is
// undefined behavior — reseed only at run boundaries (e.g. on
// POST /admin/runs/start). The lock guarantees the swap itself is
// safe; it does not provide consistency for concurrent readers
// observing values mid-reseed.
func (r *Rand) Reseed(seed int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rng = rand.New(rand.NewSource(seed))
}

// Float64 returns a random float64 in [0, 1).
func (r *Rand) Float64() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rng.Float64()
}

// Intn returns a random int in [0, n).
func (r *Rand) Intn(n int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rng.Intn(n)
}

// Int63n returns a random int64 in [0, n).
func (r *Rand) Int63n(n int64) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rng.Int63n(n)
}
