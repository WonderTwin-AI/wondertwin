// Package replay defines the interfaces and types used to capture twin
// request/response traffic for later replay or off-host inspection.
//
// This package currently provides only the public seam: the Recorder,
// the Observer fan-out hook, and the Manifest/Entry/Config value types.
// Body capture, JSONL export, and ring-buffer ordering are filled in by
// a follow-up change. The middleware returned by Recorder.Middleware()
// is intentionally a pass-through so existing twins compile and serve
// requests with no behavior change.
//
// The Observer interface is load-bearing public API — it is consumed by
// the future synthetic-data sidecar and any local-only forwarder wired
// via twincore's ObserverEndpoint configuration. Implementations MUST
// be non-blocking; the recorder calls Observe synchronously on the
// request path.
package replay

import (
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/scrub"
)

// Default Config values. Exported for documentation only — callers
// should rely on Config zero-value handling, not these constants.
const (
	defaultMaxEntries   = 10_000
	defaultMaxBodyBytes = 64 * 1024
)

// Entry is a single recorded request/response pair. Fields are populated
// best-effort: when body capture is disabled, ReqBody and ResBody are
// nil. SimTime, when non-zero, is the simulated clock value at request
// arrival.
type Entry struct {
	RunID        string
	Seq          uint64
	Timestamp    time.Time
	Method       string
	Path         string
	Query        string
	ReqHeaders   map[string]string
	ReqBody      []byte
	ResStatus    int
	ResHeaders   map[string]string
	ResBody      []byte
	DurationNs   int64
	QuirksActive []string
	SimTime      time.Time
}

// Manifest summarizes a completed run. It is emitted by FinishRun and
// is the canonical bundle metadata when entries are exported to JSONL.
type Manifest struct {
	TwinName      string
	TwinVersion   string
	RunID         string
	Seed          int64
	StartedAt     time.Time
	FinishedAt    time.Time
	EntryCount    int
	SchemaVersion int
}

// Config configures a Recorder. Zero values fall back to the package
// defaults: 10_000 entries, 64 KiB per body, body capture disabled, and
// the default scrubber set.
type Config struct {
	// MaxEntries is the upper bound on entries retained per run. When
	// the bound is reached older entries are evicted. Zero selects the
	// default of 10,000.
	MaxEntries int

	// MaxBodyBytes is the per-body capture cap. Bodies larger than the
	// cap are truncated. Zero selects the default of 65,536 bytes.
	MaxBodyBytes int

	// CaptureBodies enables request and response body capture. Body
	// capture is gated separately from header capture because it is
	// substantially more expensive on the hot path.
	CaptureBodies bool

	// Scrubbers redacts sensitive headers and bodies before they are
	// stored or fanned out to observers. A nil set means "use the
	// default scrubber set"; pass scrub.NewSet() for an empty set.
	Scrubbers *scrub.ScrubberSet
}

// Observer receives entries as they are recorded. Implementations MUST
// be non-blocking and MUST NOT panic — the recorder invokes Observe on
// the request hot path.
type Observer interface {
	Observe(entry Entry)
}

// Recorder captures request/response pairs and fans them out to any
// registered observers. It is safe for concurrent use.
//
// The recorder is currently a structural seam: Middleware passes
// requests through unchanged, Entries returns the empty slice, and
// Export writes nothing. The public surface is stable; subsequent
// changes will fill in the body.
type Recorder struct {
	cfg Config

	mu        sync.Mutex
	runID     string
	startedAt time.Time
	observers []Observer
}

// NewRecorder returns a Recorder configured per cfg. Zero-value fields
// fall back to package defaults.
func NewRecorder(cfg Config) *Recorder {
	if cfg.MaxEntries == 0 {
		cfg.MaxEntries = defaultMaxEntries
	}
	if cfg.MaxBodyBytes == 0 {
		cfg.MaxBodyBytes = defaultMaxBodyBytes
	}
	if cfg.Scrubbers == nil {
		cfg.Scrubbers = scrub.Default()
	}
	return &Recorder{cfg: cfg}
}

// Config returns the resolved configuration the recorder is using.
// Useful for tests asserting that defaults were applied.
func (r *Recorder) Config() Config {
	return r.cfg
}

// AddObserver registers an Observer for fan-out. Observers are invoked
// in registration order. Passing a nil observer is a no-op.
func (r *Recorder) AddObserver(o Observer) {
	if o == nil {
		return
	}
	r.mu.Lock()
	r.observers = append(r.observers, o)
	r.mu.Unlock()
}

// Middleware returns an http middleware that records requests served
// by the wrapped handler. In this revision the middleware is a
// pass-through so callers can wire it now and pick up real recording
// when the implementation lands.
func (r *Recorder) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req)
		})
	}
}

// StartRun marks the beginning of a recording run with the given id.
// Calling StartRun while a run is in progress replaces the active run.
func (r *Recorder) StartRun(runID string) {
	r.mu.Lock()
	r.runID = runID
	r.startedAt = time.Now().UTC()
	r.mu.Unlock()
}

// FinishRun closes the active run and returns its Manifest. If no run
// is active the returned Manifest is the zero value.
func (r *Recorder) FinishRun() Manifest {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runID == "" {
		return Manifest{}
	}
	m := Manifest{
		RunID:      r.runID,
		StartedAt:  r.startedAt,
		FinishedAt: time.Now().UTC(),
	}
	r.runID = ""
	r.startedAt = time.Time{}
	return m
}

// Export writes the active run's entries as JSONL to w. The current
// implementation writes nothing and returns nil; the export format
// will be specified when body capture is wired.
func (r *Recorder) Export(w io.Writer) error {
	_ = w
	return nil
}

// Entries returns a snapshot of the entries currently retained by the
// recorder. The current implementation returns an empty slice.
func (r *Recorder) Entries() []Entry {
	return []Entry{}
}
