package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// ---------------------------------------------------------------------------
// Mock state store
// ---------------------------------------------------------------------------

type mockState struct {
	data        map[string]string
	resetCalled bool
}

func newMockState() *mockState {
	return &mockState{data: map[string]string{"key": "value"}}
}

func (m *mockState) Snapshot() any {
	return m.data
}

func (m *mockState) LoadState(data []byte) error {
	var d map[string]string
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}
	m.data = d
	return nil
}

func (m *mockState) Reset() {
	m.resetCalled = true
	m.data = map[string]string{"key": "value"} // reset to default
}

// ---------------------------------------------------------------------------
// Mock webhook flusher
// ---------------------------------------------------------------------------

type mockFlusher struct {
	flushErr error
	flushed  bool
}

func (m *mockFlusher) FlushWebhooks() error {
	m.flushed = true
	return m.flushErr
}

// ---------------------------------------------------------------------------
// Mock config provider
// ---------------------------------------------------------------------------

type mockConfigProvider struct {
	cfg map[string]any
}

func newMockConfigProvider() *mockConfigProvider {
	return &mockConfigProvider{cfg: map[string]any{
		"port":    8080,
		"latency": "100ms",
		"verbose": false,
	}}
}

func (m *mockConfigProvider) GetConfig() map[string]any {
	return m.cfg
}

func (m *mockConfigProvider) UpdateConfig(updates map[string]any) error {
	for k, v := range updates {
		m.cfg[k] = v
	}
	return nil
}

// ---------------------------------------------------------------------------
// Mock quirk store
// ---------------------------------------------------------------------------

type mockQuirkStore struct {
	quirks []QuirkStatus
}

func newMockQuirkStore() *mockQuirkStore {
	return &mockQuirkStore{quirks: []QuirkStatus{
		{ID: "q1", Summary: "Test quirk", Enabled: false, Type: "behavior", Severity: "low"},
		{ID: "q2", Summary: "Another quirk", Enabled: true, Type: "timing", Severity: "medium"},
	}}
}

func (m *mockQuirkStore) ListQuirks() []QuirkStatus {
	return m.quirks
}

func (m *mockQuirkStore) EnableQuirk(id string) error {
	for i, q := range m.quirks {
		if q.ID == id {
			m.quirks[i].Enabled = true
			return nil
		}
	}
	return fmt.Errorf("unknown quirk: %s", id)
}

func (m *mockQuirkStore) DisableQuirk(id string) error {
	for i, q := range m.quirks {
		if q.ID == id {
			m.quirks[i].Enabled = false
			return nil
		}
	}
	return fmt.Errorf("unknown quirk: %s", id)
}

