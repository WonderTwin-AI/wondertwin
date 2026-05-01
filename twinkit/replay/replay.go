// Package replay captures twin request/response traffic for later
// inspection or off-host replay.
//
// Captured traffic is held in a per-recorder ring buffer, optionally
// fanned out to registered observers via a bounded channel, and
// exported as JSONL with a manifest header. Sensitive fields are
// redacted by the configured ScrubberSet (default: scrub.Default()).
//
// The Observer interface is load-bearing public API — it is consumed
// by the future synthetic-data sidecar and any local-only forwarder
// wired via twincore's ObserverEndpoint configuration. Implementations
// MUST be non-blocking: the recorder fans out via a bounded channel
// and drops on overflow rather than blocking the request hot path.
package replay

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/scrub"
)

// SchemaVersion is the bundle schema version emitted by Export. The
// manifest line carries this value; consumers may use it to switch
// parsing strategies if the format evolves.
const SchemaVersion = 1

// Default Config values.
const (
	defaultMaxEntries    = 10_000
	defaultMaxBodyBytes  = 64 * 1024
	defaultObserverDepth = 256
)

// QuirkSource exposes the active set of quirks at the moment a request
// is being recorded. Recorders snapshot this on each request so the
// Entry.QuirksActive field reflects the state at capture time.
//
// A nil QuirkSource (or one whose ActiveQuirks returns nil) yields an
// empty slice on the entry.
type QuirkSource interface {
	ActiveQuirks() []string
}

// SimClock returns the simulated wall-clock value at recording time.
// Used to populate Entry.SimTime independently of Entry.Timestamp
// (which is the real wall clock).
type SimClock interface {
	Now() time.Time
}

// Entry is a single recorded request/response pair. Headers are
// flattened to single-value maps; multi-valued headers are coalesced
// using http.Header.Get semantics. Bodies, when captured, are stored
// raw (post-scrubbing) and serialized as base64 in JSON.
type Entry struct {
	RunID            string            `json:"run_id,omitempty"`
	Seq              uint64            `json:"seq"`
	Timestamp        time.Time         `json:"timestamp"`
	Method           string            `json:"method"`
	Path             string            `json:"path"`
	Query            string            `json:"query,omitempty"`
	ReqHeaders       map[string]string `json:"req_headers,omitempty"`
	ReqBody          []byte            `json:"req_body,omitempty"`
	ReqBodyTruncated bool              `json:"req_body_truncated,omitempty"`
	ResStatus        int               `json:"res_status"`
	ResHeaders       map[string]string `json:"res_headers,omitempty"`
	ResBody          []byte            `json:"res_body,omitempty"`
	ResBodyTruncated bool              `json:"res_body_truncated,omitempty"`
	DurationNs       int64             `json:"duration_ns"`
	QuirksActive     []string          `json:"quirks_active,omitempty"`
	SimTime          time.Time         `json:"sim_time,omitempty"`
}

// Manifest summarizes a run. It is emitted as the first line of an
// exported JSONL bundle and returned from FinishRun.
type Manifest struct {
	TwinName      string    `json:"twin_name,omitempty"`
	TwinVersion   string    `json:"twin_version,omitempty"`
	RunID         string    `json:"run_id,omitempty"`
	Seed          int64     `json:"seed,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at,omitempty"`
	EntryCount    int       `json:"entry_count"`
	SchemaVersion int       `json:"schema_version"`
}

