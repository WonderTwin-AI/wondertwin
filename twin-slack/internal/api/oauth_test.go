package api_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// exchange calls oauth.v2.access with no token, the way an app does before it
// has one. basic sends the client credentials by HTTP Basic instead of in the
// body.
func exchange(t *testing.T, srv *httptest.Server, form url.Values, basic bool) map[string]any {
	t.Helper()
	if basic {
		id, secret := form.Get("client_id"), form.Get("client_secret")
		form.Del("client_id")
		form.Del("client_secret")
		_, _, m := callWithBasic(t, srv, form.Encode(), id, secret)
		return m
	}
	_, m := call(t, srv, "POST", "/api/oauth.v2.access", formType, form.Encode(), false)
	return m
}

func callWithBasic(t *testing.T, srv *httptest.Server, body, id, secret string) (int, http.Header, map[string]any) {
	t.Helper()
	basic := base64.StdEncoding.EncodeToString([]byte(id + ":" + secret))
	return callAs(t, srv, "POST", "/api/oauth.v2.access", formType, body, "Basic "+basic)
}

func TestOAuthV2AccessIssuesAWorkingBotToken(t *testing.T) {
	srv, _ := setupSlack(t)
	for _, basic := range []bool{false, true} {
		code := "code-body"
		if basic {
			code = "code-basic"
		}
		m := exchange(t, srv, url.Values{"client_id": {"123.456"}, "client_secret": {"s3cret"}, "code": {code}}, basic)
		if m["ok"] != true || m["token_type"] != "bot" {
			t.Fatalf("basic=%v: %v", basic, m)
		}
		tok, _ := m["access_token"].(string)
		if !strings.HasPrefix(tok, "xoxb-") || m["scope"] == "" {
			t.Errorf("basic=%v: access_token %q, scope %v", basic, tok, m["scope"])
		}
		team := m["team"].(map[string]any)["id"]

		status, _, who := callAs(t, srv, "POST", "/api/auth.test", "", "", "Bearer "+tok)
		mustOK(t, status, who)
		if who["team_id"] != team || who["bot_id"] == nil {
			t.Errorf("basic=%v: auth.test with the issued token: %v", basic, who)
		}
	}
}

func TestOAuthV2AccessTokensAreDistinct(t *testing.T) {
	srv, _ := setupSlack(t)
	creds := func(code string) url.Values {
		return url.Values{"client_id": {"1"}, "client_secret": {"s"}, "code": {code}}
	}
	a := exchange(t, srv, creds("one"), false)["access_token"]
	b := exchange(t, srv, creds("two"), false)["access_token"]
	if a == nil || a == b {
		t.Errorf("two installs got tokens %v and %v", a, b)
	}
}

func TestOAuthV2AccessErrors(t *testing.T) {
	srv, _ := setupSlack(t)
	first := exchange(t, srv, url.Values{"client_id": {"1"}, "client_secret": {"s"}, "code": {"used"}}, false)
	if first["ok"] != true {
		t.Fatalf("first exchange: %v", first)
	}
	for name, tc := range map[string]struct {
		form url.Values
		want string
	}{
		"no client_id":     {url.Values{"client_secret": {"s"}, "code": {"c1"}}, "invalid_client_id"},
		"no client_secret": {url.Values{"client_id": {"1"}, "code": {"c2"}}, "bad_client_secret"},
		"no code":          {url.Values{"client_id": {"1"}, "client_secret": {"s"}}, "invalid_code"},
		"reused code":      {url.Values{"client_id": {"1"}, "client_secret": {"s"}, "code": {"used"}}, "invalid_code"},
		"refresh grant":    {url.Values{"client_id": {"1"}, "client_secret": {"s"}, "grant_type": {"refresh_token"}, "refresh_token": {"r"}}, "invalid_refresh_token"},
		"unknown grant":    {url.Values{"client_id": {"1"}, "client_secret": {"s"}, "code": {"c3"}, "grant_type": {"password"}}, "invalid_grant_type"},
	} {
		if m := exchange(t, srv, tc.form, false); m["ok"] != false || m["error"] != tc.want {
			t.Errorf("%s: want %s, got %v", name, tc.want, m)
		}
	}
}

// A code exchanged before a state snapshot stays used after it is loaded.
func TestOAuthCodeStaysUsedAcrossASnapshot(t *testing.T) {
	srv, tc := setupSlack(t)
	exchange(t, srv, url.Values{"client_id": {"1"}, "client_secret": {"s"}, "code": {"once"}}, false)
	state := tc.Get("/admin/state")
	tc.Post("/admin/reset", nil)
	tc.Post("/admin/state", state.JSONMap())
	m := exchange(t, srv, url.Values{"client_id": {"1"}, "client_secret": {"s"}, "code": {"once"}}, false)
	if m["error"] != "invalid_code" {
		t.Errorf("a code used before the snapshot: %v", m)
	}
}

// After a state load, a new install never reuses a token the state holds.
func TestOAuthNewTokenAfterAStateLoadIsFresh(t *testing.T) {
	srv, tc := setupSlack(t)
	creds := func(code string) url.Values {
		return url.Values{"client_id": {"1"}, "client_secret": {"s"}, "code": {code}}
	}
	before := exchange(t, srv, creds("a"), false)["access_token"]
	state := tc.Get("/admin/state")
	tc.Post("/admin/reset", nil)
	tc.Post("/admin/state", state.JSONMap())
	after := exchange(t, srv, creds("b"), false)["access_token"]
	if after == nil || after == before {
		t.Errorf("token after a state load: %v, before: %v", after, before)
	}
	if m := exchange(t, srv, creds("a"), false); m["error"] != "invalid_code" {
		t.Errorf("the first install's code became usable again: %v", m)
	}
}
