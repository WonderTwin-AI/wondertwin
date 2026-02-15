package twincore

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// RequestLog
// ---------------------------------------------------------------------------

func TestNewRequestLog(t *testing.T) {
	rl := NewRequestLog(10)
	if rl == nil {
		t.Fatal("expected non-nil RequestLog")
	}
	if len(rl.Entries()) != 0 {
		t.Errorf("expected 0 entries, got %d", len(rl.Entries()))
	}
}

func TestRequestLogAdd(t *testing.T) {
	rl := NewRequestLog(10)
	rl.Add(RequestLogEntry{Method: "GET", Path: "/test"})

	entries := rl.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Method != "GET" || entries[0].Path != "/test" {
		t.Errorf("unexpected entry: %+v", entries[0])
	}
}

func TestRequestLogRingBuffer(t *testing.T) {
	rl := NewRequestLog(3)

	for i := 0; i < 5; i++ {
		rl.Add(RequestLogEntry{Path: "/" + string(rune('a'+i))})
	}

	entries := rl.Entries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (ring buffer), got %d", len(entries))
	}
	// oldest entries should have been evicted
	if entries[0].Path != "/c" {
		t.Errorf("expected /c, got %s", entries[0].Path)
	}
	if entries[1].Path != "/d" {
		t.Errorf("expected /d, got %s", entries[1].Path)
	}
	if entries[2].Path != "/e" {
		t.Errorf("expected /e, got %s", entries[2].Path)
	}
}

func TestRequestLogEntriesReturnsCopy(t *testing.T) {
	rl := NewRequestLog(10)
	rl.Add(RequestLogEntry{Path: "/orig"})

	entries := rl.Entries()
	entries[0].Path = "/mutated"

	fresh := rl.Entries()
	if fresh[0].Path != "/orig" {
		t.Error("Entries did not return a copy; mutation leaked")
	}
}

func TestRequestLogClear(t *testing.T) {
	rl := NewRequestLog(10)
	rl.Add(RequestLogEntry{Path: "/test"})
	rl.Clear()

	if len(rl.Entries()) != 0 {
		t.Errorf("expected 0 entries after clear, got %d", len(rl.Entries()))
	}
}

// ---------------------------------------------------------------------------
// FaultRegistry
// ---------------------------------------------------------------------------

func TestNewFaultRegistry(t *testing.T) {
	fr := NewFaultRegistry()
	if fr == nil {
		t.Fatal("expected non-nil FaultRegistry")
	}
	if len(fr.All()) != 0 {
		t.Errorf("expected 0 faults, got %d", len(fr.All()))
	}
}

func TestFaultRegistrySetAndCheck(t *testing.T) {
	fr := NewFaultRegistry()
	fr.Set("/v1/test", FaultConfig{StatusCode: 500, Rate: 1.0})

	fault := fr.Check("/v1/test")
	if fault == nil {
		t.Fatal("expected fault to be returned")
	}
	if fault.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", fault.StatusCode)
	}
}

func TestFaultRegistrySetDefaultRate(t *testing.T) {
	fr := NewFaultRegistry()
	// Rate=0 should default to 1.0
	fr.Set("/test", FaultConfig{StatusCode: 429, Rate: 0})

	fault := fr.Check("/test")
	if fault == nil {
		t.Fatal("expected fault with default rate=1.0")
	}
}

func TestFaultRegistryCheckNoMatch(t *testing.T) {
	fr := NewFaultRegistry()
	fr.Set("/v1/foo", FaultConfig{StatusCode: 500, Rate: 1.0})

	if fr.Check("/v1/bar") != nil {
		t.Error("expected nil for non-matching path")
	}
}

func TestFaultRegistryRemove(t *testing.T) {
	fr := NewFaultRegistry()
	fr.Set("/test", FaultConfig{StatusCode: 500, Rate: 1.0})

	if !fr.Remove("/test") {
		t.Error("expected Remove to return true for existing fault")
	}
	if fr.Remove("/test") {
		t.Error("expected Remove to return false after already removed")
	}
	if fr.Check("/test") != nil {
		t.Error("expected nil after removal")
	}
}

