package main

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// stubTwin returns a server that satisfies the /admin/runs/* and
// /admin/replay surface used by wt runs.
type stubTwin struct {
	*httptest.Server
	starts        atomic.Int64
	finishes      atomic.Int64
	currents      atomic.Int64
	lastStartBody atomic.Pointer[startRunRequest]
	finishStatus  int // override status (0 → 200)
}

func newStubTwin(t *testing.T) *stubTwin {
	t.Helper()
	s := &stubTwin{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/runs/start":
			var body startRunRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.lastStartBody.Store(&body)
			s.starts.Add(1)
			runID := body.RunID
			if runID == "" {
				runID = "wt-run-stub-1"
			}
			seedSrc := "none"
			if len(body.Fixtures) > 0 {
				seedSrc = "byo"
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id":      runID,
				"seed":        body.Seed,
				"seeded":      body.Seed != 0,
				"seed_source": seedSrc,
			})
		case "/admin/runs/finish":
			s.finishes.Add(1)
			status := s.finishStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if status == http.StatusOK {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"run_id":      "wt-run-stub-1",
					"entry_count": 3,
				})
			} else {
				_, _ = w.Write([]byte(`{"error":"stub"}`))
			}
		case "/admin/runs/current":
			s.currents.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id":      "wt-run-stub-1",
				"entry_count": 1,
				"seeded":      true,
				"seed_source": "byo",
			})
		case "/admin/replay":
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = io.WriteString(w, `{"schema_version":1,"run_id":"wt-run-stub-1","entry_count":1}`+"\n")
			_, _ = io.WriteString(w, `{"seq":0,"method":"GET","path":"/v1/x"}`+"\n")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func TestRunsStartHappy(t *testing.T) {
	s := newStubTwin(t)
	t.Setenv("WT_TWIN_URL", "")
	t.Setenv("GITHUB_ENV", "")
	if err := cmdRunsStart([]string{"--twin", s.URL, "--seed", "42", "--run-id", "smoke-1"}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if s.starts.Load() != 1 {
		t.Fatalf("expected 1 start; got %d", s.starts.Load())
	}
	body := s.lastStartBody.Load()
	if body.RunID != "smoke-1" || body.Seed != 42 {
		t.Errorf("body = %+v", body)
	}
}

func TestRunsStartFixtures(t *testing.T) {
	s := newStubTwin(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "fx.json")
	os.WriteFile(path, []byte(`{"k":"v"}`), 0o600)
	if err := cmdRunsStart([]string{"--twin", s.URL, "--fixtures", path}); err != nil {
		t.Fatal(err)
	}
	got := string(s.lastStartBody.Load().Fixtures)
	if !strings.Contains(got, `"k":"v"`) {
		t.Errorf("fixtures forwarded as %q", got)
	}
}

func TestRunsStartRejectsNonJSONFixtures(t *testing.T) {
	s := newStubTwin(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "fx.txt")
	os.WriteFile(path, []byte("not json"), 0o600)
	err := cmdRunsStart([]string{"--twin", s.URL, "--fixtures", path})
	if err == nil || !strings.Contains(err.Error(), "valid JSON") {
		t.Errorf("expected JSON validation error; got %v", err)
	}
}

func TestRunsStartGithubEnv(t *testing.T) {
	s := newStubTwin(t)
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env")
	os.WriteFile(envFile, nil, 0o600)
	t.Setenv("GITHUB_ENV", envFile)
	if err := cmdRunsStart([]string{"--twin", s.URL, "--run-id", "ci-1"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(envFile)
	out := string(got)
	if !strings.Contains(out, "WT_RUN_ID=ci-1") {
		t.Errorf("GITHUB_ENV missing WT_RUN_ID; got: %s", out)
	}
	if !strings.Contains(out, "WT_TWIN_URL="+s.URL) {
		t.Errorf("GITHUB_ENV missing WT_TWIN_URL; got: %s", out)
	}
}

func TestRunsStartGithubEnvAbsent(t *testing.T) {
	s := newStubTwin(t)
	t.Setenv("GITHUB_ENV", "")
	// Should not error or panic when GITHUB_ENV is unset.
	if err := cmdRunsStart([]string{"--twin", s.URL, "--run-id", "ci-2"}); err != nil {
		t.Fatal(err)
	}
}

func TestRunsStartJSONOutput(t *testing.T) {
	s := newStubTwin(t)
	out := captureStdout(t, func() {
		_ = cmdRunsStart([]string{"--twin", s.URL, "--run-id", "id-1", "--json"})
	})
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got["run_id"] != "id-1" {
		t.Errorf("got = %+v", got)
	}
}

func TestRunsFinishExport(t *testing.T) {
	s := newStubTwin(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "bundle.jsonl")
	if err := cmdRunsFinish([]string{"--twin", s.URL, "--run-id", "x", "--export", path}); err != nil {
		t.Fatalf("finish: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "schema_version") {
		t.Errorf("bundle missing manifest: %s", got)
	}
}

func TestRunsFinishExportGzipBySuffix(t *testing.T) {
	s := newStubTwin(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "bundle.jsonl.gz")
	if err := cmdRunsFinish([]string{"--twin", s.URL, "--run-id", "x", "--export", path}); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(path)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("not gzip: %v", err)
	}
	defer gz.Close()
	got, _ := io.ReadAll(gz)
	if !strings.Contains(string(got), "schema_version") {
		t.Errorf("decompressed = %s", got)
	}
}

func TestRunsFinish404Idempotent(t *testing.T) {
	s := newStubTwin(t)
	s.finishStatus = http.StatusNotFound
	if err := cmdRunsFinish([]string{"--twin", s.URL}); err != nil {
		t.Errorf("expected 404 to be treated as success; got %v", err)
	}
}

func TestRunsFinish409Errors(t *testing.T) {
	s := newStubTwin(t)
	s.finishStatus = http.StatusConflict
	err := cmdRunsFinish([]string{"--twin", s.URL, "--run-id", "wrong"})
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("expected mismatch error; got %v", err)
	}
}

func TestRunsFinishStepSummary(t *testing.T) {
	s := newStubTwin(t)
	dir := t.TempDir()
	summary := filepath.Join(dir, "summary.md")
	bundle := filepath.Join(dir, "bundle.jsonl")
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	if err := cmdRunsFinish([]string{"--twin", s.URL, "--run-id", "x", "--export", bundle}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(summary)
	for _, want := range []string{"WonderTwin replay bundle", "wt-run-stub-1", "entry_count"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("summary missing %q; got: %s", want, got)
		}
	}
}

func TestRunsCurrent(t *testing.T) {
	s := newStubTwin(t)
	out := captureStdout(t, func() {
		_ = cmdRunsCurrent([]string{"--twin", s.URL})
	})
	if !strings.Contains(out, "wt-run-stub-1") {
		t.Errorf("missing run id; got: %s", out)
	}
}

func TestRunsCurrentJSON(t *testing.T) {
	s := newStubTwin(t)
	out := captureStdout(t, func() {
		_ = cmdRunsCurrent([]string{"--twin", s.URL, "--json"})
	})
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if got["run_id"] != "wt-run-stub-1" {
		t.Errorf("got = %+v", got)
	}
}

func TestRunsWrapHappyPathAndCleanup(t *testing.T) {
	s := newStubTwin(t)
	dir := t.TempDir()
	bundle := filepath.Join(dir, "wrap.jsonl")
	envOut := filepath.Join(dir, "child-env.txt")

	// Use a successful child so cmdRunsWrap doesn't os.Exit. The
	// child writes its environment to envOut so we can verify the
	// inherited WT_RUN_ID / WT_TWIN_URL.
	err := cmdRunsWrap([]string{
		"--twin", s.URL,
		"--seed", "42",
		"--run-id", "wrap-1",
		"--export", bundle,
		"--",
		"sh", "-c", "env > " + envOut,
	})
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}

	if _, statErr := os.Stat(bundle); statErr != nil {
		t.Errorf("export not written; stat err = %v", statErr)
	}
	envBytes, _ := os.ReadFile(envOut)
	envText := string(envBytes)
	if !strings.Contains(envText, "WT_RUN_ID=wrap-1") {
		t.Errorf("child missing WT_RUN_ID: %s", envText)
	}
	if !strings.Contains(envText, "WT_TWIN_URL=") {
		t.Errorf("child missing WT_TWIN_URL: %s", envText)
	}
	if s.finishes.Load() < 1 {
		t.Errorf("expected finish call after wrap")
	}
}

func TestRunsWrapFinishesOnChildNonZeroExit(t *testing.T) {
	// The headline failure case: a child that exits non-zero. The
	// deferred finish + export must run before the typed error
	// propagates up to main, otherwise CI workflows lose the replay
	// artifact at exactly the moment they need it most.
	s := newStubTwin(t)
	dir := t.TempDir()
	bundle := filepath.Join(dir, "wrap-fail.jsonl")

	err := cmdRunsWrap([]string{
		"--twin", s.URL,
		"--run-id", "wrap-fail-7",
		"--export", bundle,
		"--",
		"sh", "-c", "exit 7",
	})
	var ec ExitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected ExitCodeError, got %T: %v", err, err)
	}
	if ec.Code() != 7 {
		t.Errorf("exit code = %d, want 7", ec.Code())
	}
	if s.finishes.Load() < 1 {
		t.Errorf("finish must fire even when child exits non-zero")
	}
	if _, statErr := os.Stat(bundle); statErr != nil {
		t.Errorf("export not written on non-zero exit; stat err = %v", statErr)
	}
}

func TestRunsWrapFinishesOnChildFailure(t *testing.T) {
	// Validate that when the child exits non-zero, the deferred
	// finish still fires before cmdRunsWrap exits the process.
	// We can't observe os.Exit from a unit test, so verify by
	// checking the recorded finish count via the stub server when
	// the child is a *failing-but-doesn't-actually-call-exit* path
	// — i.e. a missing binary that produces an exec error rather
	// than a non-zero exit code.
	s := newStubTwin(t)
	err := cmdRunsWrap([]string{
		"--twin", s.URL,
		"--run-id", "wrap-fail",
		"--",
		"/nonexistent/binary-that-cannot-be-found",
	})
	if err == nil {
		t.Errorf("expected exec error for missing binary")
	}
	if s.finishes.Load() < 1 {
		t.Errorf("finish must fire even when exec fails; got %d", s.finishes.Load())
	}
}

func TestRunsWrapRequiresCommand(t *testing.T) {
	s := newStubTwin(t)
	err := cmdRunsWrap([]string{"--twin", s.URL})
	if err == nil || !strings.Contains(err.Error(), "<command>") {
		t.Errorf("expected usage error; got %v", err)
	}
}
