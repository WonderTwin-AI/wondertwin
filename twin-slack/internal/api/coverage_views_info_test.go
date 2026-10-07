package api_test

import (
	"net/url"
	"testing"
)

// views.push stores a modal that views.update can then find, and refuses a
// missing view.
func TestViewsPush(t *testing.T) {
	srv, _ := setupSlack(t)
	status, m := call(t, srv, "POST", "/api/views.push", jsonType,
		`{"trigger_id":"1.2.abc","view":{"type":"modal","title":{"type":"plain_text","text":"Step 2"},"blocks":[]}}`, true)
	mustOK(t, status, m)
	pushed := viewOf(t, m)
	if pushed["type"] != "modal" || pushed["id"] == "" || pushed["id"] == nil {
		t.Fatalf("views.push answer: %v", pushed)
	}

	status, m = call(t, srv, "POST", "/api/views.update", jsonType,
		`{"view_id":"`+pushed["id"].(string)+`","view":{"type":"modal","callback_id":"step3","blocks":[]}}`, true)
	mustOK(t, status, m)
	if got := viewOf(t, m); got["id"] != pushed["id"] || got["callback_id"] != "step3" {
		t.Errorf("update of a pushed modal: %v", got)
	}

	status, m = call(t, srv, "POST", "/api/views.push", jsonType, `{"trigger_id":"1.2.abc"}`, true)
	wantError(t, status, m, "invalid_arguments")
}

// The read-only identity methods answer the workspace and its bot.
func TestWorkspaceInfoMethods(t *testing.T) {
	srv, _ := setupSlack(t)
	ident := authTestAs(t, srv, "xoxb-test-token")

	team := form(t, srv, "team.info", nil)
	mustOK(t, 200, team)
	if tm := team["team"].(map[string]any); tm["id"] != ident["team_id"] || tm["name"] == "" || tm["domain"] == "" {
		t.Errorf("team.info: %v (auth.test team %v)", tm, ident["team_id"])
	}

	bot := form(t, srv, "bots.info", url.Values{"bot": {ident["bot_id"].(string)}})
	mustOK(t, 200, bot)
	if b := bot["bot"].(map[string]any); b["id"] != ident["bot_id"] || b["user_id"] != ident["user_id"] || b["app_id"] == "" || b["deleted"] != false {
		t.Errorf("bots.info: %v (auth.test %v)", b, ident)
	}

	emoji := form(t, srv, "emoji.list", nil)
	mustOK(t, 200, emoji)
	if _, ok := emoji["emoji"].(map[string]any); !ok {
		t.Errorf("emoji.list: %v", emoji)
	}

	id := formAs(t, srv, "xoxp-identity", "users.identity", nil)
	mustOK(t, 200, id)
	if u := id["user"].(map[string]any); u["id"] == "" || id["team"].(map[string]any)["id"] != ident["team_id"] {
		t.Errorf("users.identity: %v", id)
	}
}
