package sim

import (
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
)

// IDGenerator produces IDs in various formats used by real platforms.
type IDGenerator struct {
	rng     *Rand
	counter atomic.Uint64
}

// NewIDGenerator creates a new ID generator.
func NewIDGenerator(rng *Rand) *IDGenerator {
	return &IDGenerator{rng: rng}
}

// Sequential returns a sequential ID with prefix: "inv_000001", "inv_000002", etc.
func (g *IDGenerator) Sequential(prefix string) string {
	n := g.counter.Add(1)
	return fmt.Sprintf("%s_%06d", prefix, n)
}

// UUID returns a deterministic UUID v4.
func (g *IDGenerator) UUID() string {
	var buf [16]byte
	g.rng.mu.Lock()
	for i := range buf {
		buf[i] = byte(g.rng.rng.Intn(256))
	}
	g.rng.mu.Unlock()

	// Set version 4 and variant bits.
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

// Prefixed returns a Stripe-style prefixed random ID: "pi_1A2b3C4d5E6f7G8h".
// length is the length of the random part (default 24).
func (g *IDGenerator) Prefixed(prefix string, length int) string {
	if length <= 0 {
		length = 24
	}
	const charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	var sb strings.Builder
	sb.WriteString(prefix)
	g.rng.mu.Lock()
	for i := 0; i < length; i++ {
		sb.WriteByte(charset[g.rng.rng.Intn(len(charset))])
	}
	g.rng.mu.Unlock()
	return sb.String()
}

// Nanoid returns a nanoid-style short ID (default 21 chars).
func (g *IDGenerator) Nanoid(length int) string {
	if length <= 0 {
		length = 21
	}
	const charset = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-"
	var sb strings.Builder
	g.rng.mu.Lock()
	for i := 0; i < length; i++ {
		sb.WriteByte(charset[g.rng.rng.Intn(len(charset))])
	}
	g.rng.mu.Unlock()
	return sb.String()
}

// Hex returns a random hex string of the given byte length.
func (g *IDGenerator) Hex(byteLength int) string {
	buf := make([]byte, byteLength)
	g.rng.mu.Lock()
	for i := range buf {
		buf[i] = byte(g.rng.rng.Intn(256))
	}
	g.rng.mu.Unlock()
	return hex.EncodeToString(buf)
}