func (m *mockQuirkStore) IsEnabled(id string) bool {
	for _, q := range m.quirks {
		if q.ID == id {
			return q.Enabled
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Helper to create a test server
// ---------------------------------------------------------------------------

type testServerOpts struct {
	state   StateStore
	clock   *store.Clock
	flusher WebhookFlusher
	config  ConfigProvider
	quirks  QuirkStore
}

func setupTestServer(state StateStore, clock *store.Clock, flusher WebhookFlusher) *httptest.Server {
	return setupTestServerFull(testServerOpts{state: state, clock: clock, flusher: flusher})
}

func setupTestServerFull(opts testServerOpts) *httptest.Server {
	if opts.state == nil {
		opts.state = newMockState()
	}
	cfg := &twincore.Config{Name: "test-admin"}
	mw := twincore.NewMiddleware(cfg, nil)

	h := NewHandler(opts.state, mw, opts.clock)
	if opts.flusher != nil {
		h.SetFlusher(opts.flusher)
	}
	if opts.config != nil {
		h.SetConfigProvider(opts.config)
	}
	if opts.quirks != nil {
		h.SetQuirkStore(opts.quirks)
	}

	r := chi.NewRouter()
	h.Routes(r)

	return httptest.NewServer(r)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestHandleHealth(t *testing.T) {
	srv := setupTestServer(newMockState(), store.NewClock(), nil)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/admin/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %+v", body)
	}
}

func TestHandleReset(t *testing.T) {
	state := newMockState()
	clk := store.NewClock()
	clk.Advance(1000)

	srv := setupTestServer(state, clk, nil)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/admin/reset", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !state.resetCalled {
		t.Error("expected state Reset to be called")
	}
	if clk.Offset() != 0 {
		t.Errorf("expected clock offset to be reset, got %v", clk.Offset())
	}
}

func TestHandleGetState(t *testing.T) {
	srv := setupTestServer(newMockState(), nil, nil)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/admin/state")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["key"] != "value" {
		t.Errorf("expected key=value, got %+v", body)
	}
}

func TestHandleLoadState(t *testing.T) {
	state := newMockState()
	srv := setupTestServer(state, nil, nil)
	defer srv.Close()

	newState := `{"foo":"bar"}`
	resp, err := http.Post(srv.URL+"/admin/state", "application/json", strings.NewReader(newState))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if state.data["foo"] != "bar" {
		t.Errorf("expected state to be updated, got %+v", state.data)
	}
}

func TestHandleLoadStateInvalid(t *testing.T) {
	srv := setupTestServer(newMockState(), nil, nil)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/admin/state", "application/json", strings.NewReader("{bad json"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandleInjectFault(t *testing.T) {
	cfg := &twincore.Config{Name: "test"}
	mw := twincore.NewMiddleware(cfg, nil)
	state := newMockState()

	h := NewHandler(state, mw, nil)
	r := chi.NewRouter()
	h.Routes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := `{"status_code":503,"rate":1.0}`
	resp, err := http.Post(srv.URL+"/admin/fault/charges", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Verify the fault was registered (handler prepends "/" to the endpoint param)
	fault := mw.Faults.Check("/charges")
	if fault == nil {
		t.Fatal("expected fault to be registered")
	}
	if fault.StatusCode != 503 {
		t.Errorf("expected status 503, got %d", fault.StatusCode)
	}
}

func TestHandleInjectFaultInvalidBody(t *testing.T) {
	srv := setupTestServer(newMockState(), nil, nil)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/admin/fault/test", "application/json", strings.NewReader("{bad"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandleRemoveFault(t *testing.T) {
	cfg := &twincore.Config{Name: "test"}
	mw := twincore.NewMiddleware(cfg, nil)
	mw.Faults.Set("/test", twincore.FaultConfig{StatusCode: 500, Rate: 1.0})

	state := newMockState()
	h := NewHandler(state, mw, nil)
	r := chi.NewRouter()
	h.Routes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/admin/fault/test", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	if mw.Faults.Check("/test") != nil {
		t.Error("expected fault to be removed")
	}
}

func TestHandleRemoveFaultNotFound(t *testing.T) {
	srv := setupTestServer(newMockState(), nil, nil)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/admin/fault/nonexistent", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandleListFaults(t *testing.T) {
	cfg := &twincore.Config{Name: "test"}
	mw := twincore.NewMiddleware(cfg, nil)
	mw.Faults.Set("/a", twincore.FaultConfig{StatusCode: 500, Rate: 1.0})

	state := newMockState()
	h := NewHandler(state, mw, nil)
	r := chi.NewRouter()
	h.Routes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/admin/faults")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]twincore.FaultConfig
	json.NewDecoder(resp.Body).Decode(&body)
	if _, ok := body["/a"]; !ok {
		t.Errorf("expected fault /a in listing, got %+v", body)
	}
}

func TestHandleGetRequests(t *testing.T) {
	cfg := &twincore.Config{Name: "test"}
	mw := twincore.NewMiddleware(cfg, nil)
	mw.ReqLog.Add(twincore.RequestLogEntry{Method: "GET", Path: "/test"})

	state := newMockState()
	h := NewHandler(state, mw, nil)
	r := chi.NewRouter()
	h.Routes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/admin/requests")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body []twincore.RequestLogEntry
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(body))
	}
	if body[0].Method != "GET" {
		t.Errorf("expected GET, got %s", body[0].Method)
	}
}

func TestHandleTimeAdvance(t *testing.T) {
	clk := store.NewClock()
	srv := setupTestServer(newMockState(), clk, nil)
	defer srv.Close()

	body := `{"duration":"1h"}`
	resp, err := http.Post(srv.URL+"/admin/time/advance", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "advanced" {
		t.Errorf("expected status=advanced, got %v", result["status"])
	}
}

func TestHandleTimeAdvanceNoClock(t *testing.T) {
	srv := setupTestServer(newMockState(), nil, nil)
	defer srv.Close()

	body := `{"duration":"1h"}`
	resp, err := http.Post(srv.URL+"/admin/time/advance", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 when no clock configured, got %d", resp.StatusCode)
	}
}

func TestHandleTimeAdvanceInvalidDuration(t *testing.T) {
	clk := store.NewClock()
	srv := setupTestServer(newMockState(), clk, nil)
	defer srv.Close()

	body := `{"duration":"not-a-duration"}`
	resp, err := http.Post(srv.URL+"/admin/time/advance", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid duration, got %d", resp.StatusCode)
	}
}

func TestHandleTimeAdvanceInvalidJSON(t *testing.T) {
	clk := store.NewClock()
	srv := setupTestServer(newMockState(), clk, nil)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/admin/time/advance", "application/json", strings.NewReader("{bad"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestHandleGetTime(t *testing.T) {
	clk := store.NewClock()
	srv := setupTestServer(newMockState(), clk, nil)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/admin/time")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if _, ok := body["real"]; !ok {
		t.Error("expected 'real' field")
	}
	if _, ok := body["simulated"]; !ok {
		t.Error("expected 'simulated' field")
	}
	if _, ok := body["offset"]; !ok {
		t.Error("expected 'offset' field")
	}
}

func TestHandleGetTimeNoClock(t *testing.T) {
	srv := setupTestServer(newMockState(), nil, nil)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/admin/time")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if _, ok := body["real"]; !ok {
		t.Error("expected 'real' field")
	}
	if _, ok := body["simulated"]; ok {
		t.Error("did not expect 'simulated' field when no clock")
	}
}

func TestHandleFlushWebhooksNoFlusher(t *testing.T) {
	srv := setupTestServer(newMockState(), nil, nil)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/admin/webhooks/flush", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "no webhooks configured" {
		t.Errorf("unexpected status: %s", body["status"])
	}
}

