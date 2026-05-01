package twincore

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/replay"
)

func TestReplayRoutesAutoWiredFromNew(t *testing.T) {
	twin := New(&Config{Name: "twin-test"})
	if twin.Replay == nil {
		t.Fatal("Twin.Replay must be non-nil after New")
	}
	srv := httptest.NewServer(twin.Router)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/admin/replay/start?run_id=run-1", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start status = %d", resp.StatusCode)
	}
	if got := twin.RunID; got != "run-1" {
		t.Errorf("Twin.RunID = %q, want run-1", got)
	}

	// Drive a request through the router so a captured entry exists.
	twin.Router.Get("/v1/probe", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	if r, err := http.Get(srv.URL + "/v1/probe"); err == nil {
		r.Body.Close()
	}

	mResp, err := http.Get(srv.URL + "/admin/replay?format=manifest")
	if err != nil {
		t.Fatal(err)
	}
	var m replay.Manifest
	if err := json.NewDecoder(mResp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	mResp.Body.Close()
	if m.RunID != "run-1" {
		t.Errorf("manifest RunID = %q", m.RunID)
	}
	if m.EntryCount < 1 {
		t.Errorf("EntryCount = %d, want >=1", m.EntryCount)
	}

	jResp, err := http.Get(srv.URL + "/admin/replay")
	if err != nil {
		t.Fatal(err)
	}
	defer jResp.Body.Close()
	if ct := jResp.Header.Get("Content-Type"); !strings.Contains(ct, "ndjson") {
		t.Errorf("Content-Type = %q, want ndjson", ct)
	}
	body, _ := io.ReadAll(jResp.Body)
	lines := strings.Count(strings.TrimSpace(string(body)), "\n") + 1
	if lines < 2 {
		t.Errorf("expected manifest + at least one entry; got %d lines", lines)
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
