package telemetry

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/cimode"
)

func TestMiddlewareEmitsEndpointHit(t *testing.T) {
	t.Parallel()
	var got atomic.Pointer[envelope]
	var hits atomic.Int64
	srv := collectingServer(&got, &hits)
	defer srv.Close()

	r := New(Config{Endpoint: srv.URL, OrgHash: "abc", BatchSize: 1, Flush: 50 * time.Millisecond})
	defer r.Close()

	lic := &cimode.License{Key: "lic_test"}
	statusFn := func() (cimode.Status, *cimode.License) {
		return cimode.Status{Valid: true}, lic
	}

	mw := r.Middleware("twin-x", "0.1.0", "ci", "github-actions", statusFn)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/charges", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if !waitForHit(&hits, 1, time.Second) {
		t.Fatal("no event delivered to server")
	}
	env := got.Load()
	if env == nil || len(env.Events) != 1 {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	e := env.Events[0]
	if e.Type != "endpoint_hit" {
		t.Errorf("Type = %q", e.Type)
	}
	if e.TwinName != "twin-x" || e.TwinVer != "0.1.0" {
		t.Errorf("attribution: %+v", e)
	}
	if e.Endpoint != "POST /v1/charges" {
		t.Errorf("endpoint = %q", e.Endpoint)
	}
	if e.StatusCode != 201 {
		t.Errorf("status = %d", e.StatusCode)
	}
	if e.Mode != "ci" || e.Platform != "github-actions" {
		t.Errorf("mode/platform: %+v", e)
	}
	if e.OrgHash != "abc" {
		t.Errorf("org_hash = %q", e.OrgHash)
	}
	if !e.LicenseOK || e.LicenseID != "lic_test" {
		t.Errorf("license: %+v", e)
	}
}

func TestMiddlewareSkipsAdminPaths(t *testing.T) {
	t.Parallel()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		hits.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	r := New(Config{Endpoint: srv.URL, BatchSize: 1, Flush: 50 * time.Millisecond})
	defer r.Close()

	mw := r.Middleware("twin-x", "", "local", "", nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))

	for _, p := range []string{"/admin/state", "/admin/replay", "/healthz", "/health"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, p, nil))
	}
	time.Sleep(200 * time.Millisecond)
	if hits.Load() != 0 {
		t.Errorf("admin/health paths emitted %d events, want 0", hits.Load())
	}
}

func TestMiddlewareDoesNotLeakInvalidLicenseID(t *testing.T) {
	t.Parallel()
	var got atomic.Pointer[envelope]
	var hits atomic.Int64
	srv := collectingServer(&got, &hits)
	defer srv.Close()

	r := New(Config{Endpoint: srv.URL, BatchSize: 1, Flush: 50 * time.Millisecond})
	defer r.Close()

	lic := &cimode.License{Key: "lic_secret"}
	statusFn := func() (cimode.Status, *cimode.License) {
		return cimode.Status{Reason: cimode.ReasonExpired}, lic
	}
	mw := r.Middleware("twin-x", "", "ci", "", statusFn)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/things", nil))

	if !waitForHit(&hits, 1, time.Second) {
		t.Fatal("no events received")
	}
	e := got.Load().Events[0]
	if e.LicenseID != "" {
		t.Errorf("invalid license should not leak ID; got %q", e.LicenseID)
	}
	if e.LicenseRsn != cimode.ReasonExpired {
		t.Errorf("LicenseRsn = %q", e.LicenseRsn)
	}
}

func TestMiddlewareIsNonBlockingWhenChannelSaturated(t *testing.T) {
	t.Parallel()
	// Reporter pointed at a black-hole endpoint so the goroutine
	// can't drain the channel; once the buffer fills, Record falls
	// through to its default-drop case. The middleware must remain
	// non-blocking under that pressure.
	r := New(Config{Endpoint: "http://127.0.0.1:1/", BatchSize: 1, Flush: 30 * time.Second})
	defer r.Close()

	for i := 0; i < channelBufferSize+10; i++ {
		r.Record(Event{Type: "fill"})
	}

	mw := r.Middleware("twin-x", "", "local", "", nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	start := time.Now()
	for i := 0; i < 1000; i++ {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/x", nil))
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("middleware blocked: 1000 reqs in %v", d)
	}
}

// Confirm the middleware preserves response headers and status seen
// by clients despite the wrapping.
func TestMiddlewarePassesThroughResponse(t *testing.T) {
	t.Parallel()
	r := New(Config{Endpoint: "http://127.0.0.1:1/", BatchSize: 1, Flush: 30 * time.Second})
	defer r.Close()
	mw := r.Middleware("twin-x", "", "local", "", nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Pass", "yes")
		w.WriteHeader(202)
		_, _ = w.Write([]byte("body"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != 202 {
		t.Errorf("Code = %d", rec.Code)
	}
	if rec.Header().Get("X-Pass") != "yes" {
		t.Errorf("custom header lost")
	}
	if rec.Body.String() != "body" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

// Sanity: envelope JSON fields stable.
func TestEnvelopeJSONShape(t *testing.T) {
	t.Parallel()
	body, _ := json.Marshal(envelope{SchemaVersion: SchemaVersion, Events: []Event{{Type: "x"}}})
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["schema_version"]; !ok {
		t.Error("missing schema_version")
	}
	if _, ok := raw["events"]; !ok {
		t.Error("missing events")
	}
}
