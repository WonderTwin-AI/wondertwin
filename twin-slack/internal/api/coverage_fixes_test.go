package api_test

import (
	"net/url"
	"strconv"
	"testing"
	"time"
)

// Methods that act on a channel answer channel_not_found for an unknown or
// missing one, as Slack's docs list, and store nothing.
func TestChannelMethodsRefuseAnUnknownChannel(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "real")
	for _, c := range []struct {
		method string
		args   url.Values
	}{
		{"chat.postEphemeral", url.Values{"user": {"U_USER"}, "text": {"x"}}},
		{"chat.meMessage", url.Values{"text": {"x"}}},
		{"chat.scheduleMessage", url.Values{"text": {"x"}, "post_at": {strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)}}},
		{"conversations.close", url.Values{}},
		{"conversations.mark", url.Values{"ts": {"1.000001"}}},
	} {
		for name, channel := range map[string]string{"unknown": "CNOPE", "missing": ""} {
			args := url.Values{}
			for k, v := range c.args {
				args[k] = v
			}
			if channel != "" {
				args.Set("channel", channel)
			}
			if m := form(t, srv, c.method, args); m["ok"] != false || m["error"] != "channel_not_found" {
				t.Errorf("%s with a %s channel: %v", c.method, name, m)
			}
		}
	}
	if n := len(form(t, srv, "conversations.history", url.Values{"channel": {ch}})["messages"].([]any)); n != 1 {
		t.Errorf("a refused call stored a message: %d in history", n)
	}
	if m := form(t, srv, "chat.scheduledMessages.list", url.Values{}); m["ok"] == true {
		if list, _ := m["scheduled_messages"].([]any); len(list) != 0 {
			t.Errorf("a refused schedule was kept: %v", list)
		}
	}
}

// chat.meMessage posts a me_message to a real channel and refuses no text.
func TestMeMessage(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "me")
	if m := form(t, srv, "chat.meMessage", url.Values{"channel": {ch}}); m["error"] != "no_text" {
		t.Errorf("no text: %v", m)
	}
	mustOK(t, 200, form(t, srv, "chat.meMessage", url.Values{"channel": {ch}, "text": {"waves"}}))
	latest := form(t, srv, "conversations.history", url.Values{"channel": {ch}})["messages"].([]any)[0].(map[string]any)
	if latest["subtype"] != "me_message" || latest["text"] != "waves" {
		t.Errorf("me_message: %v", latest)
	}
}

// conversations.rename refuses an empty name with invalid_name_required.
func TestRenameRequiresAName(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "named")
	if m := form(t, srv, "conversations.rename", url.Values{"channel": {ch}, "name": {""}}); m["error"] != "invalid_name_required" {
		t.Errorf("empty name: %v", m)
	}
	info := form(t, srv, "conversations.info", url.Values{"channel": {ch}})["channel"].(map[string]any)
	if info["name"] != "named" {
		t.Errorf("a refused rename changed the name: %v", info["name"])
	}
}

// usergroups.users.list answers [] for a group with no users, never null.
func TestEmptyUsergroupListsNoUsers(t *testing.T) {
	srv, _ := setupSlack(t)
	ug := form(t, srv, "usergroups.create", url.Values{"name": {"Empty"}})["usergroup"].(map[string]any)["id"].(string)
	m := form(t, srv, "usergroups.users.list", url.Values{"usergroup": {ug}})
	if users, ok := m["users"].([]any); !ok || len(users) != 0 {
		t.Errorf("users for an empty group: %#v", m["users"])
	}
}

// users.setPresence keeps the caller's presence, even for the default bot,
// which has no profile record; users.getPresence defaults to the caller and
// refuses a presence other than auto or away.
func TestPresenceIsTheCallers(t *testing.T) {
	srv, _ := setupSlack(t)
	if m := form(t, srv, "users.setPresence", url.Values{"presence": {"busy"}}); m["error"] != "invalid_presence" {
		t.Errorf("presence busy: %v", m)
	}
	mustOK(t, 200, form(t, srv, "users.setPresence", url.Values{"presence": {"away"}}))
	if p := form(t, srv, "users.getPresence", url.Values{})["presence"]; p != "away" {
		t.Errorf("the bot's own presence after away: %v", p)
	}
	if p := form(t, srv, "users.getPresence", url.Values{"user": {"U_BOT"}})["presence"]; p != "away" {
		t.Errorf("the bot's presence by id: %v", p)
	}
	if p := formAs(t, srv, "xoxp-someone", "users.getPresence", url.Values{})["presence"]; p != "active" {
		t.Errorf("another caller's own presence: %v", p)
	}
	mustOK(t, 200, form(t, srv, "users.setPresence", url.Values{"presence": {"auto"}}))
	if p := form(t, srv, "users.getPresence", url.Values{})["presence"]; p != "active" {
		t.Errorf("presence after auto: %v", p)
	}
}
