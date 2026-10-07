package api_test

import (
	"net/url"
	"testing"
)

// chat.unfurl checks which message it unfurls and what it is given, with the
// errors the docs list.
func TestChatUnfurlErrors(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "unfurls")
	ts := form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {"read <https://example.com/a>"}})["ts"].(string)
	good := `{"https://example.com/a":{"text":"A"}}`

	wantErrors(t, srv, "chat.unfurl", map[string]errCase{
		"no channel":        {url.Values{"ts": {ts}, "unfurls": {good}}, "missing_channel"},
		"no ts":             {url.Values{"channel": {ch}, "unfurls": {good}}, "missing_ts"},
		"no unfurls":        {url.Values{"channel": {ch}, "ts": {ts}}, "missing_unfurls"},
		"unknown channel":   {url.Values{"channel": {"CNOPE"}, "ts": {ts}, "unfurls": {good}}, "cannot_find_channel"},
		"unknown message":   {url.Values{"channel": {ch}, "ts": {"1.000001"}, "unfurls": {good}}, "cannot_find_message"},
		"unfurls not JSON":  {url.Values{"channel": {ch}, "ts": {ts}, "unfurls": {"nope"}}, "invalid_unfurls_format"},
		"unfurl not object": {url.Values{"channel": {ch}, "ts": {ts}, "unfurls": {`{"https://example.com/a":"A"}`}}, "invalid_unfurls_format"},
		"link not in text":  {url.Values{"channel": {ch}, "ts": {ts}, "unfurls": {`{"https://other.example":{"text":"B"}}`}}, "cannot_unfurl_message"},
		"id without source": {url.Values{"unfurl_id": {"U1"}, "unfurls": {good}}, "missing_source"},
		"source without id": {url.Values{"source": {"composer"}, "unfurls": {good}}, "missing_unfurl_id"},
		"bad source":        {url.Values{"unfurl_id": {"U1"}, "source": {"elsewhere"}, "unfurls": {good}}, "invalid_source"},
		"unknown unfurl id": {url.Values{"unfurl_id": {"U1"}, "source": {"composer"}, "unfurls": {good}}, "invalid_unfurl_id"},
	})
	mustOK(t, 200, form(t, srv, "chat.unfurl", url.Values{"channel": {ch}, "ts": {ts}, "unfurls": {good}}))
}

// bots.info answers for a bot by its ID: the default bot, and a seeded one.
func TestBotsInfo(t *testing.T) {
	srv, _ := setupSlack(t)
	b := form(t, srv, "bots.info", url.Values{"bot": {"B_BOT"}})["bot"].(map[string]any)
	if b["id"] != "B_BOT" || b["user_id"] != "U_BOT" || b["app_id"] != "A_SIM" || b["name"] == "" {
		t.Errorf("default bot: %v", b)
	}
	status, m := call(t, srv, "POST", "/admin/state", jsonType, `{"bots":{"B_OTHER":{"id":"B_OTHER","name":"other","app_id":"A_OTHER","updated":1700000000}}}`, false)
	if status != 200 || m["status"] != "loaded" {
		t.Fatalf("seed bot: %v", m)
	}
	o := form(t, srv, "bots.info", url.Values{"bot": {"B_OTHER"}})["bot"].(map[string]any)
	if o["name"] != "other" || o["app_id"] != "A_OTHER" || o["updated"] != float64(1700000000) {
		t.Errorf("seeded bot: %v", o)
	}
	wantErrors(t, srv, "bots.info", map[string]errCase{
		"unknown bot": {url.Values{"bot": {"BNOPE"}}, "bot_not_found"},
		"no bot":      {url.Values{}, "bot_not_found"},
	})
}

// users.identity takes a user token and answers who that user is.
func TestUsersIdentity(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, `"U_ADA":{"id":"U_ADA","name":"ada","real_name":"Ada Lovelace"}`)
	seedTokens(t, srv, `"xoxp-ada":{"token":"xoxp-ada","type":"user","user_id":"U_ADA"}`)

	if m := form(t, srv, "users.identity", nil); m["error"] != "not_allowed_token_type" {
		t.Errorf("a bot token: %v", m)
	}
	id := formAs(t, srv, "xoxp-ada", "users.identity", nil)
	mustOK(t, 200, id)
	if u := id["user"].(map[string]any); u["id"] != "U_ADA" || u["name"] != "Ada Lovelace" {
		t.Errorf("identity of a seeded user: %v", u)
	}
	if u := formAs(t, srv, "xoxp-anyone", "users.identity", nil)["user"].(map[string]any); u["id"] != "U_USER" {
		t.Errorf("identity of an unseeded user token: %v", u)
	}
}

// emoji.list lists the workspace's custom emoji: none until seeded, then each
// name with its URL or alias.
func TestEmojiList(t *testing.T) {
	srv, _ := setupSlack(t)
	if e := form(t, srv, "emoji.list", nil)["emoji"].(map[string]any); len(e) != 0 {
		t.Errorf("a new workspace has custom emoji: %v", e)
	}
	status, m := call(t, srv, "POST", "/admin/state", jsonType,
		`{"emoji":{"squirrel":"https://emoji.example/squirrel.png","shipit":"alias:squirrel"}}`, false)
	if status != 200 || m["status"] != "loaded" {
		t.Fatalf("seed emoji: %v", m)
	}
	e := form(t, srv, "emoji.list", nil)["emoji"].(map[string]any)
	if len(e) != 2 || e["squirrel"] != "https://emoji.example/squirrel.png" || e["shipit"] != "alias:squirrel" {
		t.Errorf("seeded emoji: %v", e)
	}
}

// team.info answers the workspace object, and refuses another team or a
// domain lookup, which needs an Enterprise organization.
func TestTeamInfo(t *testing.T) {
	srv, _ := setupSlack(t)
	tm := form(t, srv, "team.info", nil)["team"].(map[string]any)
	icon, _ := tm["icon"].(map[string]any)
	if tm["id"] != "T0001" || tm["name"] == "" || tm["domain"] == "" || icon["image_default"] != true {
		t.Errorf("team.info: %v", tm)
	}
	if _, ok := tm["email_domain"]; !ok {
		t.Errorf("team.info has no email_domain: %v", tm)
	}
	mustOK(t, 200, form(t, srv, "team.info", url.Values{"team": {"T0001"}}))
	wantErrors(t, srv, "team.info", map[string]errCase{
		"other team": {url.Values{"team": {"T9999"}}, "team_not_found"},
		"by domain":  {url.Values{"domain": {"example.com"}}, "team_not_on_enterprise"},
	})
}
