// Package telemetry provides a non-blocking event reporter that twins
// use to emit usage and operational signals.
//
// The contract that callers depend on is simple: Record MUST NOT block,
// MUST NOT return an error, and MUST NOT panic — even under sustained
// load or when the upstream collector is unavailable. The current
// implementation is a structural seam: events are accepted into an
// internal channel and dropped on overflow. The HTTP transport is
// added later; subsequent changes must keep Record's non-blocking
// guarantee intact (see the Benchmark in this package).
package telemetry

import (
	"context"
	"time"
)

// Default Config values.
const (
	defaultBatchSize = 50
	defaultFlush     = 30 * time.Second
	defaultBuffer    = 4096
)

// Event is a single telemetry record. All fields are optional; the
// reporter never inspects payload contents and applies no validation.
type Event struct {
	Type       string
	TwinName   string
	TwinVer    string
	Mode       string
	Platform   string
	Endpoint   string
	StatusCode int
	DurationMs int
	QuirksOn   []string
	OrgHash    string
	RunID      string
	Seeded     bool
	SeedSource string
	Timestamp  time.Time
}

// Config configures a Reporter. Zero values fall back to package
// defaults.
type Config struct {
	// Endpoint is the collector URL. Empty means events are accepted
	// and dropped — useful for local development and tests.
	Endpoint string

	// OptOut disables emission entirely. When set, Record is a cheap
	// no-op. Callers should honor WONDERTWIN_TELEMETRY=off by setting
	// this field.
	OptOut bool

	// BatchSize is the number of events flushed per HTTP request.
	// Zero selects the default of 50.
	BatchSize int

	// Flush is the maximum interval between flushes. Zero selects the
	// default of 30s.
	Flush time.Duration

	// OrgHash is the stable, non-reversible hash of the user's
	// organization identifier, included on every event.
	OrgHash string

	// LicenseID identifies the license the events are emitted under.
	LicenseID string
}

// Reporter accepts events via Record and (eventually) ships them to the
// configured Endpoint. The current implementation drops events on
// overflow without surfacing an error.
type Reporter struct {
	cfg Config
	ch  chan Event
}

// New returns a Reporter configured per cfg. The reporter is ready to
// accept events immediately.
func New(cfg Config) *Reporter {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}
	if cfg.Flush <= 0 {
		cfg.Flush = defaultFlush
	}
	r := &Reporter{cfg: cfg}
	if !cfg.OptOut {
		r.ch = make(chan Event, defaultBuffer)
	}
	return r
}

// Record enqueues an event for asynchronous emission. It is
// non-blocking by contract: when the buffer is full or OptOut is set,
// the event is silently dropped.
//
// Callers may invoke Record from any goroutine, including the request
// hot path. Record never returns an error and never panics.
func (r *Reporter) Record(ev Event) {
	if r == nil || r.ch == nil {
		return
	}
	select {
	case r.ch <- ev:
	default:
	}
}

// Flush is a hook for tests and graceful shutdown to drain pending
// events. The current implementation has no transport, so Flush
// returns nil immediately.
func (r *Reporter) Flush(ctx context.Context) error {
	_ = ctx
	return nil
}

// Close stops the reporter. After Close, subsequent calls to Record
// continue to be safe but drop their input.
func (r *Reporter) Close() {
	if r == nil || r.ch == nil {
		return
	}
	r.ch = nil
}
