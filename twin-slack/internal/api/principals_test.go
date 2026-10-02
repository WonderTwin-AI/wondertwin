package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

// callAs is call with an explicit Authorization header (empty sends none).
func callAs(t *testing.T, srv *httptest.Server, method, path, contentType, body, authorization string) (int, http.Header, map[string]any) {
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
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
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

func authTestAs(t *testing.T, srv *httptest.Server, token string) map[string]any {
	t.Helper()
	status, _, m := callAs(t, srv, "POST", "/api/auth.test", "", "", "Bearer "+token)
	mustOK(t, status, m)
	return m
}

func TestTokenSources(t *testing.T) {
	srv, _ := setupSlack(t)
	const tok = "xoxb-from-somewhere"

	var multi bytes.Buffer
	mw := multipart.NewWriter(&multi)
	_ = mw.WriteField("token", tok)
	_ = mw.Close()

	cases := []struct {
		name, path, contentType, body, authorization, want string
	}{
		{"Authorization header", "/api/auth.test", "", "", "Bearer " + tok, ""},
		{"form body", "/api/auth.test", formType, url.Values{"token": {tok}}.Encode(), "", ""},
		{"multipart body", "/api/auth.test", mw.FormDataContentType(), multi.String(), "", ""},
		{"text/plain body", "/api/auth.test", "text/plain", url.Values{"token": {tok}}.Encode(), "", ""},
		{"query string only", "/api/auth.test?token=" + tok, "", "", "", "not_authed"},
		{"query string beside an empty body", "/api/auth.test?token=" + tok, formType, "a=b", "", "not_authed"},
		{"JSON body only", "/api/auth.test", jsonType, `{"token":"` + tok + `"}`, "", "not_authed"},
		{"no token at all", "/api/auth.test", "", "", "", "not_authed"},
		{"header wins over a bad body token", "/api/auth.test", formType, "token=nonsense", "Bearer " + tok, ""},
		{"a bad header is not rescued by a good body token", "/api/auth.test", formType, url.Values{"token": {tok}}.Encode(), "Bearer nonsense", "invalid_auth"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			verb := "POST"
			status, h, m := callAs(t, srv, verb, c.path, c.contentType, c.body, c.authorization)
			if c.want == "" {
				mustOK(t, status, m)
				return
			}
			wantError(t, status, m, c.want)
			if got := h.Get("X-Slack-Failure"); got != c.want {
				t.Errorf("x-slack-failure = %q, want %q", got, c.want)
			}
		})
	}
}

func TestTokenShapes(t *testing.T) {
	srv, _ := setupSlack(t)
	for _, tok := range []string{"xoxb-abc", "xoxp-abc", "xapp-1-abc"} {
		status, _, m := callAs(t, srv, "POST", "/api/auth.test", "", "", "Bearer "+tok)
		mustOK(t, status, m)
	}
	for _, c := range []struct{ name, authorization string }{
		{"no known prefix", "Bearer abc"},
		{"prefix with nothing after it", "Bearer xoxb-"},
		{"unknown prefix", "Bearer xoxz-abc"},
		{"not the Bearer scheme", "Basic xoxb-abc"},
		{"Bearer with no token", "Bearer "},
		{"lower-case scheme", "bearer xoxb-abc"},
	} {
		t.Run(c.name, func(t *testing.T) {
			status, h, m := callAs(t, srv, "POST", "/api/auth.test", "", "", c.authorization)
			wantError(t, status, m, "invalid_auth")
			if h.Get("X-Slack-Failure") != "invalid_auth" {
				t.Errorf("x-slack-failure = %q", h.Get("X-Slack-Failure"))
			}
		})
	}
}

func TestAuthTestReflectsTheToken(t *testing.T) {
	srv, _ := setupSlack(t)

	bot := authTestAs(t, srv, "xoxb-one")
	if bot["user"] != "bot" || bot["user_id"] != "U_BOT" || bot["bot_id"] != "B_BOT" {
		t.Errorf("bot token: %v", bot)
	}
	if bot["team_id"] != "T0001" || bot["team"] != "WonderTwin" || bot["url"] != "https://wondertwin.slack.com/" {
		t.Errorf("bot token workspace fields: %v", bot)
	}

	user := authTestAs(t, srv, "xoxp-two")
	if user["user_id"] != "U_USER" || user["user"] != "user" {
		t.Errorf("user token: %v", user)
	}
	if _, has := user["bot_id"]; has {
		t.Errorf("a user token has no bot_id: %v", user)
	}

	app := authTestAs(t, srv, "xapp-1-three")
	if _, has := app["user_id"]; has {
		t.Errorf("an app-level token speaks for no user: %v", app)
	}
	if app["team_id"] != "T0001" {
		t.Errorf("app token workspace fields: %v", app)
	}
}