func TestFaultRegistryAll(t *testing.T) {
	fr := NewFaultRegistry()
	fr.Set("/a", FaultConfig{StatusCode: 500, Rate: 1.0})
	fr.Set("/b", FaultConfig{StatusCode: 429, Rate: 1.0})

	all := fr.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 faults, got %d", len(all))
	}
	if all["/a"].StatusCode != 500 || all["/b"].StatusCode != 429 {
		t.Errorf("unexpected faults: %+v", all)
	}
}

func TestFaultRegistryAllReturnsCopy(t *testing.T) {
	fr := NewFaultRegistry()
	fr.Set("/a", FaultConfig{StatusCode: 500, Rate: 1.0})

	all := fr.All()
	all["/a"] = FaultConfig{StatusCode: 200}

	fresh := fr.All()
	if fresh["/a"].StatusCode != 500 {
		t.Error("All did not return a copy; mutation leaked")
	}
}

func TestFaultRegistryReset(t *testing.T) {
	fr := NewFaultRegistry()
	fr.Set("/test", FaultConfig{StatusCode: 500, Rate: 1.0})
	fr.Reset()

	if len(fr.All()) != 0 {
		t.Errorf("expected 0 faults after reset, got %d", len(fr.All()))
	}
}

// ---------------------------------------------------------------------------
// IdempotencyTracker
// ---------------------------------------------------------------------------

func TestNewIdempotencyTracker(t *testing.T) {
	it := NewIdempotencyTracker()
	if it == nil {
		t.Fatal("expected non-nil IdempotencyTracker")
	}
}

func TestIdempotencyTrackerStoreAndCheck(t *testing.T) {
	it := NewIdempotencyTracker()
	it.Store("key1", 200, []byte(`{"id":"123"}`))

	status, body, ok := it.Check("key1")
	if !ok {
		t.Fatal("expected key to be found")
	}
	if status != 200 {
		t.Errorf("expected status 200, got %d", status)
	}
	if string(body) != `{"id":"123"}` {
		t.Errorf("unexpected body: %s", string(body))
	}
}

func TestIdempotencyTrackerCheckMissing(t *testing.T) {
	it := NewIdempotencyTracker()
	_, _, ok := it.Check("nonexistent")
	if ok {
		t.Error("expected ok=false for missing key")
	}
}

func TestIdempotencyTrackerReset(t *testing.T) {
	it := NewIdempotencyTracker()
	it.Store("k", 200, []byte("ok"))
	it.Reset()

	_, _, ok := it.Check("k")
	if ok {
		t.Error("expected key to be cleared after reset")
	}
}

// ---------------------------------------------------------------------------
// Middleware – CORS
// ---------------------------------------------------------------------------

func TestCORSMiddleware(t *testing.T) {
	cfg := &Config{}
	mw := NewMiddleware(cfg, slog.Default())

	handler := mw.CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Normal request
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected Access-Control-Allow-Origin: *")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestCORSOptionsRequest(t *testing.T) {
	cfg := &Config{}
	mw := NewMiddleware(cfg, slog.Default())

	innerCalled := false
	handler := mw.CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 for OPTIONS, got %d", rec.Code)
	}
	if innerCalled {
		t.Error("expected inner handler NOT to be called for OPTIONS")
	}
	if rec.Header().Get("Access-Control-Max-Age") != "3600" {
		t.Error("expected Access-Control-Max-Age: 3600")
	}
}

// ---------------------------------------------------------------------------
// Middleware – RequestLog (the middleware, not the data structure)
// ---------------------------------------------------------------------------

