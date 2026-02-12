package twincore

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"
)

// RequestLogEntry captures details of an incoming request for admin inspection.
type RequestLogEntry struct {
	Timestamp  time.Time         `json:"timestamp"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Headers    map[string]string `json:"headers,omitempty"`
	StatusCode int               `json:"status_code"`
	Duration   time.Duration     `json:"duration_ms"`
	RequestID  string            `json:"request_id,omitempty"`
}

// RequestLog is a thread-safe ring buffer of recent requests.
type RequestLog struct {
	mu      sync.RWMutex
	entries []RequestLogEntry
	maxSize int
}

// NewRequestLog creates a request log with the given max size.
func NewRequestLog(maxSize int) *RequestLog {
	return &RequestLog{
		entries: make([]RequestLogEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add appends an entry, evicting the oldest if at capacity.
func (rl *RequestLog) Add(entry RequestLogEntry) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if len(rl.entries) >= rl.maxSize {
		rl.entries = rl.entries[1:]
	}
	rl.entries = append(rl.entries, entry)
}

// Entries returns a copy of all log entries.
func (rl *RequestLog) Entries() []RequestLogEntry {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	out := make([]RequestLogEntry, len(rl.entries))
	copy(out, rl.entries)
	return out
}

// Clear removes all entries.
func (rl *RequestLog) Clear() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.entries = rl.entries[:0]
}

// FaultConfig defines a fault injection for a specific endpoint pattern.
type FaultConfig struct {
	StatusCode int           `json:"status_code"`
	Body       string        `json:"body,omitempty"`
	Delay      time.Duration `json:"delay_ms,omitempty"`
	Rate       float64       `json:"rate"` // 0.0-1.0, probability of fault triggering
}

// FaultRegistry manages injected faults for specific endpoint patterns.
type FaultRegistry struct {
	mu     sync.RWMutex
	faults map[string]FaultConfig // path pattern -> fault config
}

// NewFaultRegistry creates a new fault registry.
func NewFaultRegistry() *FaultRegistry {
	return &FaultRegistry{
		faults: make(map[string]FaultConfig),
	}
}

// Set injects a fault for the given endpoint pattern.
func (fr *FaultRegistry) Set(pattern string, fault FaultConfig) {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	if fault.Rate == 0 {
		fault.Rate = 1.0
	}
	fr.faults[pattern] = fault
}

// Remove removes a fault for the given endpoint pattern.
func (fr *FaultRegistry) Remove(pattern string) bool {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	_, existed := fr.faults[pattern]
	delete(fr.faults, pattern)
	return existed
}

// Check returns a fault config if one matches the given path, or nil if no fault applies.
func (fr *FaultRegistry) Check(path string) *FaultConfig {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	if f, ok := fr.faults[path]; ok {
		if f.Rate >= 1.0 || rand.Float64() < f.Rate {
			return &f
		}
	}
	return nil
}

// All returns all registered faults.
func (fr *FaultRegistry) All() map[string]FaultConfig {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	out := make(map[string]FaultConfig, len(fr.faults))
	for k, v := range fr.faults {
		out[k] = v
	}
	return out
}

// Reset clears all faults.
func (fr *FaultRegistry) Reset() {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.faults = make(map[string]FaultConfig)
}

// IdempotencyTracker tracks idempotency keys and their cached responses.
type IdempotencyTracker struct {
	mu      sync.RWMutex
	entries map[string]idempotencyEntry
}

type idempotencyEntry struct {
	StatusCode int
	Body       []byte
	CreatedAt  time.Time
}

// NewIdempotencyTracker creates a new tracker.
func NewIdempotencyTracker() *IdempotencyTracker {
	return &IdempotencyTracker{
		entries: make(map[string]idempotencyEntry),
	}
}

// Check returns cached response data for the given key, or false if not seen.
func (it *IdempotencyTracker) Check(key string) (int, []byte, bool) {
	it.mu.RLock()
	defer it.mu.RUnlock()
	if e, ok := it.entries[key]; ok {
		return e.StatusCode, e.Body, true
	}
	return 0, nil, false
}

// Store caches a response for the given idempotency key.
func (it *IdempotencyTracker) Store(key string, statusCode int, body []byte) {
	it.mu.Lock()
	defer it.mu.Unlock()
	it.entries[key] = idempotencyEntry{
		StatusCode: statusCode,
		Body:       body,
		CreatedAt:  time.Now(),
	}
}

// Reset clears all tracked keys.
func (it *IdempotencyTracker) Reset() {
	it.mu.Lock()
	defer it.mu.Unlock()
	it.entries = make(map[string]idempotencyEntry)
}

// Middleware provides common middleware functions for all twins.
type Middleware struct {
	cfg        *Config
	logger     *slog.Logger
	ReqLog     *RequestLog
	Faults     *FaultRegistry
	Idempotent *IdempotencyTracker
}

// NewMiddleware creates a new Middleware instance.
func NewMiddleware(cfg *Config, logger *slog.Logger) *Middleware {
	return &Middleware{
		cfg:        cfg,
		logger:     logger,
		ReqLog:     NewRequestLog(1000),
		Faults:     NewFaultRegistry(),
		Idempotent: NewIdempotencyTracker(),
	}
}

// CORS adds permissive CORS headers (appropriate for a test twin).
func (m *Middleware) CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Idempotency-Key, Stripe-Account, X-Api-Key")
		w.Header().Set("Access-Control-Max-Age", "3600")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the status code written by downstream handlers.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
}

// RequestLog middleware captures request details into the ring buffer.
func (m *Middleware) RequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, statusCode: 200}

		next.ServeHTTP(rec, r)

		entry := RequestLogEntry{
			Timestamp:  start,
			Method:     r.Method,
			Path:       r.URL.Path,
			StatusCode: rec.statusCode,
			Duration:   time.Since(start),
		}
		if m.cfg.Verbose {
			entry.Headers = make(map[string]string)
			for k := range r.Header {
				entry.Headers[k] = r.Header.Get(k)
			}
		}
		m.ReqLog.Add(entry)

		if m.cfg.Verbose {
			m.logger.Debug("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.statusCode,
				"duration", time.Since(start),
			)
		}
	})
}

// LatencyInjection adds configurable latency to every request.
func (m *Middleware) LatencyInjection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.cfg.Latency > 0 {
			// Add some jitter: 80-120% of configured latency
			jitter := 0.8 + rand.Float64()*0.4
			delay := time.Duration(float64(m.cfg.Latency) * jitter)
			time.Sleep(delay)
		}
		next.ServeHTTP(w, r)
	})
}

// RandomFailure randomly returns 500 errors based on the configured fail rate.
func (m *Middleware) RandomFailure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.cfg.FailRate > 0 && rand.Float64() < m.cfg.FailRate {
			Error(w, http.StatusInternalServerError, "simulated random failure")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// FaultInjection checks the fault registry and applies any matching faults.
// This should be applied INSIDE route groups, not globally, so admin endpoints
// are not affected.
func (m *Middleware) FaultInjection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fault := m.Faults.Check(r.URL.Path); fault != nil {
			if fault.Delay > 0 {
				time.Sleep(fault.Delay)
			}
			if fault.StatusCode > 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(fault.StatusCode)
				if fault.Body != "" {
					fmt.Fprint(w, fault.Body)
				} else {
					fmt.Fprintf(w, `{"error":{"message":"injected fault","type":"api_error","code":%d}}`, fault.StatusCode)
				}
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