func TestSeededTokensSpeakForTheirPrincipal(t *testing.T) {
	srv, tc := setupSlack(t)
	testutil.NewAdminClient(tc).LoadState(map[string]any{
		"users": map[string]any{
			"U123": map[string]any{"id": "U123", "team_id": "T0001", "name": "grace", "real_name": "Grace Hopper"},
		},
		"tokens": map[string]any{
			// Not shaped like a Slack token: a seeded token is accepted as it is.
			"custom-credential": map[string]any{"token": "custom-credential", "type": "user", "user_id": "U123"},
			"xoxb-seeded-bot":   map[string]any{"token": "xoxb-seeded-bot", "type": "bot", "user_id": "U999", "bot_id": "B999", "team_id": "T0042"},
		},
	}).AssertStatus(200)

	user := authTestAs(t, srv, "custom-credential")
	if user["user"] != "grace" || user["user_id"] != "U123" {
		t.Errorf("seeded user token: %v", user)
	}
	bot := authTestAs(t, srv, "xoxb-seeded-bot")
	if bot["user_id"] != "U999" || bot["bot_id"] != "B999" || bot["team_id"] != "T0042" {
		t.Errorf("seeded bot token: %v", bot)
	}
	// A well-formed token that was not seeded still gets the default identity.
	if other := authTestAs(t, srv, "xoxb-unseeded"); other["user_id"] != "U_BOT" {
		t.Errorf("unseeded bot token: %v", other)
	}
}

func TestRevokedTokenStopsWorking(t *testing.T) {
	srv, _ := setupSlack(t)
	const tok, other = "xoxb-to-revoke", "xoxb-bystander"

	status, _, m := callAs(t, srv, "POST", "/api/auth.revoke", "", "", "Bearer "+tok)
	mustOK(t, status, m)
	if m["revoked"] != true {
		t.Fatalf("revoked = %v, want true", m["revoked"])
	}

	status, h, m := callAs(t, srv, "POST", "/api/auth.test", "", "", "Bearer "+tok)
	wantError(t, status, m, "token_revoked")
	if h.Get("X-Slack-Failure") != "token_revoked" {
		t.Errorf("x-slack-failure = %q", h.Get("X-Slack-Failure"))
	}
	// Revoking it again is refused the same way, and other tokens are untouched.
	status, _, m = callAs(t, srv, "POST", "/api/auth.revoke", "", "", "Bearer "+tok)
	wantError(t, status, m, "token_revoked")
	authTestAs(t, srv, other)
}

func TestRevokeInTestMode(t *testing.T) {
	srv, _ := setupSlack(t)
	const tok = "xoxb-test-mode"
	for _, c := range []struct{ name, path, contentType, body string }{
		{"query", "/api/auth.revoke?test=true", "", ""},
		{"form", "/api/auth.revoke", formType, "test=1"},
		{"JSON", "/api/auth.revoke", jsonType, `{"test":true}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			status, _, m := callAs(t, srv, "POST", c.path, c.contentType, c.body, "Bearer "+tok)
			mustOK(t, status, m)
			if m["revoked"] != false {
				t.Errorf("revoked = %v, want false in test mode", m["revoked"])
			}
			authTestAs(t, srv, tok)
		})
	}
}

func TestAPITestAnswersWithoutAToken(t *testing.T) {
	srv, _ := setupSlack(t)

	// Slack's live answer to a bare call, observed without credentials.
	status, _, m := callAs(t, srv, "POST", "/api/api.test", "", "", "")
	mustOK(t, status, m)
	if args, ok := m["args"].(map[string]any); !ok || len(args) != 0 {
		t.Errorf("args = %#v, want {}", m["args"])
	}

	status, _, m = callAs(t, srv, "GET", "/api/api.test?foo=bar", "", "", "")
	mustOK(t, status, m)
	if m["args"].(map[string]any)["foo"] != "bar" {
		t.Errorf("args = %v", m["args"])
	}

	status, _, m = callAs(t, srv, "POST", "/api/api.test", formType, "foo=baz", "")
	mustOK(t, status, m)
	if m["args"].(map[string]any)["foo"] != "baz" {
		t.Errorf("args = %v", m["args"])
	}

	status, _, m = callAs(t, srv, "POST", "/api/api.test", jsonType, `{"foo":"qux"}`, "")
	mustOK(t, status, m)
	if m["args"].(map[string]any)["foo"] != "qux" {
		t.Errorf("args = %v", m["args"])
	}
}

func TestAPITestReturnsTheRequestedError(t *testing.T) {
	srv, _ := setupSlack(t)
	status, h, m := callAs(t, srv, "POST", "/api/api.test", formType, "error=my_error", "")
	wantError(t, status, m, "my_error")
	if m["args"].(map[string]any)["error"] != "my_error" {
		t.Errorf("args = %v", m["args"])
	}
	if h.Get("X-Slack-Failure") != "my_error" {
		t.Errorf("x-slack-failure = %q", h.Get("X-Slack-Failure"))
	}
}

func TestAPITestNeverEchoesTheToken(t *testing.T) {
	srv, _ := setupSlack(t)
	status, _, m := callAs(t, srv, "POST", "/api/api.test", formType, "token=xoxb-secret&foo=bar", "")
	mustOK(t, status, m)
	args := m["args"].(map[string]any)
	if _, has := args["token"]; has {
		t.Errorf("api.test must not reflect a credential: %v", args)
	}
	if args["foo"] != "bar" {
		t.Errorf("args = %v", args)
	}
}

func TestAPITestReportsUndecodableRequests(t *testing.T) {
	srv, _ := setupSlack(t)
	status, _, m := callAs(t, srv, "POST", "/api/api.test", jsonType, `{"foo":`, "")
	wantError(t, status, m, "invalid_json")
}