func TestHandleFlushWebhooksSuccess(t *testing.T) {
	flusher := &mockFlusher{}
	srv := setupTestServer(newMockState(), nil, flusher)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/admin/webhooks/flush", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !flusher.flushed {
		t.Error("expected FlushWebhooks to be called")
	}
}

func TestHandleFlushWebhooksError(t *testing.T) {
	flusher := &mockFlusher{flushErr: fmt.Errorf("delivery failed")}
	srv := setupTestServer(newMockState(), nil, flusher)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/admin/webhooks/flush", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestHandleResetWithNilClock(t *testing.T) {
	state := newMockState()
	srv := setupTestServer(state, nil, nil)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/admin/reset", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !state.resetCalled {
		t.Error("expected state Reset to be called")
	}
}

// ---------------------------------------------------------------------------
// Config endpoint tests
// ---------------------------------------------------------------------------

func TestHandleGetConfig(t *testing.T) {
	srv := setupTestServerFull(testServerOpts{config: newMockConfigProvider()})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/admin/config")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["port"] != float64(8080) {
		t.Errorf("expected port=8080, got %v", body["port"])
	}
}

func TestHandleGetConfigNilProvider(t *testing.T) {
	srv := setupTestServer(newMockState(), nil, nil)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/admin/config")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 when no config provider, got %d", resp.StatusCode)
	}
}

func TestHandleUpdateConfig(t *testing.T) {
	cp := newMockConfigProvider()
	srv := setupTestServerFull(testServerOpts{config: cp})
	defer srv.Close()

	body := `{"verbose": true, "latency": "200ms"}`
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/admin/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	if cp.cfg["verbose"] != true {
		t.Errorf("expected verbose=true, got %v", cp.cfg["verbose"])
	}
	if cp.cfg["latency"] != "200ms" {
		t.Errorf("expected latency=200ms, got %v", cp.cfg["latency"])
	}
}

func TestHandleUpdateConfigNilProvider(t *testing.T) {
	srv := setupTestServer(newMockState(), nil, nil)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/admin/config", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 when no config provider, got %d", resp.StatusCode)
	}
}

func TestHandleUpdateConfigInvalidJSON(t *testing.T) {
	srv := setupTestServerFull(testServerOpts{config: newMockConfigProvider()})
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/admin/config", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Quirk endpoint tests
// ---------------------------------------------------------------------------

func TestHandleListQuirks(t *testing.T) {
	srv := setupTestServerFull(testServerOpts{quirks: newMockQuirkStore()})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/admin/quirks")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body []QuirkStatus
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body) != 2 {
		t.Fatalf("expected 2 quirks, got %d", len(body))
	}
}

func TestHandleListQuirksNilStore(t *testing.T) {
	srv := setupTestServer(newMockState(), nil, nil)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/admin/quirks")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body []QuirkStatus
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body) != 0 {
		t.Errorf("expected empty list, got %d items", len(body))
	}
}

func TestHandleEnableQuirk(t *testing.T) {
	qs := newMockQuirkStore()
	srv := setupTestServerFull(testServerOpts{quirks: qs})
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/admin/quirks/q1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !qs.IsEnabled("q1") {
		t.Error("expected q1 to be enabled")
	}
}

func TestHandleEnableQuirkNotFound(t *testing.T) {
	srv := setupTestServerFull(testServerOpts{quirks: newMockQuirkStore()})
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/admin/quirks/nonexistent", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandleEnableQuirkNilStore(t *testing.T) {
	srv := setupTestServer(newMockState(), nil, nil)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/admin/quirks/q1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 when no quirk store, got %d", resp.StatusCode)
	}
}

func TestHandleDisableQuirk(t *testing.T) {
	qs := newMockQuirkStore()
	srv := setupTestServerFull(testServerOpts{quirks: qs})
	defer srv.Close()

	// q2 is initially enabled
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/admin/quirks/q2", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if qs.IsEnabled("q2") {
		t.Error("expected q2 to be disabled")
	}
}

func TestHandleDisableQuirkNotFound(t *testing.T) {
	srv := setupTestServerFull(testServerOpts{quirks: newMockQuirkStore()})
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/admin/quirks/nonexistent", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandleDisableQuirkNilStore(t *testing.T) {
	srv := setupTestServer(newMockState(), nil, nil)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/admin/quirks/q1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 when no quirk store, got %d", resp.StatusCode)
	}
}
