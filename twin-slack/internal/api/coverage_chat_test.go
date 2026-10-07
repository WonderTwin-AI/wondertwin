package api_test

import (
	"net/url"
	"strings"
	"testing"
)

// chat.postEphemeral answers a message_ts and stores nothing in history.
func TestChatPostEphemeral(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "ephemeral")

	m := form(t, srv, "chat.postEphemeral", url.Values{"channel": {ch}, "user": {"U_BOT"}, "text": {"only you"}})
	mustOK(t, 200, m)
	if ts, _ := m["message_ts"].(string); ts == "" {
		t.Errorf("no message_ts: %v", m)
	}
	for _, text := range historyTexts(t, srv, ch) {
		if text == "only you" {
			t.Error("an ephemeral message is in the channel history")
		}
	}

	wantErrors(t, srv, "chat.postEphemeral", map[string]errCase{
		"no channel": {url.Values{"user": {"U_BOT"}, "text": {"x"}}, "channel_not_found"},
		"no user":    {url.Values{"channel": {ch}, "text": {"x"}}, "user_not_found"},
	})
}

// chat.getPermalink answers a permalink for a message in a known channel.
func TestChatGetPermalink(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, ts := postIn(t, srv, "links")

	m := form(t, srv, "chat.getPermalink", url.Values{"channel": {ch}, "message_ts": {ts}})
	mustOK(t, 200, m)
	link, _ := m["permalink"].(string)
	if !strings.HasPrefix(link, "https://") || !strings.Contains(link, ch) || m["channel"] != ch {
		t.Errorf("permalink answer: %v", m)
	}

	wantErrors(t, srv, "chat.getPermalink", map[string]errCase{
		"no channel":      {url.Values{"message_ts": {ts}}, "channel_not_found"},
		"unknown channel": {url.Values{"channel": {"CNOPE"}, "message_ts": {ts}}, "channel_not_found"},
	})
}

// chat.meMessage posts a me_message into the channel history.
func TestChatMeMessage(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "me")

	m := form(t, srv, "chat.meMessage", url.Values{"channel": {ch}, "text": {"waves"}})
	mustOK(t, 200, m)
	if m["channel"] != ch || m["ts"] == "" || m["ts"] == nil {
		t.Errorf("meMessage answer: %v", m)
	}
	found := false
	for _, msg := range historyMessages(t, srv, ch) {
		if msg["text"] == "waves" {
			found = true
			if msg["subtype"] != "me_message" {
				t.Errorf("subtype %v, want me_message", msg["subtype"])
			}
		}
	}
	if !found {
		t.Error("the me_message is not in the channel history")
	}
}

// chat.unfurl acknowledges an unfurl of a link in the message.
func TestChatUnfurl(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "unfurl")
	ts := form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {"see <https://example.com>"}})["ts"].(string)
	m := form(t, srv, "chat.unfurl", url.Values{"channel": {ch}, "ts": {ts}, "unfurls": {`{"https://example.com":{"text":"x"}}`}})
	mustOK(t, 200, m)
}

// Scheduling: an unknown scheduled message id is refused, a deleted message
// leaves the list, and an empty list is [] rather than null.
func TestChatScheduledMessages(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "later")

	empty := form(t, srv, "chat.scheduledMessages.list", nil)
	mustOK(t, 200, empty)
	if list, ok := empty["scheduled_messages"].([]any); !ok || len(list) != 0 {
		t.Errorf("empty list: %v", empty["scheduled_messages"])
	}

	s := form(t, srv, "chat.scheduleMessage", url.Values{"channel": {ch}, "text": {"soon"}, "post_at": {"9999999999"}})
	mustOK(t, 200, s)
	id, _ := s["scheduled_message_id"].(string)
	if id == "" || s["channel"] != ch || s["post_at"] != float64(9999999999) {
		t.Errorf("scheduleMessage answer: %v", s)
	}

	wantErrors(t, srv, "chat.scheduleMessage", map[string]errCase{
		"no channel": {url.Values{"text": {"x"}, "post_at": {"9999999999"}}, "channel_not_found"},
	})
	wantErrors(t, srv, "chat.deleteScheduledMessage", map[string]errCase{
		"unknown id": {url.Values{"channel": {ch}, "scheduled_message_id": {"Q-nope"}}, "invalid_scheduled_message_id"},
		"no id":      {url.Values{"channel": {ch}}, "invalid_scheduled_message_id"},
	})

	mustOK(t, 200, form(t, srv, "chat.deleteScheduledMessage", url.Values{"channel": {ch}, "scheduled_message_id": {id}}))
	after := form(t, srv, "chat.scheduledMessages.list", nil)
	if list, _ := after["scheduled_messages"].([]any); len(list) != 0 {
		t.Errorf("a deleted scheduled message is still listed: %v", list)
	}
	wantErrors(t, srv, "chat.deleteScheduledMessage", map[string]errCase{
		"deleted twice": {url.Values{"channel": {ch}, "scheduled_message_id": {id}}, "invalid_scheduled_message_id"},
	})
}
