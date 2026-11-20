package api_test

import (
	"net/http"
	"strings"
	"testing"
)

// TestBatchValidation holds /batch/'s rejections (D11): an empty batch, an
// event with no name and an event with no distinct_id each answer 400 in the
// capture service's text body, and a rejected batch stores nothing. A
// distinct_id over 200 characters is cut to 200 and the event is kept
// (ph-ee-002, a hypothesis).
func TestBatchValidation(t *testing.T) {
	post := func(srv string, body string) *http.Response {
		return postRaw(t, srv, "/batch/", "application/json", []byte(body))
	}

	t.Run("empty batch", func(t *testing.T) {
		srv, _ := setupPostHog(t)
		resp := post(srv.URL, `{"api_key":"phc_k","batch":[]}`)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status %d, want 400", resp.StatusCode)
		}
		if got := bodyOf(t, resp); got != "request holds no event" {
			t.Errorf("body %q, want %q", got, "request holds no event")
		}
	})

	t.Run("missing distinct_id rejects the whole batch", func(t *testing.T) {
		srv, tc := setupPostHog(t)
		resp := post(srv.URL, `{"api_key":"phc_k","batch":[{"event":"ok","distinct_id":"u1"},{"event":"bad"}]}`)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status %d, want 400", resp.StatusCode)
		}
		if got := bodyOf(t, resp); got != "event submitted without a distinct_id" {
			t.Errorf("body %q", got)
		}
		r := tc.Get("/admin/events")
		r.AssertStatus(200)
		if n := len(r.JSONMap()["events"].([]any)); n != 0 {
			t.Errorf("%d events stored from a rejected batch, want 0", n)
		}
	})

	t.Run("missing event name", func(t *testing.T) {
		srv, _ := setupPostHog(t)
		resp := post(srv.URL, `{"api_key":"phc_k","batch":[{"distinct_id":"u1"}]}`)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status %d, want 400", resp.StatusCode)
		}
		if got := bodyOf(t, resp); !strings.Contains(got, "missing event name attribute") {
			t.Errorf("body %q", got)
		}
	})

	t.Run("long distinct_id is cut to 200", func(t *testing.T) {
		srv, tc := setupPostHog(t)
		long := strings.Repeat("x", 250)
		resp := post(srv.URL, `{"api_key":"phc_k","batch":[{"event":"long","distinct_id":"`+long+`"}]}`)
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

	t.Run("top-level $set on identify reaches the person", func(t *testing.T) {
		srv, tc := setupPostHog(t)
		resp := post(srv.URL, `{"api_key":"phc_k","batch":[{"event":"$identify","distinct_id":"u9","$set":{"email":"u9@example.com"}}]}`)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d, want 200", resp.StatusCode)
		}
		resp.Body.Close()
		r := tc.Get("/admin/persons?distinct_id=u9")
		r.AssertStatus(200)
		persons := r.JSONMap()["persons"].([]any)
		if len(persons) != 1 || persons[0].(map[string]any)["properties"].(map[string]any)["email"] != "u9@example.com" {
			t.Errorf("person u9 lacks email: %v", persons)
		}
	})

	t.Run("distinct_id inside properties is read", func(t *testing.T) {
		srv, tc := setupPostHog(t)
		resp := post(srv.URL, `{"api_key":"phc_k","batch":[{"event":"$create_alias","properties":{"distinct_id":"u8","alias":"u8-alias"}}]}`)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d, want 200 (posthog-go sends alias with the id only in properties)", resp.StatusCode)
		}
		resp.Body.Close()
		r := tc.Get("/admin/aliases")
		r.AssertStatus(200)
		al := r.JSONMap()["aliases"].([]any)
		if len(al) != 1 || al[0].(map[string]any)["distinct_id"] != "u8" {
			t.Errorf("alias should carry distinct_id u8: %v", al)
		}
	})
}
