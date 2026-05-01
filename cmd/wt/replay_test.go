package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleBundle = `{"twin_name":"twin-stripe","twin_version":"0.1.0","run_id":"run-1","schema_version":1,"started_at":"2026-05-01T12:00:00Z","entry_count":2}
{"seq":0,"timestamp":"2026-05-01T12:00:00Z","method":"POST","path":"/v1/charges","res_status":201,"duration_ns":1234000}
{"seq":1,"timestamp":"2026-05-01T12:00:01Z","method":"GET","path":"/v1/charges/ch_1","res_status":200,"duration_ns":987000}
`

func TestReplayViewParsesBundle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.jsonl")
	if err := os.WriteFile(path, []byte(sampleBundle), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := cmdReplayView([]string{path}); err != nil {
			t.Fatalf("cmdReplayView error: %v", err)
		}
	})
	for _, want := range []string{"twin-stripe", "run-1", "POST", "/v1/charges", "201"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q; full output:\n%s", want, out)
		}
	}
}

func TestReplayViewHandlesGzip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.jsonl.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	gz.Write([]byte(sampleBundle))
	gz.Close()
	f.Close()

	out := captureStdout(t, func() {
		if err := cmdReplayView([]string{path}); err != nil {
			t.Fatalf("cmdReplayView gzip error: %v", err)
		}
	})
	if !strings.Contains(out, "/v1/charges") {
		t.Errorf("expected gzip bundle to decode; output:\n%s", out)
	}
}

func TestReplayExportFetches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.Copy(w, strings.NewReader(sampleBundle))
	}))
	defer srv.Close()

	dir := t.TempDir()
	out := filepath.Join(dir, "captured.jsonl")
	if err := cmdReplayExport([]string{"--twin", srv.URL, "--output", out}); err != nil {
		t.Fatalf("cmdReplayExport error: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != sampleBundle {
		t.Errorf("captured bundle mismatch: %q", got)
	}
}

func TestReplayViewGoldenSample(t *testing.T) {
	out := captureStdout(t, func() {
		if err := cmdReplayView([]string{"../../twinkit/replay/testdata/sample-run.jsonl"}); err != nil {
			t.Fatalf("cmdReplayView error: %v", err)
		}
	})
	for _, want := range []string{"twin-stripe", "run-2026-05-01-001", "/v1/customers", "/v1/payment_intents", "404", "201"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected golden output to contain %q; output:\n%s", want, out)
		}
	}
}

func TestReplayExportGzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.Copy(w, strings.NewReader(sampleBundle))
	}))
	defer srv.Close()

	dir := t.TempDir()
	out := filepath.Join(dir, "captured.jsonl.gz")
	if err := cmdReplayExport([]string{"--twin", srv.URL, "--output", out, "--gzip"}); err != nil {
		t.Fatalf("export gzip error: %v", err)
	}
	f, _ := os.Open(out)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("not gzip: %v", err)
	}
	defer gz.Close()
	got, _ := io.ReadAll(gz)
	if string(got) != sampleBundle {
		t.Errorf("decompressed bundle mismatch: %q", got)
	}
}

// captureStdout temporarily redirects stdout for the duration of fn and
// returns the captured output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	fn()
	w.Close()
	<-done
	os.Stdout = orig
	return buf.String()
}
