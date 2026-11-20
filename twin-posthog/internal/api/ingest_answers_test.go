package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func bodyOf(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

// TestIngestAnswers holds the capture service's answers on the legacy ingest
// routes: {"status":"Ok"} on success, text/plain on rejection, an unknown but
// well-formed token accepted, and gzip bodies read on /batch/ and /e/
// (ph-br-004, ph-ee-014, ph-ee-007, ph-ee-001; D4, D10, D13).
func TestIngestAnswers(t *testing.T) {
	event := map[string]any{"api_key": "phc_unknown_but_well_formed", "event": "e1", "distinct_id": "u1"}
	eventJSON, _ := json.Marshal(event)
	batchJSON, _ := json.Marshal(map[string]any{
		"api_key": "phc_unknown_but_well_formed",
		"batch":   []map[string]any{{"event": "b1", "distinct_id": "u1"}},
	})

	t.Run("success body is status Ok on every capture route", func(t *testing.T) {
		srv, _ := setupPostHog(t)
		for _, tc := range []struct {
			path string
			body []byte
		}{
			{"/capture/", eventJSON}, {"/e/", eventJSON}, {"/batch/", batchJSON},
		} {
			resp := postRaw(t, srv.URL, tc.path, "application/json", tc.body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("%s: status %d, want 200", tc.path, resp.StatusCode)
			}
			if got := bodyOf(t, resp); got != `{"status":"Ok"}` {
				t.Errorf("%s: body %q, want {\"status\":\"Ok\"}", tc.path, got)
			}
		}
	})

	t.Run("rejections are text/plain", func(t *testing.T) {
		srv, _ := setupPostHog(t)
		for _, tc := range []struct {
			name   string
			path   string
			body   []byte
			status int
			want   string
		}{
			{"no key", "/e/", []byte(`{"event":"e","distinct_id":"u"}`), 401, "event submitted without an api_key"},
			{"no event name", "/e/", []byte(`{"api_key":"phc_k","distinct_id":"u"}`), 400,
				"failed to hydrate events from request: non-engage request missing event name attribute"},
			{"malformed", "/e/", []byte(`{`), 400, "failed to parse request: "},
			{"empty body", "/e/", nil, 400, "request missing data payload"},
			{"empty batch body", "/batch/", nil, 400, "request missing data payload"},
		} {
			resp := postRaw(t, srv.URL, tc.path, "application/json", tc.body)
			if resp.StatusCode != tc.status {
				t.Errorf("%s: status %d, want %d", tc.name, resp.StatusCode, tc.status)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
				t.Errorf("%s: content type %q, want text/plain", tc.name, ct)
			}
			if got := bodyOf(t, resp); !strings.HasPrefix(got, tc.want) {
				t.Errorf("%s: body %q, want prefix %q", tc.name, got, tc.want)
			}
		}
	})

	t.Run("unknown well-formed token is accepted", func(t *testing.T) {
		srv, _ := setupPostHog(t)
		resp := postRaw(t, srv.URL, "/e/", "application/json", eventJSON)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d, want 200 (no token check on capture, ph-ee-007)", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("gzip bodies are read on /batch/ and /e/", func(t *testing.T) {
		srv, tc := setupPostHog(t)
		for _, c := range []struct {
			path, event string
			body        []byte
		}{
			{"/e/?compression=gzip", "e1", eventJSON},
			{"/batch/?compression=gzip", "b1", batchJSON},
		} {
			resp := postRaw(t, srv.URL, c.path, "text/plain", gzipBytes(t, c.body))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("%s: status %d, want 200", c.path, resp.StatusCode)
			}
			resp.Body.Close()
			r := tc.Get("/admin/events?event=" + c.event)
			r.AssertStatus(200)
			if n := len(r.JSONMap()["events"].([]any)); n != 1 {
				t.Errorf("%s: %d stored %s events, want 1", c.path, n, c.event)
			}
		}
	})
}
