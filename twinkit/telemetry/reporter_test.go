package telemetry

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func collectingServer(received *atomic.Pointer[envelope], hits *atomic.Int64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var env envelope
		_ = json.Unmarshal(body, &env)
		received.Store(&env)
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
}

func TestReporterRoundTrip(t *testing.T) {
	t.Parallel()
	var got atomic.Pointer[envelope]
	var hits atomic.Int64
	srv := collectingServer(&got, &hits)
	defer srv.Close()

	r := New(Config{Endpoint: srv.URL, BatchSize: 1, Flush: 50 * time.Millisecond})
	defer r.Close()
	r.Record(Event{Type: "endpoint_hit", TwinName: "twin-x"})

	if !waitForHit(&hits, 1, time.Second) {
		t.Fatal("server never received POST")
	}
	env := got.Load()
	if env == nil {
		t.Fatal("envelope nil")
	}
	if env.SchemaVersion != SchemaVersion {
		t.Errorf("schema = %d", env.SchemaVersion)
	}
	if len(env.Events) != 1 || env.Events[0].Type != "endpoint_hit" {
		t.Errorf("events = %+v", env.Events)
	}
}

func TestReporterBatchesAtBatchSize(t *testing.T) {
	t.Parallel()
	var batches atomic.Int64
	var lastEnv atomic.Pointer[envelope]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var env envelope
		_ = json.Unmarshal(body, &env)
		lastEnv.Store(&env)
		batches.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := New(Config{Endpoint: srv.URL, BatchSize: 50, Flush: time.Hour})
	defer r.Close()
	for i := 0; i < 50; i++ {
		r.Record(Event{Type: "endpoint_hit"})
	}
	if !waitForHit(&batches, 1, time.Second) {
		t.Fatal("expected one batch send within 1s")
	}
	env := lastEnv.Load()
	if env == nil || len(env.Events) != 50 {
		t.Errorf("expected 50 events; got %+v", env)
	}
}

func TestReporterFlushesOnClose(t *testing.T) {
	t.Parallel()
	var hits atomic.Int64
	var lastEnv atomic.Pointer[envelope]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var env envelope
		_ = json.Unmarshal(body, &env)
		lastEnv.Store(&env)
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := New(Config{Endpoint: srv.URL, BatchSize: 100, Flush: time.Hour})
	r.Record(Event{Type: "endpoint_hit"})
	r.Record(Event{Type: "endpoint_hit"})
	r.Close()

	if hits.Load() != 1 {
		t.Errorf("expected 1 batch on close, got %d", hits.Load())
	}
	env := lastEnv.Load()
	if env == nil || len(env.Events) != 2 {
		t.Errorf("expected 2 events on close; got %+v", env)
	}
}

func TestReporterOptOutNoTraffic(t *testing.T) {
	t.Parallel()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	r := New(Config{Endpoint: srv.URL, OptOut: true})
	for i := 0; i < 1000; i++ {
		r.Record(Event{Type: "x"})
	}
	r.Close()
	if hits.Load() != 0 {
		t.Errorf("opt-out reporter sent %d POSTs", hits.Load())
	}
}

func TestReporterChaosBlackHoleEndpoint(t *testing.T) {
	// 127.0.0.1:1 reliably refuses connections fast on every OS.
	r := New(Config{Endpoint: "http://127.0.0.1:1/v1/events", BatchSize: 50, Flush: 10 * time.Millisecond})
	defer r.Close()

	const total = 100_000
	start := time.Now()
	for i := 0; i < total; i++ {
		r.Record(Event{Type: "chaos"})
	}
	elapsed := time.Since(start)
	// 100k Record calls must complete quickly (well under 1s) since
	// Record never blocks on the transport.
	if elapsed > time.Second {
		t.Errorf("100k Record calls took %v; non-blocking contract broken", elapsed)
	}
}

func TestReporterRetriesOn500ThenSucceeds(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	r := New(Config{Endpoint: srv.URL, BatchSize: 1, Flush: 30 * time.Second})
	defer r.Close()
	r.Record(Event{Type: "x"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if attempts.Load() >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if attempts.Load() != 2 {
		t.Errorf("attempts = %d, want 2 (one 500 + one retry)", attempts.Load())
	}
}

func TestReporterDoesNotRetry4xx(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(400)
	}))
	defer srv.Close()

	r := New(Config{Endpoint: srv.URL, BatchSize: 1, Flush: 30 * time.Second})
	defer r.Close()
	r.Record(Event{Type: "x"})

	time.Sleep(300 * time.Millisecond)
	if attempts.Load() != 1 {
		t.Errorf("4xx should not retry; attempts = %d", attempts.Load())
	}
}

func TestReporterDropAfterRetryExhausted(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(500)
	}))
	defer srv.Close()

	r := New(Config{Endpoint: srv.URL, BatchSize: 1, Flush: 30 * time.Second})
	defer r.Close()
	r.Record(Event{Type: "x"})

	time.Sleep(500 * time.Millisecond)
	got := attempts.Load()
	if got != 2 {
		t.Errorf("attempts = %d, want 2 (one + one retry, then drop)", got)
	}
}

func TestReporter429RespectsRetryAfter(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	r := New(Config{Endpoint: srv.URL, BatchSize: 1, Flush: 30 * time.Second})
	defer r.Close()

	start := time.Now()
	r.Record(Event{Type: "x"})
	for attempts.Load() < 2 {
		time.Sleep(50 * time.Millisecond)
		if time.Since(start) > 3*time.Second {
			t.Fatal("never retried")
		}
	}
	elapsed := time.Since(start)
	if elapsed < 900*time.Millisecond {
		t.Errorf("retry happened too soon: %v (expected ~1s)", elapsed)
	}
}

func TestIsTelemetryDisabled(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"off", "OFF", "0", "false", "FALSE", "no", "disabled"} {
		if !IsTelemetryDisabled(v) {
			t.Errorf("IsTelemetryDisabled(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"", "on", "1", "true", "yes", "enabled"} {
		if IsTelemetryDisabled(v) {
			t.Errorf("IsTelemetryDisabled(%q) = true, want false", v)
		}
	}
}

func waitForHit(c *atomic.Int64, want int64, deadline time.Duration) bool {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if c.Load() >= want {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// Sanity: the goroutine must not pile up under sustained Record load.
func TestNoGoroutineLeakUnderLoad(t *testing.T) {
	t.Parallel()
	r := New(Config{Endpoint: "http://127.0.0.1:1/v1/events", BatchSize: 50, Flush: 10 * time.Millisecond})
	var wg sync.WaitGroup
	for w := 0; w < 16; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10_000; i++ {
				r.Record(Event{Type: "load"})
			}
		}()
	}
	wg.Wait()
	r.Close()
}
