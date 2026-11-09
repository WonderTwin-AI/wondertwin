package api_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestRateLimitHeaders(t *testing.T) {
	srv, _ := setupResend(t)
	want := map[string]string{
		"Ratelimit-Limit":     "10",
		"Ratelimit-Policy":    "10;w=1",
		"Ratelimit-Remaining": "9",
		"Ratelimit-Reset":     "1",
	}
	do := func(method, path, body string, withKey bool) http.Header {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if withKey {
			req.Header.Set("Authorization", resendHeaders["Authorization"])
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.Header
	}

	authenticated := []struct{ name, method, path, body string }{
		{"send", "POST", "/emails", `{"from":"a@acme.dev","to":["delivered@resend.dev"],"subject":"s","html":"h"}`},
		{"validation error", "POST", "/emails", `{}`},
		{"unknown endpoint", "GET", "/no-such-endpoint", ""},
		{"list domains", "GET", "/domains", ""},
	}
	for _, c := range authenticated {
		t.Run(c.name+" carries the headers", func(t *testing.T) {
			h := do(c.method, c.path, c.body, true)
			for k, v := range want {
				if got := h.Get(k); got != v {
					t.Errorf("%s: %q, want %q", k, got, v)
				}
			}
		})
	}
	for _, c := range []struct{ name, method, path string }{
		{"missing key", "POST", "/emails"},
		{"missing key on unknown endpoint", "GET", "/no-such-endpoint"},
	} {
		t.Run(c.name+" carries none", func(t *testing.T) {
			h := do(c.method, c.path, "{}", false)
			for k := range want {
				if got := h.Get(k); got != "" {
					t.Errorf("%s: %q on an unauthenticated answer", k, got)
				}
			}
		})
	}
}
