package replay

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMiddlewareCapturesRequestAndResponse(t *testing.T) {
	t.Parallel()
	r := NewRecorder(Config{CaptureBodies: true})
	defer r.Close()
	r.StartRun("run-1")

	handler := r.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		if string(body) != `{"foo":"bar"}` {
			t.Errorf("downstream handler should still see body, got %q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"x"}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/charges?foo=1", strings.NewReader(`{"foo":"bar"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("downstream status = %d, want 201", w.Code)
	}
	entries := r.Entries()
	if len(entries) != 1 {
		t.Fatalf("Entries len = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Method != http.MethodPost || e.Path != "/v1/charges" || e.Query != "foo=1" {
		t.Errorf("entry method/path/query mismatch: %+v", e)
	}
	if e.ResStatus != http.StatusCreated {
		t.Errorf("entry status = %d", e.ResStatus)
	}
	if e.ReqHeaders["Authorization"] != "REDACTED" {
		t.Errorf("Authorization should be redacted; got %q", e.ReqHeaders["Authorization"])
	}
	if string(e.ReqBody) != `{"foo":"bar"}` {
		t.Errorf("ReqBody = %q", e.ReqBody)
	}
	if string(e.ResBody) != `{"id":"x"}` {
		t.Errorf("ResBody = %q", e.ResBody)
	}
	if e.RunID != "run-1" {
		t.Errorf("entry RunID = %q, want run-1", e.RunID)
	}
}

func TestMiddlewareTruncatesLargeBodies(t *testing.T) {
	t.Parallel()
	r := NewRecorder(Config{CaptureBodies: true, MaxBodyBytes: 16})
	defer r.Close()

	big := strings.Repeat("x", 1024)
	handler := r.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		_, _ = w.Write([]byte(big))
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(big))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	entries := r.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	e := entries[0]
	if !e.ReqBodyTruncated {
		t.Error("ReqBodyTruncated should be true")
	}
	if !e.ResBodyTruncated {
		t.Error("ResBodyTruncated should be true")
	}
	if len(e.ReqBody) != 16 || len(e.ResBody) != 16 {
		t.Errorf("body lengths = %d / %d, want 16 / 16", len(e.ReqBody), len(e.ResBody))
	}
}

func TestMiddlewareSkipsBodiesWhenDisabled(t *testing.T) {
	t.Parallel()
	r := NewRecorder(Config{CaptureBodies: false})
	defer r.Close()

	handler := r.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		_, _ = w.Write([]byte("hi"))
	}))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("data"))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	entries := r.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries len = %d", len(entries))
	}
	if entries[0].ReqBody != nil || entries[0].ResBody != nil {
		t.Errorf("bodies should be nil when capture disabled, got req=%v res=%v", entries[0].ReqBody, entries[0].ResBody)
	}
}

func TestMiddlewareRedactsJSONBodyFields(t *testing.T) {
	t.Parallel()
	r := NewRecorder(Config{CaptureBodies: true})
	defer r.Close()

	handler := r.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"client_secret":"sk_live_xxx","other":"ok"}`))
	}))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"password":"hunter2"}`))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	e := r.Entries()[0]
	if !strings.Contains(string(e.ReqBody), `"password":"REDACTED"`) {
		t.Errorf("password not redacted: %s", e.ReqBody)
	}
	if !strings.Contains(string(e.ResBody), `"client_secret":"REDACTED"`) {
		t.Errorf("client_secret not redacted: %s", e.ResBody)
	}
}

func TestRingBufferRotation(t *testing.T) {
	t.Parallel()
	r := NewRecorder(Config{MaxEntries: 3})
	defer r.Close()

	handler := r.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	entries := r.Entries()
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}
	// The newest three should have seq 7, 8, 9 (0-indexed).
	want := []uint64{7, 8, 9}
	for i, e := range entries {
		if e.Seq != want[i] {
			t.Errorf("entries[%d].Seq = %d, want %d", i, e.Seq, want[i])
		}
	}
}

type slowObserver struct {
	mu    sync.Mutex
	seen  int
	block chan struct{}
}

func (s *slowObserver) Observe(_ Entry) {
	<-s.block
	s.mu.Lock()
	s.seen++
	s.mu.Unlock()
}

func TestObserverOverflowDoesNotBlock(t *testing.T) {
	t.Parallel()
	r := NewRecorder(Config{ObserverChannelDepth: 2})
	defer r.Close()
	obs := &slowObserver{block: make(chan struct{})}
	r.AddObserver(obs)

	handler := r.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("middleware blocked under observer back-pressure")
	}
	close(obs.block)
}

type captureAll struct {
	mu      sync.Mutex
	entries []Entry
}

func (c *captureAll) Observe(e Entry) {
	c.mu.Lock()
	c.entries = append(c.entries, e)
	c.mu.Unlock()
}

func TestObserverReceivesEntries(t *testing.T) {
	t.Parallel()
	r := NewRecorder(Config{})
	defer r.Close()
	obs := &captureAll{}
	r.AddObserver(obs)

	handler := r.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	for i := 0; i < 5; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		obs.mu.Lock()
		n := len(obs.entries)
		obs.mu.Unlock()
		if n == 5 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	obs.mu.Lock()
	defer obs.mu.Unlock()
	t.Fatalf("observer received %d entries, want 5", len(obs.entries))
}

func TestExportRoundTripsEntry(t *testing.T) {
	t.Parallel()
	r := NewRecorder(Config{CaptureBodies: true, TwinName: "twin-stripe", TwinVersion: "0.1.0", Seed: 42})
	defer r.Close()
	r.StartRun("run-42")

	handler := r.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/charges?limit=10", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	var buf bytes.Buffer
	if err := r.Export(&buf); err != nil {
		t.Fatalf("Export error: %v", err)
	}

	sc := bufio.NewScanner(&buf)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	if !sc.Scan() {
		t.Fatal("no manifest line")
	}
	var m Manifest
	if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
		t.Fatalf("manifest unmarshal: %v", err)
	}
	if m.TwinName != "twin-stripe" || m.RunID != "run-42" || m.SchemaVersion != SchemaVersion {
		t.Errorf("manifest mismatch: %+v", m)
	}
	if !sc.Scan() {
		t.Fatal("no entry line")
	}
	var e Entry
	if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
		t.Fatalf("entry unmarshal: %v", err)
	}
	if e.Path != "/v1/charges" || e.Query != "limit=10" || e.RunID != "run-42" {
		t.Errorf("entry mismatch: %+v", e)
	}
	if string(e.ResBody) != `{"ok":true}` {
		t.Errorf("ResBody round-trip = %q", e.ResBody)
	}
}

type quirkSrc struct{ q []string }

func (q quirkSrc) ActiveQuirks() []string { return q.q }

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

func TestEntryQuirksAndSimTime(t *testing.T) {
	t.Parallel()
	want := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	r := NewRecorder(Config{Quirks: quirkSrc{q: []string{"slow_payouts"}}, Clock: fixedClock{t: want}})
	defer r.Close()
	handler := r.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	entries := r.Entries()
	if len(entries[0].QuirksActive) != 1 || entries[0].QuirksActive[0] != "slow_payouts" {
		t.Errorf("QuirksActive = %v", entries[0].QuirksActive)
	}
	if !entries[0].SimTime.Equal(want) {
		t.Errorf("SimTime = %v, want %v", entries[0].SimTime, want)
	}
}
