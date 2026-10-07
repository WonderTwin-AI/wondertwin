package api_test

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

// The users an unseeded token speaks for are workspace users: the caller can
// look itself up, and so can anyone else.
func TestDefaultPrincipalsAreUsers(t *testing.T) {
	srv, _ := setupSlack(t)
	for _, c := range []struct{ token, user string }{
		{"xoxb-test-token", "U_BOT"},
		{"xoxp-someone", "U_USER"},
	} {
		info := formAs(t, srv, c.token, "users.info", url.Values{"user": {c.user}})
		mustOK(t, 200, info)
		u := info["user"].(map[string]any)
		if u["id"] != c.user || u["team_id"] != "T0001" {
			t.Errorf("users.info %s: %v", c.user, u)
		}
		mustOK(t, 200, formAs(t, srv, c.token, "users.profile.get", nil))
	}
	if u := form(t, srv, "users.info", url.Values{"user": {"U_BOT"}})["user"].(map[string]any); u["is_bot"] != true {
		t.Errorf("the bot user is not a bot: %v", u)
	}
}

// users.lookupByEmail with no email finds nobody, even though users without
// an email exist.
func TestLookupByEmptyEmailFindsNobody(t *testing.T) {
	srv, _ := setupSlack(t)
	if m := form(t, srv, "users.lookupByEmail", url.Values{"email": {""}}); m["error"] != "users_not_found" {
		t.Errorf("empty email: %v", m)
	}
}

// users.profile.set takes a user token, sets the documented fields of profile
// or the one field name names, and keeps first, last and real name in step.
func TestProfileSetFields(t *testing.T) {
	srv, _ := setupSlack(t)
	const me = "xoxp-profile-set"

	if m := form(t, srv, "users.profile.set", url.Values{"name": {"title"}, "value": {"x"}}); m["error"] != "not_allowed_token_type" {
		t.Errorf("a bot token: %v", m)
	}
	if m := formAs(t, srv, me, "users.profile.set", url.Values{"profile": {"not json"}}); m["error"] != "invalid_profile" {
		t.Errorf("a profile that is not JSON: %v", m)
	}

	set := formAs(t, srv, me, "users.profile.set", url.Values{"profile": {
		`{"first_name":"Ada","last_name":"Lovelace","title":"Analyst","phone":"555","pronouns":"she/her","status_text":"busy","fields":{"Xf01":{"value":"Engines","alt":""}}}`}})
	mustOK(t, 200, set)
	p := set["profile"].(map[string]any)
	if p["real_name"] != "Ada Lovelace" || p["first_name"] != "Ada" || p["last_name"] != "Lovelace" ||
		p["title"] != "Analyst" || p["phone"] != "555" || p["pronouns"] != "she/her" || p["status_text"] != "busy" || p["skype"] != "" {
		t.Errorf("profile after set: %v", p)
	}
	if f := p["fields"].(map[string]any)["Xf01"].(map[string]any); f["value"] != "Engines" {
		t.Errorf("custom field: %v", f)
	}

	mustOK(t, 200, formAs(t, srv, me, "users.profile.set", url.Values{"name": {"real_name"}, "value": {"Grace"}}))
	got := formAs(t, srv, me, "users.profile.get", nil)["profile"].(map[string]any)
	if got["real_name"] != "Grace" || got["first_name"] != "Grace" || got["last_name"] != nil {
		t.Errorf("a single real_name clears last_name: %v", got)
	}
	if u := form(t, srv, "users.info", url.Values{"user": {"U_USER"}})["user"].(map[string]any); u["real_name"] != "Grace" {
		t.Errorf("the user's real_name does not follow the profile: %v", u["real_name"])
	}

	mustOK(t, 200, formAs(t, srv, me, "users.profile.set", url.Values{"name": {"Xf02"}, "value": {"Ops"}}))
	got = formAs(t, srv, me, "users.profile.get", nil)["profile"].(map[string]any)
	if f, _ := got["fields"].(map[string]any)["Xf02"].(map[string]any); f["value"] != "Ops" {
		t.Errorf("a custom field set by name: %v", got["fields"])
	}

	long := make([]rune, 101)
	for i := range long {
		long[i] = 'a'
	}
	if m := formAs(t, srv, me, "users.profile.set", url.Values{"name": {"status_text"}, "value": {string(long)}}); m["error"] != "too_long" {
		t.Errorf("a 101-character status: %v", m)
	}
	if m := formAs(t, srv, me, "users.profile.set", url.Values{"name": {"email"}, "value": {"me@example.com"}}); m["error"] != "not_admin" {
		t.Errorf("setting one's own email: %v", m)
	}
}

// Only an admin or owner sets another user's profile, and only an owner an
// admin's. An admin may set another user's email, if no one else has it.
func TestProfileSetForAnotherUser(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, `"U_ADMIN":{"id":"U_ADMIN","name":"admin","is_admin":true},
		"U_OWNER":{"id":"U_OWNER","name":"owner","is_owner":true},
		"U_PLAIN":{"id":"U_PLAIN","name":"plain"},
		"U_TAKEN":{"id":"U_TAKEN","name":"taken","profile":{"email":"taken@example.com"}}`)
	seedTokens(t, srv, `"xoxp-admin":{"token":"xoxp-admin","type":"user","user_id":"U_ADMIN"},
		"xoxp-owner":{"token":"xoxp-owner","type":"user","user_id":"U_OWNER"},
		"xoxp-plain":{"token":"xoxp-plain","type":"user","user_id":"U_PLAIN"}`)

	title := func(user string) url.Values {
		return url.Values{"user": {user}, "name": {"title"}, "value": {"Set by someone else"}}
	}
	if m := formAs(t, srv, "xoxp-plain", "users.profile.set", title("U_TAKEN")); m["error"] != "not_admin" {
		t.Errorf("a member setting another's profile: %v", m)
	}
	mustOK(t, 200, formAs(t, srv, "xoxp-admin", "users.profile.set", title("U_PLAIN")))
	if p := form(t, srv, "users.profile.get", url.Values{"user": {"U_PLAIN"}})["profile"].(map[string]any); p["title"] != "Set by someone else" {
		t.Errorf("the admin's change is not on the target: %v", p)
	}
	if m := formAs(t, srv, "xoxp-admin", "users.profile.set", title("U_OWNER")); m["error"] != "cannot_update_admin_user" {
		t.Errorf("an admin setting an owner's profile: %v", m)
	}
	mustOK(t, 200, formAs(t, srv, "xoxp-owner", "users.profile.set", title("U_ADMIN")))

	email := func(addr string) url.Values {
		return url.Values{"user": {"U_PLAIN"}, "name": {"email"}, "value": {addr}}
	}
	if m := formAs(t, srv, "xoxp-admin", "users.profile.set", email("taken@example.com")); m["error"] != "email_taken" {
		t.Errorf("an email in use: %v", m)
	}
	mustOK(t, 200, formAs(t, srv, "xoxp-admin", "users.profile.set", email("plain@example.com")))
}

func seedTokens(t *testing.T, srv *httptest.Server, tokens string) {
	t.Helper()
	status, m := call(t, srv, "POST", "/admin/state", jsonType, `{"tokens":{`+tokens+`}}`, false)
	if status != 200 || m["status"] != "loaded" {
		t.Fatalf("seed tokens: %d %v", status, m)
	}
}
