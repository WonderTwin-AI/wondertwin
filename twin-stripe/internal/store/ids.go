package store

import (
	"strings"

	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
)

// idLengths are the suffix lengths Stripe's documented example IDs use
// (cus_NffrFeUfNV2Hib, prod_NWjs8kKbJWmuuc: 14; pi_3MtwBwLkdIwHu7ix28a3tqPa,
// ch_3LmzzQ2eZvKYlo2C0XjzUzJV: 24). Other prefixes use 24.
var idLengths = map[string]int{"cus": 14, "prod": 14}

// idPrefixes maps the kit store's prefix to Stripe's where they differ:
// Checkout Session IDs carry the mode ("cs_test_...").
var idPrefixes = map[string]string{"cs": "cs_test"}

// StripeID turns a sequential kit ID ("cus_000001") into an opaque,
// Stripe-shaped one ("cus_NffrFeUfNV2Hib") with the same prefix, drawn from
// the store's seeded random source so a seeded run reproduces its IDs. With
// no random source it returns seq unchanged.
func (s *MemoryStore) StripeID(seq string) string {
	if s == nil || s.Rand == nil {
		return seq
	}
	prefix, _, ok := strings.Cut(seq, "_")
	if !ok {
		return seq
	}
	n := idLengths[prefix]
	if n == 0 {
		n = 24
	}
	if p, ok := idPrefixes[prefix]; ok {
		prefix = p
	}
	return sim.NewIDGenerator(s.Rand).Prefixed(prefix+"_", n)
}
