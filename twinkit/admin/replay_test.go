package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/replay"
	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func newReplayServer(t *testing.T, rec *replay.Recorder) *httptest.Server {
	t.Helper()
	cfg := &twincore.Config{Name: "test-admin"}
	mw := twincore.NewMiddleware(cfg, nil)
	h := NewHandler(newMockState(), mw, pkgstate.NewClock())
	if rec != nil {
		h.SetReplayRecorder(rec)
	}
	r := chi.NewRouter()
	h.Routes(r)
	return httptest.NewServer(r)
}

func TestReplayGetReturnsEmptyManifestWhenNoRecorder(t *testing.T) {
	srv := newReplayServer(t, nil)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/admin/replay")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var m replay.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m.SchemaVersion != replay.SchemaVersion {
		t.Errorf("schema_version = %d, want %d", m.SchemaVersion, replay.SchemaVersion)
	}
}

func TestReplayStartFinish404WhenNoRecorder(t *testing.T) {
	srv := newReplayServer(t, nil)
	defer srv.Close()
	for _, path := range []string{"/admin/replay/start", "/admin/replay/finish"} {
		resp, err := http.Post(srv.URL+path, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", path, resp.StatusCode)
		}
	}
}

func TestReplayLifecycleWithRecorder(t *testing.T) {
	rec := replay.NewRecorder(replay.Config{TwinName: "twin-x", CaptureBodies: true})
	defer rec.Close()

	srv := newReplayServer(t, rec)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/admin/replay/start?run_id=run-99", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start status = %d", resp.StatusCode)
	}

	// Drive one captured request through the recorder middleware.
	mw := rec.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	mw.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/things", nil))

	// Manifest endpoint.
	mResp, err := http.Get(srv.URL + "/admin/replay?format=manifest")
	if err != nil {
		t.Fatal(err)
	}
	var m replay.Manifest
	if err := json.NewDecoder(mResp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	mResp.Body.Close()
	if m.RunID != "run-99" {
		t.Errorf("manifest RunID = %q", m.RunID)
	}
	if m.EntryCount != 1 {
		t.Errorf("manifest EntryCount = %d, want 1", m.EntryCount)
	}

	// JSONL endpoint.
	jResp, err := http.Get(srv.URL + "/admin/replay")
	if err != nil {
		t.Fatal(err)
	}
	defer jResp.Body.Close()
	if ct := jResp.Header.Get("Content-Type"); !strings.Contains(ct, "ndjson") {
		t.Errorf("Content-Type = %q, want ndjson", ct)
	}
	body, _ := io.ReadAll(jResp.Body)
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d: %s", len(lines), body)
	}

	fResp, err := http.Post(srv.URL+"/admin/replay/finish", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fResp.Body.Close()
	var finish replay.Manifest
	if err := json.NewDecoder(fResp.Body).Decode(&finish); err != nil {
		t.Fatal(err)
	}
	if finish.FinishedAt.IsZero() {
		t.Error("FinishedAt should be set")
	}
}

func TestReplayStartReadsBody(t *testing.T) {
	rec := replay.NewRecorder(replay.Config{})
	defer rec.Close()
	srv := newReplayServer(t, rec)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/admin/replay/start", "application/json", strings.NewReader(`{"run_id":"from-body"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := rec.CurrentManifest().RunID; got != "from-body" {
		t.Errorf("RunID = %q, want from-body", got)
	}
}
