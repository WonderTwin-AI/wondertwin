package store

import "github.com/wondertwin-ai/wondertwin/twinkit/sim"

// NewHex returns n lowercase hex characters drawn from the store's seeded
// Rand, so a seeded run reissues the same values.
func (s *MemoryStore) NewHex(n int) string {
	return sim.NewIDGenerator(s.Rand).Hex((n + 1) / 2)[:n]
}

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// idLength is the length of an item_id and an account_id: 37 base62
// characters, opaque and never sequential (quirks IDENTITY-002, IDENTITY-004).
// The kit's Nanoid is not used because its alphabet includes _ and -.
const idLength = 37

// NewID returns an opaque 37-character base62 id from the seeded Rand. The Rand
// is not part of a snapshot, so a seeded run that loads state replays the draws
// that made it; an id already held by an Item, Account or Transaction is
// skipped, never reissued.
func (s *MemoryStore) NewID() string {
	for {
		b := make([]byte, idLength)
		for i := range b {
			b[i] = base62[s.Rand.Intn(len(base62))]
		}
		id := string(b)
		if !s.idTaken(id) {
			return id
		}
	}
}

func (s *MemoryStore) idTaken(id string) bool {
	if _, ok := s.Items.Get(id); ok {
		return true
	}
	if _, ok := s.Accounts.Get(id); ok {
		return true
	}
	_, ok := s.Transactions.Get(id)
	return ok
}
