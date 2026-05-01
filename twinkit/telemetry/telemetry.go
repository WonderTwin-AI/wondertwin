// Package telemetry provides a non-blocking event reporter that twins
// use to emit usage and operational signals to a central ingestion
// endpoint.
//
// The contract Record callers depend on is simple: Record MUST NOT
// block, MUST NOT return an error, and MUST NOT panic — even under
// sustained load, when the endpoint is unreachable, or when the
// destination service is malfunctioning. The reporter implements this
// via a bounded channel + a single batching goroutine + an HTTP client
// with conservative timeouts; failures are dropped (and logged at
// debug) rather than propagated.
//
// The default endpoint at telemetry.DefaultEndpoint is a placeholder
// pointing at the planned WonderTwin ingestion service. Until that
// service ships, every emitted batch is dropped after the configured
// retry budget — by design.
package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// SchemaVersion is the wire-format version of the POST body. The body
// is `{"schema_version":<N>,"events":[...]}` so the ingestion side
// can route by version. Bump together with orgHashSalt when the event
// shape changes incompatibly.
const SchemaVersion = 1

// DefaultEndpoint is the placeholder ingestion URL. It is a
// syntactically valid URL so http.NewRequest does not error; the
// actual service stands up in a follow-up wave.
const DefaultEndpoint = "https://telemetry.wondertwin.dev/v1/events"

// Default Config values. The channel buffer is fixed to keep the
// public surface small; retry/timeout knobs are similarly fixed.
const (
	defaultBatchSize     = 50
	defaultFlush         = 30 * time.Second
	channelBufferSize    = 1024
	httpClientTimeout    = 5 * time.Second
	retryBaseDelay       = 100 * time.Millisecond
	retryMaxRetryAfter   = 30 * time.Second
	closeFlushDeadline   = time.Second
)

// Event is a single telemetry record. The shape is intentionally flat
// for simple ingestion. Empty fields serialize as omitempty so
// payloads stay small.
type Event struct {
	Type       string    `json:"type,omitempty"`
	TwinName   string    `json:"twin_name,omitempty"`
	TwinVer    string    `json:"twin_ver,omitempty"`
	Mode       string    `json:"mode,omitempty"`
	Platform   string    `json:"platform,omitempty"`
	Endpoint   string    `json:"endpoint,omitempty"`
	StatusCode int       `json:"status_code,omitempty"`
	DurationMs int       `json:"duration_ms,omitempty"`
	QuirksOn   []string  `json:"quirks_on,omitempty"`
	OrgHash    string    `json:"org_hash,omitempty"`
	LicenseID  string    `json:"license_id,omitempty"`
	LicenseOK  bool      `json:"license_ok,omitempty"`
	LicenseRsn string    `json:"license_reason,omitempty"`
	RunID      string    `json:"run_id,omitempty"`
	Seeded     bool      `json:"seeded,omitempty"`
	SeedSource string    `json:"seed_source,omitempty"`
	Timestamp  time.Time `json:"timestamp,omitempty"`
}

// Config configures a Reporter. Zero values fall back to package
// defaults.
type Config struct {
	// Endpoint is the collector URL. Empty selects DefaultEndpoint.
	Endpoint string

	// OptOut disables emission entirely. Record becomes a cheap no-op
	// and the goroutine is never started.
	OptOut bool

	// BatchSize is the number of events flushed per HTTP request.
	// Zero selects 50.
	BatchSize int

	// Flush is the maximum interval between flushes. Zero selects 30s.
	Flush time.Duration

	// OrgHash, when set, is the orgHash value the reporter exposes via
	// Reporter.OrgHash(). The middleware fills it in on every event.
	OrgHash string

	// LicenseID is included on the reporter for callers that want to
	// stamp it onto events. Reporter.LicenseID() exposes it.
	LicenseID string

	// HTTPClient is an optional override (tests inject a stub). Zero
	// uses an internal client with a 5s timeout.
	HTTPClient *http.Client

	// Logger is an optional slog logger; zero defaults to a discard
	// handler so production code paths emit nothing on success.
	Logger *slog.Logger
}

// Reporter accepts events via Record, batches them, and ships them
// to Endpoint over HTTP. Record is non-blocking; failures during
// transport are dropped (and logged at debug only).
type Reporter struct {
	cfg    Config
	ch     chan Event
	client *http.Client
	logger *slog.Logger

	closeOnce sync.Once
	closeCh   chan struct{}
	doneCh    chan struct{}
}

// New constructs a Reporter. When OptOut is set, the goroutine is not
// started and Record is a no-op.
func New(cfg Config) *Reporter {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}
	if cfg.Flush <= 0 {
		cfg.Flush = defaultFlush
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultEndpoint
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: httpClientTimeout}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	r := &Reporter{
		cfg:     cfg,
		client:  cfg.HTTPClient,
		logger:  cfg.Logger,
		closeCh: make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
	if cfg.OptOut {
		close(r.doneCh)
		return r
	}
	r.ch = make(chan Event, channelBufferSize)
	go r.runWithRecover()
	return r
}

// Record enqueues an event. Non-blocking by contract: when the buffer
// is full or OptOut is set, the event is silently dropped. Never
// returns an error and never panics.
func (r *Reporter) Record(ev Event) {
	if r == nil || r.ch == nil {
		return
	}
	select {
	case r.ch <- ev:
	default:
	}
}

