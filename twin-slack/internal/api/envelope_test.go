package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// callHeaders is call, but it also returns the response headers.
func callHeaders(t *testing.T, srv *httptest.Server, method, path, contentType, body string, authed bool) (int, http.Header, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if authed {
		req.Header.Set("Authorization", "Bearer xoxb-test-token")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s %s: answer is not JSON: %q", method, path, raw)
	}
	return resp.StatusCode, resp.Header, out
}

func wantUnknownMethod(t *testing.T, status int, h http.Header, m map[string]any, name string) {
	t.Helper()
	wantError(t, status, m, "unknown_method")
	if m["req_method"] != name {
		t.Errorf("req_method = %v, want %q", m["req_method"], name)
	}
	if got := h.Get("X-Slack-Failure"); got != "unknown_method" {
		t.Errorf("x-slack-failure = %q, want unknown_method", got)
	}
}

// Slack answers a method it retired (channels.list, retired 2021-02-24) and a
// name that never existed in the same way, before it looks at a token.
func TestUnknownMethodAnswersBeforeAuthentication(t *testing.T) {
	srv, _ := setupSlack(t)
	for _, name := range []string{"channels.list", "definitely.notAMethod", "nothing"} {
		for _, verb := range []string{"GET", "POST"} {
			t.Run(verb+" "+name, func(t *testing.T) {
				status, h, m := callHeaders(t, srv, verb, "/api/"+name, "", "", false)
				wantUnknownMethod(t, status, h, m, name)
			})
		}
	}
}

// The app emulator does not serve methods Slack sunset (files.upload,
// 2025-11-12) or degraded (reminders.* and stars.*, July 2023).
func TestRetiredAndDegradedMethodsAreNotServed(t *testing.T) {
	srv, _ := setupSlack(t)
	for _, name := range []string{
		"files.upload",
		"reminders.add", "reminders.complete", "reminders.delete", "reminders.info", "reminders.list",
		"stars.add", "stars.list", "stars.remove",
	} {
		t.Run(name, func(t *testing.T) {
			status, h, m := callHeaders(t, srv, "POST", "/api/"+name, "", "", true)
			wantUnknownMethod(t, status, h, m, name)
		})
	}
}

func TestUnknownMethodIgnoresTokenAndBody(t *testing.T) {
	srv, _ := setupSlack(t)

	status, h, m := callHeaders(t, srv, "POST", "/api/channels.list", jsonType, `{"broken":`, true)
	wantUnknownMethod(t, status, h, m, "channels.list")

	status, h, m = callHeaders(t, srv, "POST", "/api/definitely.notAMethod", "application/xml", "<a/>", false)
	wantUnknownMethod(t, status, h, m, "definitely.notAMethod")

	status, h, m = callHeaders(t, srv, "GET", "/api/channels.list?limit=abc", "", "", true)
	wantUnknownMethod(t, status, h, m, "channels.list")
}

func TestRealMethodsAreNotUnknown(t *testing.T) {
	srv, _ := setupSlack(t)
	// A real method without a token is not_authed, and has no req_method.
	status, h, m := callHeaders(t, srv, "POST", "/api/conversations.list", "", "", false)
	wantError(t, status, m, "not_authed")
	if h.Get("X-Slack-Failure") != "not_authed" {
		t.Errorf("x-slack-failure = %q, want not_authed", h.Get("X-Slack-Failure"))
	}
	if _, ok := m["req_method"]; ok {
		t.Errorf("a known method must not carry req_method: %v", m)
	}
}

func TestEveryErrorRepeatsItsCodeInTheFailureHeader(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")

	cases := []struct {
		name, method, path, contentType, body string
		authed                                bool
		want                                  string
	}{
		{"missing token", "POST", "/api/auth.test", "", "", false, "not_authed"},
		{"handler error", "POST", "/api/chat.postMessage", formType, "channel=C-nope&text=x", true, "channel_not_found"},
		{"decoder error", "POST", "/api/chat.postMessage", jsonType, `{"channel":`, true, "invalid_json"},
		{"post type error", "POST", "/api/chat.postMessage", "application/xml", "<a/>", true, "invalid_post_type"},
		{"missing argument", "POST", "/api/chat.postMessage", formType, "channel=" + ch, true, "no_text"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, h, m := callHeaders(t, srv, c.method, c.path, c.contentType, c.body, c.authed)
			wantError(t, status, m, c.want)
			if got := h.Get("X-Slack-Failure"); got != c.want {
				t.Errorf("x-slack-failure = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSuccessCarriesNoFailureHeader(t *testing.T) {
	srv, _ := setupSlack(t)
	status, h, m := callHeaders(t, srv, "POST", "/api/auth.test", "", "", true)
	mustOK(t, status, m)
	if got := h.Get("X-Slack-Failure"); got != "" {
		t.Errorf("a successful call must not set x-slack-failure, got %q", got)
	}
}

func TestAnswersAreJSONWithACharset(t *testing.T) {
	srv, _ := setupSlack(t)
	for _, c := range []struct {
		name, path string
		authed     bool
	}{
		{"success", "/api/auth.test", true},
		{"api error", "/api/auth.test", false},
		{"unknown method", "/api/channels.list", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, h, _ := callHeaders(t, srv, "POST", c.path, "", "", c.authed)
			if got := h.Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q", got)
			}
		})
	}
}

// Slack signals lifecycle only in the JSON body. It sends no Deprecation,
// Sunset, Link or Warning header, not even on a retired method.
func TestNoLifecycleHeaders(t *testing.T) {
	srv, _ := setupSlack(t)
	for _, c := range []struct {
		path   string
		authed bool
	}{
		{"/api/auth.test", true},
		{"/api/auth.test", false},
		{"/api/channels.list", false},
		{"/api/channels.list", true},
	} {
		_, h, _ := callHeaders(t, srv, "POST", c.path, "", "", c.authed)
		for _, name := range []string{"Deprecation", "Sunset", "Link", "Warning"} {
			if v := h.Get(name); v != "" {
				t.Errorf("%s: unexpected %s header %q", c.path, name, v)
			}
		}
	}
}
