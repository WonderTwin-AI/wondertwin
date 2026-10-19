package store

import "github.com/wondertwin-ai/wondertwin/twinkit/sim"

// NewHex returns n lowercase hex characters drawn from the store's seeded
// Rand, so a seeded run reissues the same values.
func (s *MemoryStore) NewHex(n int) string {
	return sim.NewIDGenerator(s.Rand).Hex((n + 1) / 2)[:n]
}
