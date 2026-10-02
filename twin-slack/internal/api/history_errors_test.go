package api_test

import (
	"net/url"
	"testing"
)

func TestHistoryAndRepliesNeedAChannelThatExists(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, ts := postIn(t, srv, "hist")
	for name, tc := range map[string]struct {
		method string
		v      url.Values
		want   string
	}{
		"history, no channel":      {"conversations.history", url.Values{}, "channel_not_found"},
		"history, unknown channel": {"conversations.history", url.Values{"channel": {"CNOPE"}}, "channel_not_found"},
		"replies, no channel":      {"conversations.replies", url.Values{"ts": {ts}}, "channel_not_found"},
		"replies, unknown channel": {"conversations.replies", url.Values{"channel": {"CNOPE"}, "ts": {ts}}, "channel_not_found"},
		"replies, no ts":           {"conversations.replies", url.Values{"channel": {ch}}, "thread_not_found"},
		"replies, unknown ts":      {"conversations.replies", url.Values{"channel": {ch}, "ts": {"1.000000"}}, "thread_not_found"},
	} {
		if m := form(t, srv, tc.method, tc.v); m["ok"] != false || m["error"] != tc.want {
			t.Errorf("%s: want %s, got %v", name, tc.want, m)
		}
	}

	// A ts from another channel is not a thread in this one.
	other, _ := postIn(t, srv, "other")
	if m := form(t, srv, "conversations.replies", url.Values{"channel": {other}, "ts": {ts}}); m["error"] != "thread_not_found" {
		t.Errorf("a ts from another channel: %v", m)
	}

	// The thread's own parent answers, with the parent as its first message.
	m := form(t, srv, "conversations.replies", url.Values{"channel": {ch}, "ts": {ts}})
	mustOK(t, 200, m)
	if msgs := m["messages"].([]any); len(msgs) != 1 || msgs[0].(map[string]any)["ts"] != ts {
		t.Errorf("replies to a message with no replies is the message itself: %v", msgs)
	}
}
