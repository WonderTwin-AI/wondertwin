package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// rawRequest sends body as given, so a test can send malformed JSON.
func rawRequest(t *testing.T, srvURL, method, path, body string, withKey bool) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, srvURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if withKey {
		req.Header.Set("Authorization", resendHeaders["Authorization"])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("%s %s: content type %q, want JSON", method, path, ct)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("%s %s: body is not JSON: %v", method, path, err)
	}
	return resp.StatusCode, got
}

func assertError(t *testing.T, status int, body map[string]any, wantStatus int, wantName, wantMessage string) {
	t.Helper()
	if status != wantStatus {
		t.Errorf("status %d, want %d", status, wantStatus)
	}
	if body["statusCode"] != float64(wantStatus) {
		t.Errorf("statusCode %v, want %d (must equal the HTTP status)", body["statusCode"], wantStatus)
	}
	if body["name"] != wantName {
		t.Errorf("name %v, want %s", body["name"], wantName)
	}
	if body["message"] != wantMessage {
		t.Errorf("message %q, want %q", body["message"], wantMessage)
	}
}

func TestErrorEnvelope(t *testing.T) {
	srv, _ := setupResend(t)
	for _, path := range []string{"/emails", "/emails/batch"} {
		t.Run("malformed JSON on "+path, func(t *testing.T) {
			status, body := rawRequest(t, srv.URL, "POST", path, "{not json", true)
			assertError(t, status, body, 400, "validation_error", "Request body must be valid JSON.")
		})
	}
	t.Run("wrong JSON shape keeps 422 with a matching statusCode", func(t *testing.T) {
		status, body := rawRequest(t, srv.URL, "POST", "/emails", `{"subject": 7}`, true)
		if status != 422 || body["statusCode"] != float64(422) {
			t.Errorf("got %d / %v, want 422 / 422", status, body["statusCode"])
		}
	})
}

func TestUnknownRoute(t *testing.T) {
	srv, _ := setupResend(t)
	t.Run("unknown endpoint", func(t *testing.T) {
		status, body := rawRequest(t, srv.URL, "GET", "/no-such-endpoint", "", true)
		assertError(t, status, body, 404, "not_found", "The requested endpoint does not exist.")
	})
	t.Run("unknown path under a known resource", func(t *testing.T) {
		status, body := rawRequest(t, srv.URL, "GET", "/emails/a/b", "", true)
		assertError(t, status, body, 404, "not_found", "The requested endpoint does not exist.")
	})
	t.Run("wrong method", func(t *testing.T) {
		status, body := rawRequest(t, srv.URL, "DELETE", "/emails", "", true)
		assertError(t, status, body, 405, "method_not_allowed", "Method DELETE is not allowed for the requested path.")
	})
}

func TestAuthBeforeRouting(t *testing.T) {
	srv, _ := setupResend(t)
	cases := []struct{ name, method, path, body string }{
		{"known route", "POST", "/emails", "{}"},
		{"unknown endpoint", "GET", "/no-such-endpoint", ""},
		{"wrong method", "DELETE", "/emails", ""},
		{"malformed body", "POST", "/emails", "{not json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, body := rawRequest(t, srv.URL, c.method, c.path, c.body, false)
			assertError(t, status, body, 401, "missing_api_key", "Missing API Key")
		})
	}
	t.Run("admin paths need no key", func(t *testing.T) {
		status, body := rawRequest(t, srv.URL, "GET", "/admin/no-such-endpoint", "", false)
		assertError(t, status, body, 404, "not_found", "The requested endpoint does not exist.")
	})
}