func TestRequestLogMiddleware(t *testing.T) {
	cfg := &Config{Verbose: false}
	mw := NewMiddleware(cfg, slog.Default())

	handler := mw.RequestLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest("POST", "/v1/resources", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	entries := mw.ReqLog.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Method != "POST" {
		t.Errorf("expected POST, got %s", entries[0].Method)
	}
	if entries[0].Path != "/v1/resources" {
		t.Errorf("expected /v1/resources, got %s", entries[0].Path)
	}
	if entries[0].StatusCode != 201 {
		t.Errorf("expected 201, got %d", entries[0].StatusCode)
	}
}

func TestRequestLogMiddlewareVerbose(t *testing.T) {
	cfg := &Config{Verbose: true}
	mw := NewMiddleware(cfg, slog.Default())

	handler := mw.RequestLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Custom", "value")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	entries := mw.ReqLog.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Headers == nil {
		t.Fatal("expected headers to be captured in verbose mode")
	}
	if entries[0].Headers["X-Custom"] != "value" {
		t.Errorf("expected X-Custom=value, got %s", entries[0].Headers["X-Custom"])
	}
}

// ---------------------------------------------------------------------------
// Middleware – FaultInjection
// ---------------------------------------------------------------------------

func TestFaultInjectionMiddleware(t *testing.T) {
	cfg := &Config{}
	mw := NewMiddleware(cfg, slog.Default())
	mw.Faults.Set("/v1/test", FaultConfig{StatusCode: 503, Rate: 1.0})

	innerCalled := false
	handler := mw.FaultInjection(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 503 {
		t.Errorf("expected 503 from fault injection, got %d", rec.Code)
	}
	if innerCalled {
		t.Error("expected inner handler NOT to be called when fault is injected")
	}
}

func TestFaultInjectionWithCustomBody(t *testing.T) {
	cfg := &Config{}
	mw := NewMiddleware(cfg, slog.Default())
	mw.Faults.Set("/v1/test", FaultConfig{StatusCode: 429, Body: `{"error":"rate_limited"}`, Rate: 1.0})

	handler := mw.FaultInjection(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 429 {
		t.Errorf("expected 429, got %d", rec.Code)
	}
	if rec.Body.String() != `{"error":"rate_limited"}` {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestFaultInjectionNoFault(t *testing.T) {
	cfg := &Config{}
	mw := NewMiddleware(cfg, slog.Default())

	innerCalled := false
	handler := mw.FaultInjection(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/clean", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !innerCalled {
		t.Error("expected inner handler to be called when no fault matches")
	}
}

// ---------------------------------------------------------------------------
// Middleware – LatencyInjection
// ---------------------------------------------------------------------------

func TestLatencyInjectionMiddleware(t *testing.T) {
	cfg := &Config{Latency: 50 * time.Millisecond}
	mw := NewMiddleware(cfg, slog.Default())

	handler := mw.LatencyInjection(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	start := time.Now()
	handler.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	// Expect at least 80% of 50ms = 40ms (due to jitter)
	if elapsed < 30*time.Millisecond {
		t.Errorf("expected at least ~40ms latency, got %v", elapsed)
	}
}

func TestLatencyInjectionZero(t *testing.T) {
	cfg := &Config{Latency: 0}
	mw := NewMiddleware(cfg, slog.Default())

	handler := mw.LatencyInjection(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	start := time.Now()
	handler.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if elapsed > 10*time.Millisecond {
		t.Errorf("expected near-zero latency, got %v", elapsed)
	}
}

// ---------------------------------------------------------------------------
// Middleware – RandomFailure
// ---------------------------------------------------------------------------

func TestRandomFailureAlways(t *testing.T) {
	cfg := &Config{FailRate: 1.0}
	mw := NewMiddleware(cfg, slog.Default())

	handler := mw.RandomFailure(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 with fail rate 1.0, got %d", rec.Code)
	}
}

func TestRandomFailureNever(t *testing.T) {
	cfg := &Config{FailRate: 0.0}
	mw := NewMiddleware(cfg, slog.Default())

	handler := mw.RandomFailure(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with fail rate 0.0, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// NewMiddleware
// ---------------------------------------------------------------------------

func TestNewMiddleware(t *testing.T) {
	cfg := &Config{}
	mw := NewMiddleware(cfg, slog.Default())

	if mw.ReqLog == nil {
		t.Error("expected non-nil ReqLog")
	}
	if mw.Faults == nil {
		t.Error("expected non-nil Faults")
	}
	if mw.Idempotent == nil {
		t.Error("expected non-nil Idempotent")
	}
}
