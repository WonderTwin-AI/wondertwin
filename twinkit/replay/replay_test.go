package replay

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRecorderAppliesDefaults(t *testing.T) {
	t.Parallel()
	r := NewRecorder(Config{})
	cfg := r.Config()
	if cfg.MaxEntries != 10_000 {
		t.Errorf("MaxEntries default = %d, want 10000", cfg.MaxEntries)
	}
	if cfg.MaxBodyBytes != 64*1024 {
		t.Errorf("MaxBodyBytes default = %d, want 65536", cfg.MaxBodyBytes)
	}
	if cfg.Scrubbers == nil {
		t.Error("Scrubbers default = nil, want non-nil default set")
	}
}

func TestMiddlewareIsPassthrough(t *testing.T) {
	t.Parallel()
	r := NewRecorder(Config{})

	handler := r.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hi"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}
	if rec.Body.String() != "hi" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "hi")
	}
}

type captureObserver struct {
	entries []Entry
}

func (c *captureObserver) Observe(e Entry) { c.entries = append(c.entries, e) }

func TestAddObserverAcceptsMultiple(t *testing.T) {
	t.Parallel()
	r := NewRecorder(Config{})
	r.AddObserver(&captureObserver{})
	r.AddObserver(&captureObserver{})
	r.AddObserver(nil) // must not panic
}

func TestStartFinishRun(t *testing.T) {
	t.Parallel()
	r := NewRecorder(Config{})
	defer r.Close()
	r.StartRun("run-1")
	m := r.FinishRun()
	if m.RunID != "run-1" {
		t.Errorf("Manifest.RunID = %q, want run-1", m.RunID)
	}
	if m.StartedAt.IsZero() || m.FinishedAt.IsZero() {
		t.Error("Manifest timestamps must be set")
	}
	if m.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", m.SchemaVersion, SchemaVersion)
	}
}

func TestExportEmptyWritesManifestOnly(t *testing.T) {
	t.Parallel()
	r := NewRecorder(Config{})
	defer r.Close()
	var buf bytes.Buffer
	if err := r.Export(&buf); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Export should write at least the manifest line")
	}
	if got := r.Entries(); len(got) != 0 {
		t.Errorf("Entries() = %d, want 0", len(got))
	}
}
