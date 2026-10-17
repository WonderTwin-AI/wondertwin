package store

import "github.com/wondertwin-ai/wondertwin/twinkit/sim"

// NewID returns a Xero identifier: a lowercase GUID, as every Xero record ID
// is, drawn from the store's seeded Rand so a seeded run reissues the same IDs.
func (s *MemoryStore) NewID() string {
	return sim.NewIDGenerator(s.Rand).UUID()
}