// Config configures a Recorder. Zero values fall back to package
// defaults.
type Config struct {
	// MaxEntries bounds the ring buffer. Zero selects 10,000.
	MaxEntries int

	// MaxBodyBytes caps captured request and response body size.
	// Larger bodies are truncated and the corresponding *Truncated
	// flag is set on the Entry. Zero selects 64 KiB.
	MaxBodyBytes int

	// CaptureBodies enables body capture. Headers are always captured.
	CaptureBodies bool

	// Scrubbers redacts sensitive headers and bodies before storage
	// and observer fan-out. A nil set selects scrub.Default().
	Scrubbers *scrub.ScrubberSet

	// Quirks supplies the active quirk set at capture time. Optional.
	Quirks QuirkSource

	// Clock supplies simulated time for Entry.SimTime. Optional;
	// absent clock leaves SimTime as the zero value.
	Clock SimClock

	// TwinName / TwinVersion seed Manifest fields. Optional.
	TwinName    string
	TwinVersion string

	// Seed is captured into the manifest for reproducibility tracking.
	Seed int64

	// ObserverChannelDepth is the buffer size for the observer fan-out
	// channel. Zero selects 256. The channel is shared by all
	// observers; on overflow, entries are dropped.
	ObserverChannelDepth int
}

// Observer receives entries asynchronously. Implementations need not
// be thread-safe — Observe is invoked from a single fan-out goroutine.
// Implementations MUST be non-blocking; the recorder drops events
// rather than blocking the request path. See package doc.
type Observer interface {
	Observe(entry Entry)
}

// Recorder captures request/response pairs and fans them out to
// registered observers. It is safe for concurrent use.
type Recorder struct {
	cfg Config
	seq atomic.Uint64

	mu        sync.Mutex
	runID     string
	startedAt time.Time
	entries   []Entry
	head      int
	count     int

	obsMu     sync.RWMutex
	observers []Observer

	fanOut    chan Entry
	closeOnce sync.Once
	closeCh   chan struct{}
	wg        sync.WaitGroup
}

// NewRecorder returns a Recorder configured per cfg. Zero-value fields
// fall back to package defaults. The recorder spawns a single fan-out
// goroutine when observers will be added; the goroutine exits when
// Close is called.
func NewRecorder(cfg Config) *Recorder {
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = defaultMaxEntries
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = defaultMaxBodyBytes
	}
	if cfg.Scrubbers == nil {
		cfg.Scrubbers = scrub.Default()
	}
	if cfg.ObserverChannelDepth <= 0 {
		cfg.ObserverChannelDepth = defaultObserverDepth
	}
	r := &Recorder{
		cfg:     cfg,
		entries: make([]Entry, cfg.MaxEntries),
		fanOut:  make(chan Entry, cfg.ObserverChannelDepth),
		closeCh: make(chan struct{}),
	}
	r.wg.Add(1)
	go r.runFanOut()
	return r
}

// Config returns the resolved configuration the recorder is using.
func (r *Recorder) Config() Config { return r.cfg }

// Close stops the fan-out goroutine. After Close, the recorder
// continues to accept entries via Middleware but does not deliver them
// to observers. Close is idempotent.
func (r *Recorder) Close() {
	r.closeOnce.Do(func() {
		close(r.closeCh)
	})
	r.wg.Wait()
}

// AddObserver registers an Observer. Passing a nil observer is a
// no-op.
func (r *Recorder) AddObserver(o Observer) {
	if o == nil {
		return
	}
	r.obsMu.Lock()
	r.observers = append(r.observers, o)
	r.obsMu.Unlock()
}

// StartRun marks the beginning of a recording run. The buffer is
// cleared and entries written after this call carry the given run id.
// Calling StartRun while a run is in progress replaces the active run
// and clears its buffered entries.
func (r *Recorder) StartRun(runID string) {
	r.mu.Lock()
	r.runID = runID
	r.startedAt = time.Now().UTC()
	r.head = 0
	r.count = 0
	r.mu.Unlock()
}

// FinishRun closes the active run and returns its Manifest. The buffer
// is retained so a subsequent Export can serialize the captured
// entries; the next StartRun clears it.
func (r *Recorder) FinishRun() Manifest {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := Manifest{
		TwinName:      r.cfg.TwinName,
		TwinVersion:   r.cfg.TwinVersion,
		RunID:         r.runID,
		Seed:          r.cfg.Seed,
		StartedAt:     r.startedAt,
		FinishedAt:    time.Now().UTC(),
		EntryCount:    r.count,
		SchemaVersion: SchemaVersion,
	}
	return m
}