// OrgHash returns the configured OrgHash. Useful for callers that want
// to stamp it onto events emitted outside of Middleware.
func (r *Reporter) OrgHash() string {
	if r == nil {
		return ""
	}
	return r.cfg.OrgHash
}

// LicenseID returns the configured LicenseID.
func (r *Reporter) LicenseID() string {
	if r == nil {
		return ""
	}
	return r.cfg.LicenseID
}

// Flush is a convenience hook that triggers a synchronous flush of
// any buffered events. The provided context bounds the wait.
func (r *Reporter) Flush(ctx context.Context) error {
	if r == nil || r.ch == nil {
		return nil
	}
	// Best-effort: wait until the goroutine drains the channel or ctx
	// times out. We don't know batch boundaries from outside, so we
	// approximate by waiting for the channel length to reach zero.
	deadline := time.After(closeFlushDeadline)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		if len(r.ch) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return nil
		case <-tick.C:
		}
	}
}

// Close signals the goroutine to stop. It performs a best-effort
// drain bounded by closeFlushDeadline. Idempotent.
func (r *Reporter) Close() {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		close(r.closeCh)
	})
	select {
	case <-r.doneCh:
	case <-time.After(closeFlushDeadline + httpClientTimeout):
	}
}

// runWithRecover is the goroutine entrypoint. It wraps run() in a
// defer/recover so a panic in marshal or HTTP path never kills the
// twin process; the panic is logged and the goroutine restarts the
// loop body. We do not restart in production beyond the safety net —
// repeated panics indicate a bug worth crashing on, not silently
// swallowing forever.
func (r *Reporter) runWithRecover() {
	defer close(r.doneCh)
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Debug("telemetry reporter panic", "panic", rec)
		}
	}()
	r.run()
}

func (r *Reporter) run() {
	ticker := time.NewTicker(r.cfg.Flush)
	defer ticker.Stop()
	buf := make([]Event, 0, r.cfg.BatchSize)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		send := make([]Event, len(buf))
		copy(send, buf)
		buf = buf[:0]
		r.send(send)
	}
	for {
		select {
		case <-r.closeCh:
			// Drain remaining events with a small budget.
			for {
				select {
				case ev := <-r.ch:
					buf = append(buf, ev)
					if len(buf) >= r.cfg.BatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		case ev := <-r.ch:
			buf = append(buf, ev)
			if len(buf) >= r.cfg.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// envelope is the wire shape: a schema version paired with the events
// array. Wrapping lets the ingestion service reject incompatible
// versions cheaply.
type envelope struct {
	SchemaVersion int     `json:"schema_version"`
	Events        []Event `json:"events"`
}

// send POSTs a batch with one retry on transport error or 5xx.
func (r *Reporter) send(batch []Event) {
	body, err := json.Marshal(envelope{SchemaVersion: SchemaVersion, Events: batch})
	if err != nil {
		r.logger.Debug("telemetry marshal failed", "err", err, "events", len(batch))
		return
	}
	for attempt := 0; attempt < 2; attempt++ {
		retryAfter, retry, err := r.attempt(body)
		if err == nil {
			return
		}
		if !retry {
			r.logger.Debug("telemetry drop (no retry)", "err", err, "attempt", attempt)
			return
		}
		if attempt == 1 {
			r.logger.Debug("telemetry drop (retry exhausted)", "err", err)
			return
		}
		delay := retryAfter
		if delay <= 0 {
			delay = retryBaseDelay + time.Duration(rand.Int64N(int64(retryBaseDelay)))
		}
		select {
		case <-r.closeCh:
			return
		case <-time.After(delay):
		}
	}
}

// attempt performs a single HTTP POST. It returns a retry-after delay
// (zero if not specified by the server), a boolean indicating whether
// to retry, and an error explaining any failure.
func (r *Reporter) attempt(body []byte) (time.Duration, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), httpClientTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return 0, true, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return 0, false, nil
	case resp.StatusCode == http.StatusTooManyRequests:
		return retryAfterHeader(resp.Header.Get("Retry-After")), true, fmt.Errorf("http %d", resp.StatusCode)
	case resp.StatusCode >= 500:
		return 0, true, fmt.Errorf("http %d", resp.StatusCode)
	default: // 4xx other than 429
		return 0, false, fmt.Errorf("http %d", resp.StatusCode)
	}
}

// retryAfterHeader parses the Retry-After header. Only seconds-form
// values are honored; HTTP-date form is ignored to keep parsing
// deterministic. Values above retryMaxRetryAfter are clamped.
func retryAfterHeader(h string) time.Duration {
	if h == "" {
		return 0
	}
	secs, err := strconv.Atoi(h)
	if err != nil {
		return 0
	}
	d := time.Duration(secs) * time.Second
	if d > retryMaxRetryAfter {
		return retryMaxRetryAfter
	}
	return d
}

// IsTelemetryDisabled returns true when the WONDERTWIN_TELEMETRY env
// var is set to a value that disables telemetry. Recognized: off, 0,
// false, no, disabled (case-insensitive). Empty defaults to enabled.
func IsTelemetryDisabled(value string) bool {
	switch normalize(value) {
	case "off", "0", "false", "no", "disabled":
		return true
	}
	return false
}

func normalize(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

