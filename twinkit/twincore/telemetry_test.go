package twincore

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

type capturedBatch struct {
	SchemaVersion int               `json:"schema_version"`
	Events        []json.RawMessage `json:"events"`
}

func startTelemetrySink(t *testing.T) (string, *atomic.Int64, func() []capturedBatch) {
	t.Helper()
	var hits atomic.Int64
	var mu sync.Mutex
	var batches []capturedBatch
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var b capturedBatch
		_ = json.Unmarshal(body, &b)
		mu.Lock()
		batches = append(batches, b)
		mu.Unlock()
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &hits, func() []capturedBatch {
		mu.Lock()
		defer mu.Unlock()
		out := make([]capturedBatch, len(batches))
		copy(out, batches)
		return out
	}
}

func TestTelemetryStartupEvents(t *testing.T) {
	url, hits, snapshot := startTelemetrySink(t)
	t.Setenv(envTelemetryEndpoint, url)
	t.Setenv(envTelemetry, "")

	tw := New(&Config{Name: "twin-test"})
	defer tw.Close()
	if tw.Telemetry == nil {
		t.Fatal("Telemetry should be wired")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hits.Load() > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	tw.Telemetry.Close()

	batches := snapshot()
	if len(batches) == 0 {
		t.Fatal("no batches received")
	}
	types := map[string]int{}
	for _, b := range batches {
		for _, raw := range b.Events {
			var e map[string]any
			_ = json.Unmarshal(raw, &e)
			if t, ok := e["type"].(string); ok {
				types[t]++
			}
		}
	}
	if types["twin_started"] == 0 {
		t.Errorf("expected twin_started; got %v", types)
	}
	if types["license_status"] == 0 {
		t.Errorf("expected license_status; got %v", types)
	}
}

func TestTelemetryOptOutSendsNothing(t *testing.T) {
	url, hits, _ := startTelemetrySink(t)
	t.Setenv(envTelemetryEndpoint, url)
	t.Setenv(envTelemetry, "off")

	tw := New(&Config{Name: "twin-test"})
	defer tw.Close()
	tw.Telemetry.Close()

	time.Sleep(200 * time.Millisecond)
	if hits.Load() != 0 {
		t.Errorf("opt-out twin sent %d batches", hits.Load())
	}
}

func TestEndpointHitFiresThroughRouter(t *testing.T) {
	url, hits, snapshot := startTelemetrySink(t)
	t.Setenv(envTelemetryEndpoint, url)
	t.Setenv(envTelemetry, "")

	tw := New(&Config{Name: "twin-test"})
	tw.Router.Get("/v1/probe", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})

	srv := httptest.NewServer(tw.Router)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/probe")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Force a flush by closing the reporter; this drains both the
	// startup events and the just-emitted endpoint_hit.
	tw.Telemetry.Close()

	var found bool
	for _, b := range snapshot() {
		for _, raw := range b.Events {
			var e map[string]any
			_ = json.Unmarshal(raw, &e)
			if e["type"] == "endpoint_hit" {
				found = true
				if e["endpoint"] != "GET /v1/probe" {
					t.Errorf("endpoint = %v", e["endpoint"])
				}
				if int(e["status_code"].(float64)) != 200 {
					t.Errorf("status_code = %v", e["status_code"])
				}
			}
		}
	}
	if !found {
		t.Fatalf("endpoint_hit never observed; hits=%d", hits.Load())
	}
}
