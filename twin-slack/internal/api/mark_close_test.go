package api_test

import (
	"net/url"
	"testing"
)

// conversations.mark moves the caller's read cursor, which conversations.info
// reports as last_read; it refuses a ts that is not one, and a caller who is
// not a member (the conversations.mark docs).
func TestMarkMovesTheCallersReadCursor(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, ts := postIn(t, srv, "cursor")

	mustOK(t, 200, form(t, srv, "conversations.mark", url.Values{"channel": {ch}, "ts": {ts}}))
	if info := form(t, srv, "conversations.info", url.Values{"channel": {ch}})["channel"].(map[string]any); info["last_read"] != ts {
		t.Errorf("last_read after mark: %v", info["last_read"])
	}
	// Another user's cursor is their own.
	if info := formAs(t, srv, "xoxp-reader", "conversations.info", url.Values{"channel": {ch}})["channel"].(map[string]any); info["last_read"] != nil {
		t.Errorf("another caller's last_read: %v", info["last_read"])
	}

	wantErrors(t, srv, "conversations.mark", map[string]errCase{
		"no ts":    {url.Values{"channel": {ch}}, "invalid_timestamp"},
		"not a ts": {url.Values{"channel": {ch}, "ts": {"abc"}}, "invalid_timestamp"},
		"unknown":  {url.Values{"channel": {"CNOPE"}, "ts": {ts}}, "channel_not_found"},
	})
	if m := formAs(t, srv, "xoxp-outsider", "conversations.mark", url.Values{"channel": {ch}, "ts": {ts}}); m["error"] != "not_in_channel" {
		t.Errorf("mark by a non-member: %v", m)
	}
}

// conversations.close closes a direct or multi-person message for the caller,
// answers no_op and already_closed when it is closed already, and refuses a
// channel; conversations.open reopens it.
func TestCloseClosesADirectMessage(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, twoUsers)
	dm := open(t, srv, url.Values{"users": {"U1"}})["channel"].(map[string]any)["id"].(string)

	isOpen := func() any {
		return form(t, srv, "conversations.info", url.Values{"channel": {dm}})["channel"].(map[string]any)["is_open"]
	}
	if isOpen() != true {
		t.Errorf("a new DM is open: %v", isOpen())
	}
	if m := form(t, srv, "conversations.close", url.Values{"channel": {dm}}); m["ok"] != true || m["no_op"] != nil {
		t.Errorf("first close: %v", m)
	}
	if isOpen() != false {
		t.Errorf("is_open after close: %v", isOpen())
	}
	if m := form(t, srv, "conversations.close", url.Values{"channel": {dm}}); m["ok"] != true || m["no_op"] != true || m["already_closed"] != true {
		t.Errorf("closing a closed DM: %v", m)
	}
	open(t, srv, url.Values{"users": {"U1"}})
	if isOpen() != true {
		t.Errorf("is_open after reopening: %v", isOpen())
	}

	ch, _ := postIn(t, srv, "public")
	if m := form(t, srv, "conversations.close", url.Values{"channel": {ch}}); m["error"] != "method_not_supported_for_channel_type" {
		t.Errorf("closing a channel: %v", m)
	}
	if m := formAs(t, srv, "xoxp-stranger", "conversations.close", url.Values{"channel": {dm}}); m["error"] != "user_does_not_own_channel" {
		t.Errorf("closing someone else's DM: %v", m)
	}
}
