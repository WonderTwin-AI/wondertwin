package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/replay"
	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// stubReseeder records every Reseed call.
type stubReseeder struct {
	mu    sync.Mutex
	seeds []int64
}

func (s *stubReseeder) Reseed(seed int64) {
	s.mu.Lock()
	s.seeds = append(s.seeds, seed)
	s.mu.Unlock()
}

// stubAttribution returns canned attribution.
type stubAttribution struct{}

func (stubAttribution) TwinName() string      { return "twin-x" }
func (stubAttribution) TwinVersion() string   { return "0.0.1" }
func (stubAttribution) ModeString() string    { return "ci" }
func (stubAttribution) Platform() string      { return "github-actions" }
func (stubAttribution) OrgHash() string       { return "deadbeefcafe1234" }
func (stubAttribution) LicenseID() string     { return "" }
func (stubAttribution) LicenseOK() bool       { return false }
func (stubAttribution) LicenseReason() string { return "no license" }

// stubEmitter captures telemetry events.
type stubEmitter struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *stubEmitter) Record(ev telemetry.Event) {
	s.mu.Lock()
	s.events = append(s.events, ev)
	s.mu.Unlock()
}

func (s *stubEmitter) snapshot() []telemetry.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]telemetry.Event, len(s.events))
	copy(out, s.events)
	return out
}

func newRunsServer(t *testing.T) (*httptest.Server, *replay.Recorder, *stubReseeder, *stubEmitter, *mockState) {
	t.Helper()
	cfg := &twincore.Config{Name: "twin-x"}
	mw := twincore.NewMiddleware(cfg, nil)
	st := newMockState()
	clk := pkgstate.NewClock()

	rec := replay.NewRecorder(replay.Config{TwinName: "twin-x", CaptureBodies: false})
	t.Cleanup(rec.Close)

	rs := &stubReseeder{}
	em := &stubEmitter{}

	h := NewHandler(st, mw, clk)
	h.SetReseeder(rs)
	h.SetReplayRecorder(rec)
	h.SetTelemetryEmitter(em)
	h.SetRunAttribution(stubAttribution{})
	h.SetRunIDSetter(&stubRunID{})

	r := chi.NewRouter()
	h.Routes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, rec, rs, em, st
}

type stubRunID struct{ id string }

func (s *stubRunID) SetRunID(id string) { s.id = id }

func TestRunStartAppliesSeedAndFixtures(t *testing.T) {
	srv, rec, rs, em, st := newRunsServer(t)
	body := `{"run_id":"r-1","seed":42,"fixtures":{"key":"loaded"}}`
	resp, err := http.Post(srv.URL+"/admin/runs/start", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	if out["run_id"] != "r-1" || out["seeded"] != true || out["seed_source"] != "byo" {
		t.Errorf("response = %+v", out)
	}
	if len(rs.seeds) != 1 || rs.seeds[0] != 42 {
		t.Errorf("Reseed seeds = %v", rs.seeds)
	}
	if rec.CurrentManifest().RunID != "r-1" {
		t.Errorf("recorder run = %q", rec.CurrentManifest().RunID)
	}
	if !st.resetCalled {
		t.Error("state.Reset should have been called")
	}
	if v, ok := st.data["key"]; !ok || v != "loaded" {
		t.Errorf("fixtures not loaded; data = %+v", st.data)
	}
	events := em.snapshot()
	if len(events) != 1 || events[0].Type != "run_start" {
		t.Errorf("events = %+v", events)
	}
}

func TestRunStartAutoFinishesPreviousRun(t *testing.T) {
	srv, rec, _, em, _ := newRunsServer(t)
	for _, runID := range []string{"r-a", "r-b"} {
		body, _ := json.Marshal(map[string]any{"run_id": runID})
		resp, err := http.Post(srv.URL+"/admin/runs/start", "application/json", strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if rec.CurrentManifest().RunID != "r-b" {
		t.Errorf("current = %q", rec.CurrentManifest().RunID)
	}
	events := em.snapshot()
	// Sequence: run_start(r-a), run_finish(r-a), run_start(r-b)
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3: %+v", len(events), events)
	}
	if events[0].Type != "run_start" || events[0].RunID != "r-a" {
		t.Errorf("events[0] = %+v", events[0])
	}
	if events[1].Type != "run_finish" || events[1].RunID != "r-a" {
		t.Errorf("events[1] = %+v", events[1])
	}
	if events[2].Type != "run_start" || events[2].RunID != "r-b" {
		t.Errorf("events[2] = %+v", events[2])
	}
}

func TestRunFinishMismatchReturns409(t *testing.T) {
	srv, _, _, _, _ := newRunsServer(t)
	http.Post(srv.URL+"/admin/runs/start", "application/json", strings.NewReader(`{"run_id":"r-current"}`))
	resp, err := http.Post(srv.URL+"/admin/runs/finish", "application/json", strings.NewReader(`{"run_id":"r-other"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Errorf("status = %d, want 409", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "mismatch") {
		t.Errorf("body = %s", body)
	}
}

func TestRunFinishNoActiveRunReturns404(t *testing.T) {
	srv, _, _, _, _ := newRunsServer(t)
	resp, err := http.Post(srv.URL+"/admin/runs/finish", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestRunCurrentEmptyWhenNoRun(t *testing.T) {
	srv, _, _, _, _ := newRunsServer(t)
	resp, err := http.Get(srv.URL + "/admin/runs/current")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	var out currentRunResponse
	json.NewDecoder(resp.Body).Decode(&out)
	if out.RunID != "" || out.EntryCount != 0 {
		t.Errorf("expected empty; got %+v", out)
	}
}

func TestRunFinishReturnsManifestAndEmitsEvent(t *testing.T) {
	srv, _, _, em, _ := newRunsServer(t)
	http.Post(srv.URL+"/admin/runs/start", "application/json", strings.NewReader(`{"run_id":"r-1"}`))
	resp, err := http.Post(srv.URL+"/admin/runs/finish", "application/json", strings.NewReader(`{"run_id":"r-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m replay.Manifest
	json.NewDecoder(resp.Body).Decode(&m)
	if m.RunID != "r-1" {
		t.Errorf("manifest = %+v", m)
	}
	events := em.snapshot()
	last := events[len(events)-1]
	if last.Type != "run_finish" || last.RunID != "r-1" {
		t.Errorf("last event = %+v", last)
	}
}

func TestRunStartGeneratesIDWhenAbsent(t *testing.T) {
	srv, _, _, _, _ := newRunsServer(t)
	resp, err := http.Post(srv.URL+"/admin/runs/start", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out startRunResponse
	json.NewDecoder(resp.Body).Decode(&out)
	if !strings.HasPrefix(out.RunID, "wt-run-") {
		t.Errorf("auto-generated id = %q", out.RunID)
	}
}