// CurrentManifest returns the manifest for the active run without
// closing it. Used by GET /admin/replay?format=manifest.
func (r *Recorder) CurrentManifest() Manifest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Manifest{
		TwinName:      r.cfg.TwinName,
		TwinVersion:   r.cfg.TwinVersion,
		RunID:         r.runID,
		Seed:          r.cfg.Seed,
		StartedAt:     r.startedAt,
		EntryCount:    r.count,
		SchemaVersion: SchemaVersion,
	}
}

// Entries returns a snapshot of the entries currently retained, in
// chronological order.
func (r *Recorder) Entries() []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked()
}

func (r *Recorder) snapshotLocked() []Entry {
	out := make([]Entry, r.count)
	if r.count == 0 {
		return out
	}
	if r.count < r.cfg.MaxEntries {
		copy(out, r.entries[:r.count])
		return out
	}
	// Ring buffer: head points at the oldest slot.
	n := copy(out, r.entries[r.head:])
	copy(out[n:], r.entries[:r.head])
	return out
}

// Export writes the active run's bundle as JSONL to w. The first line
// is the Manifest; subsequent lines are Entry objects. The writer is
// not closed.
func (r *Recorder) Export(w io.Writer) error {
	r.mu.Lock()
	manifest := Manifest{
		TwinName:      r.cfg.TwinName,
		TwinVersion:   r.cfg.TwinVersion,
		RunID:         r.runID,
		Seed:          r.cfg.Seed,
		StartedAt:     r.startedAt,
		EntryCount:    r.count,
		SchemaVersion: SchemaVersion,
	}
	entries := r.snapshotLocked()
	r.mu.Unlock()

	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(manifest); err != nil {
		return err
	}
	for i := range entries {
		if err := enc.Encode(entries[i]); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// append stores an entry, evicting the oldest when at capacity, and
// hands it to the fan-out goroutine. Never blocks the request path.
func (r *Recorder) append(e Entry) {
	r.mu.Lock()
	if r.count < r.cfg.MaxEntries {
		// Buffer not yet full; entries occupy slots [0, count).
		r.entries[r.count] = e
		r.count++
	} else {
		// Buffer full; head is the oldest slot we now overwrite.
		r.entries[r.head] = e
		r.head = (r.head + 1) % r.cfg.MaxEntries
	}
	r.mu.Unlock()

	select {
	case r.fanOut <- e:
	default:
		// Drop on overflow; observers are non-blocking by contract.
	}
}

func (r *Recorder) runFanOut() {
	defer r.wg.Done()
	for {
		select {
		case <-r.closeCh:
			return
		case e := <-r.fanOut:
			r.obsMu.RLock()
			obs := append([]Observer(nil), r.observers...)
			r.obsMu.RUnlock()
			for _, o := range obs {
				safeObserve(o, e)
			}
		}
	}
}

func safeObserve(o Observer, e Entry) {
	defer func() {
		_ = recover()
	}()
	o.Observe(e)
}

// Middleware returns an http middleware that captures requests served
// by the wrapped handler and stores them in the recorder's buffer.
//
// Header maps are flattened to single-value maps using http.Header.Get
// semantics; multi-value headers (which are extremely rare on twin
// surfaces) are coalesced lossily. Bodies, when captured, are read
// fully into memory (capped at MaxBodyBytes), scrubbed, and emitted as
// base64 in JSON output.
func (r *Recorder) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			seq := r.seq.Add(1) - 1

			var reqBody []byte
			var reqTrunc bool
			if r.cfg.CaptureBodies && req.Body != nil {
				reqBody, reqTrunc = readCapped(req.Body, r.cfg.MaxBodyBytes)
				_ = req.Body.Close()
				req.Body = io.NopCloser(bytes.NewReader(reqBody))
			}

			rec := newCaptureWriter(w, r.cfg.MaxBodyBytes, r.cfg.CaptureBodies)
			next.ServeHTTP(rec, req)

			r.mu.Lock()
			runID := r.runID
			r.mu.Unlock()

			scrubbers := r.cfg.Scrubbers
			reqHeaders := flattenHeaders(req.Header, scrubbers)
			resHeaders := flattenHeaders(rec.Header(), scrubbers)

			reqContentType := req.Header.Get("Content-Type")
			resContentType := rec.Header().Get("Content-Type")
			if r.cfg.CaptureBodies && len(reqBody) > 0 {
				reqBody = scrubbers.ScrubBody(reqContentType, reqBody)
			}
			resBody := rec.body
			resTrunc := rec.truncated
			if r.cfg.CaptureBodies && len(resBody) > 0 {
				resBody = scrubbers.ScrubBody(resContentType, resBody)
			}

			var quirks []string
			if r.cfg.Quirks != nil {
				quirks = r.cfg.Quirks.ActiveQuirks()
			}
			var simNow time.Time
			if r.cfg.Clock != nil {
				simNow = r.cfg.Clock.Now()
			}

			r.append(Entry{
				RunID:            runID,
				Seq:              seq,
				Timestamp:        start.UTC(),
				Method:           req.Method,
				Path:             req.URL.Path,
				Query:            req.URL.RawQuery,
				ReqHeaders:       reqHeaders,
				ReqBody:          reqBody,
				ReqBodyTruncated: reqTrunc,
				ResStatus:        rec.status,
				ResHeaders:       resHeaders,
				ResBody:          resBody,
				ResBodyTruncated: resTrunc,
				DurationNs:       time.Since(start).Nanoseconds(),
				QuirksActive:     quirks,
				SimTime:          simNow,
			})
		})
	}
}

