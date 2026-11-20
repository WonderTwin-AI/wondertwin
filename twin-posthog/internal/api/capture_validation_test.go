package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

// TestCaptureAndE holds the single-event routes' rejections (D11): /capture/
// and /e/ answer 400 in the capture service's text body for a missing event
// name or distinct_id, as /i/v0/e/ does (ph-ee-014, ph-br-004), read the
// distinct_id from properties the way posthog-js sends it, and keep form,
// base64 and gzip bodies working (D4). Nothing is stored from a rejection.
func TestCaptureAndE(t *testing.T) {
	routes := []string{"/capture/", "/e/"}
	for _, route := range routes {
		route := route
		t.Run(route+" missing distinct_id", func(t *testing.T) {
			srv, tc := setupPostHog(t)
			resp := postRaw(t, srv.URL, route, "application/json", []byte(`{"api_key":"phc_k","event":"pv"}`))
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status %d, want 400", resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
				t.Errorf("content-type %q, want text/plain", ct)
			}
			if got := bodyOf(t, resp); got != "event submitted without a distinct_id" {
				t.Errorf("body %q", got)
			}
			assertNoEvents(t, tc)
		})

		t.Run(route+" missing event name", func(t *testing.T) {
			srv, tc := setupPostHog(t)
			resp := postRaw(t, srv.URL, route, "application/json", []byte(`{"api_key":"phc_k","distinct_id":"u1"}`))
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status %d, want 400", resp.StatusCode)
			}
			if got := bodyOf(t, resp); !strings.Contains(got, "missing event name attribute") {
				t.Errorf("body %q", got)
			}
			assertNoEvents(t, tc)
		})

		t.Run(route+" distinct_id in properties", func(t *testing.T) {
			srv, tc := setupPostHog(t)
			resp := postRaw(t, srv.URL, route, "application/json",
				[]byte(`{"api_key":"phc_k","event":"pv","properties":{"distinct_id":"js-user"}}`))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status %d, want 200 (posthog-js repeats distinct_id in properties)", resp.StatusCode)
			}
			resp.Body.Close()
			r := tc.Get("/admin/events?event=pv")
			r.AssertStatus(200)
			evts := r.JSONMap()["events"].([]any)
			if len(evts) != 1 || evts[0].(map[string]any)["distinct_id"] != "js-user" {
				t.Errorf("event should carry distinct_id js-user: %v", evts)
			}
		})

		t.Run(route+" long distinct_id is cut to 200", func(t *testing.T) {
			srv, tc := setupPostHog(t)
			long := strings.Repeat("y", 250)
			resp := postRaw(t, srv.URL, route, "application/json",
				[]byte(`{"api_key":"phc_k","event":"long","distinct_id":"`+long+`"}`))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status %d, want 200", resp.StatusCode)
			}
			resp.Body.Close()
			r := tc.Get("/admin/events?event=long")
			r.AssertStatus(200)
			evts := r.JSONMap()["events"].([]any)
			if len(evts) != 1 {
				t.Fatalf("%d events, want 1", len(evts))
			}
			if got := evts[0].(map[string]any)["distinct_id"].(string); len(got) != 200 {
				t.Errorf("distinct_id length %d, want 200", len(got))
			}
		})

		t.Run(route+" valid event", func(t *testing.T) {
			srv, _ := setupPostHog(t)
			resp := postRaw(t, srv.URL, route, "application/json",
				[]byte(`{"api_key":"phc_k","event":"pv","distinct_id":"u1"}`))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status %d, want 200", resp.StatusCode)
			}
			if got := bodyOf(t, resp); got != `{"status":"Ok"}` {
				t.Errorf("body %q", got)
			}
		})
	}
}

// assertNoEvents fails the test if the twin holds any captured event.
func assertNoEvents(t *testing.T, tc *testutil.TwinClient) {
	t.Helper()
	r := tc.Get("/admin/events")
	r.AssertStatus(200)
	if n := len(r.JSONMap()["events"].([]any)); n != 0 {
		t.Errorf("%d events stored from a rejected request, want 0", n)
	}
}
