package api_test

import (
	"regexp"
	"testing"
)

func TestFrontHeaders(t *testing.T) {
	_, tc := setupResend(t)
	etag := regexp.MustCompile(`^W/"[0-9a-f]+-[A-Za-z0-9+/]{27}"$`)

	for _, path := range []string{"/domains", "/no-such-route"} {
		resp := resendGet(tc, path)
		if ct := resp.Headers.Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("GET %s content-type %q", path, ct)
		}
		if resp.Headers.Get("X-Frame-Options") != "DENY" || resp.Headers.Get("Strict-Transport-Security") == "" {
			t.Errorf("GET %s: security headers missing: %v", path, resp.Headers)
		}
		if !etag.MatchString(resp.Headers.Get("Etag")) {
			t.Errorf("GET %s etag %q", path, resp.Headers.Get("Etag"))
		}
	}

	send := resendPost(tc, "/emails", map[string]any{
		"from": "a@acme.dev", "to": "delivered@resend.dev", "subject": "s", "html": "<p>h</p>",
	}).AssertStatus(200)
	if ct := send.Headers.Get("Content-Type"); ct != "application/json" {
		t.Errorf("POST /emails content-type %q, want application/json", ct)
	}
	if send.Headers.Get("X-Frame-Options") != "" || send.Headers.Get("Etag") != "" {
		t.Errorf("POST /emails must carry no security headers or etag: %v", send.Headers)
	}
}