// readCapped reads up to max bytes from r and returns the bytes plus a
// truncated flag. Any error other than io.EOF causes the read to stop;
// already-read bytes are still returned.
func readCapped(r io.Reader, max int) ([]byte, bool) {
	if max <= 0 {
		return nil, false
	}
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	truncated := false
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			remaining := max - len(buf)
			if remaining <= 0 {
				truncated = true
				// drain remainder so downstream reads complete cleanly
				_, _ = io.Copy(io.Discard, r)
				break
			}
			if n > remaining {
				buf = append(buf, tmp[:remaining]...)
				truncated = true
				_, _ = io.Copy(io.Discard, r)
				break
			}
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf, truncated
}

func flattenHeaders(h http.Header, scrubbers *scrub.ScrubberSet) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k := range h {
		v := h.Get(k)
		if scrubbers != nil {
			v = scrubbers.ScrubHeader(k, v)
		}
		if v == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// captureWriter records the response status, headers, and (optionally)
// body bytes while passing them through to the underlying writer. It
// extends rather than replaces the existing twincore.statusRecorder
// pattern.
type captureWriter struct {
	http.ResponseWriter
	status    int
	max       int
	capture   bool
	body      []byte
	truncated bool
}

func newCaptureWriter(w http.ResponseWriter, max int, capture bool) *captureWriter {
	return &captureWriter{ResponseWriter: w, status: http.StatusOK, max: max, capture: capture}
}

func (c *captureWriter) WriteHeader(code int) {
	c.status = code
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureWriter) Write(p []byte) (int, error) {
	if c.capture && !c.truncated {
		remaining := c.max - len(c.body)
		switch {
		case remaining <= 0:
			c.truncated = true
		case len(p) > remaining:
			c.body = append(c.body, p[:remaining]...)
			c.truncated = true
		default:
			c.body = append(c.body, p...)
		}
	}
	return c.ResponseWriter.Write(p)
}

// ContentType helper — left exported for tests that want to assert
// scrubber behavior on the captured Content-Type header without
// reaching into the unexported map.
func (e Entry) ContentType() string {
	if e.ResHeaders == nil {
		return ""
	}
	for k, v := range e.ResHeaders {
		if strings.EqualFold(k, "Content-Type") {
			return v
		}
	}
	return ""
}
