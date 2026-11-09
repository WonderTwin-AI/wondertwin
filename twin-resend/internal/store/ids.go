package store

import (
	"encoding/binary"
	"fmt"
)

// Resend issues UUIDv7 ids for emails (time-ordered) and UUIDv4 ids for
// every other resource (quirk identity-001). Both draw their random bits from
// the store's seeded Rand, and the v7 timestamp comes from the store's Clock,
// so a run with the same seed and clock issues the same ids.

// NewEmailID returns a UUIDv7 whose timestamp is the store clock's current
// Unix time in milliseconds.
func (s *MemoryStore) NewEmailID() string {
	var b [16]byte
	s.randBytes(b[:])
	ms := uint64(s.Clock.Now().UnixMilli()) //nolint:gosec // G115: a clock before 1970 is not a supported state.
	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], ms)
	copy(b[0:6], ts[2:8])
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80
	return formatUUID(b)
}

// NewID returns a UUIDv4 for any resource other than an email.
func (s *MemoryStore) NewID() string {
	var b [16]byte
	s.randBytes(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return formatUUID(b)
}

// NewMessageID returns a message id in the Amazon SES form Resend reports,
// <{16 hex}-{uuid}-000000@email.amazonses.com>.
func (s *MemoryStore) NewMessageID() string {
	var b [8]byte
	s.randBytes(b[:])
	return fmt.Sprintf("<%x-%s-000000@email.amazonses.com>", b, s.NewID())
}

// RandBytes fills buf from the store's seeded Rand, so generated values
// such as DKIM keys repeat for a seed.
func (s *MemoryStore) RandBytes(buf []byte) {
	s.randBytes(buf)
}

func (s *MemoryStore) randBytes(buf []byte) {
	s.randMu.RLock()
	rng := s.Rand
	s.randMu.RUnlock()
	for i := range buf {
		//nolint:gosec // G115: Intn(256) returns 0..255 by contract; the
		// conversion to byte cannot overflow.
		buf[i] = byte(rng.Intn(256))
	}
}

func formatUUID(b [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
