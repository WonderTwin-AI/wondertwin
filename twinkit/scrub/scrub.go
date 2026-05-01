// Package scrub provides a shared header- and body-redaction registry
// used by twinkit components that capture or forward request/response
// data (replay recorders, observer forwarders, telemetry payloads, and
// the future synthetic-data sidecar).
//
// Scrubbers are pure functions invoked on the hot path; they MUST NOT
// allocate excessively, perform I/O, or block. A scrubber returning the
// empty string for a header drops that header entirely.
package scrub

import (
	"bytes"
	"strings"
)

// HeaderScrubber redacts the value of a single header. It receives the
// canonical header name and the raw value, and returns the redacted
// value. Returning "" instructs the caller to drop the header.
type HeaderScrubber func(name string, value string) string

// BodyScrubber redacts a request or response body. The contentType
// argument is the Content-Type header value (or empty); body is the
// raw bytes. The returned slice may alias body if no changes were made.
type BodyScrubber func(contentType string, body []byte) []byte

// ScrubberSet is an ordered collection of header and body scrubbers.
// Scrubbers run in the order they were added; later scrubbers see the
// output of earlier ones.
//
// The zero value is a usable empty set, but callers should prefer
// NewSet or Default.
type ScrubberSet struct {
	headers []HeaderScrubber
	bodies  []BodyScrubber
}

// NewSet returns an empty ScrubberSet.
func NewSet() *ScrubberSet { return &ScrubberSet{} }

// AddHeader appends a header scrubber to the set.
func (s *ScrubberSet) AddHeader(scrub HeaderScrubber) {
	if scrub == nil {
		return
	}
	s.headers = append(s.headers, scrub)
}

// AddBody appends a body scrubber to the set.
func (s *ScrubberSet) AddBody(scrub BodyScrubber) {
	if scrub == nil {
		return
	}
	s.bodies = append(s.bodies, scrub)
}

// ScrubHeader runs every registered header scrubber against the given
// header name/value and returns the final value. An empty return means
// the header should be dropped.
func (s *ScrubberSet) ScrubHeader(name, value string) string {
	for _, fn := range s.headers {
		value = fn(name, value)
		if value == "" {
			return ""
		}
	}
	return value
}

// ScrubBody runs every registered body scrubber in order. The returned
// slice may alias body when no scrubber modified it.
func (s *ScrubberSet) ScrubBody(contentType string, body []byte) []byte {
	for _, fn := range s.bodies {
		body = fn(contentType, body)
	}
	return body
}

// Default returns a ScrubberSet seeded with redactors for the common
// secret-bearing headers and cookies. The exact set is intentionally
// conservative — replay and telemetry consumers may extend it via
// AddHeader and AddBody.
func Default() *ScrubberSet {
	s := NewSet()
	s.AddHeader(redactSensitiveHeaders)
	s.AddBody(redactCookieAttributes)
	return s
}

var sensitiveHeaderNames = map[string]struct{}{
	"authorization":  {},
	"x-api-key":      {},
	"stripe-account": {},
	"set-cookie":     {},
	"cookie":         {},
}

func redactSensitiveHeaders(name, value string) string {
	if _, ok := sensitiveHeaderNames[strings.ToLower(name)]; ok {
		return "REDACTED"
	}
	return value
}

// redactCookieAttributes is a no-op placeholder for body-level cookie
// redaction. Real implementations may rewrite Set-Cookie attributes
// embedded in HTML bodies; for now we return the input unchanged so
// existing tests remain stable.
func redactCookieAttributes(_ string, body []byte) []byte {
	return body
}

// Equal reports whether two byte slices are equal. Exported for the
// benefit of scrubbers that need to short-circuit when the input is
// unchanged.
func Equal(a, b []byte) bool { return bytes.Equal(a, b) }
