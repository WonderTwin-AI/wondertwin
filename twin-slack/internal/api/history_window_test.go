package api_test

import (
	"net/url"
	"slices"
	"testing"
)

func historyTSs(t *testing.T, m map[string]any) []string {
	t.Helper()
	mustOK(t, 200, m)
	var out []string
	for _, v := range m["messages"].([]any) {
		out = append(out, v.(map[string]any)["ts"].(string))
	}
	return out
}

// conversations.history keeps messages between oldest and latest, leaving out
// a message on a bound unless inclusive is set.
func TestHistoryTimeWindow(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, first := postIn(t, srv, "window")
	var ts []string
	ts = append(ts, first)
	for _, text := range []string{"b", "c", "d"} {
		ts = append(ts, form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {text}})["ts"].(string))
	}
	newestFirst := func(s ...string) []string { slices.Reverse(s); return s }

	got := historyTSs(t, form(t, srv, "conversations.history", url.Values{"channel": {ch}, "oldest": {ts[1]}, "latest": {ts[3]}}))
	if !slices.Equal(got, []string{ts[2]}) {
		t.Errorf("exclusive window: %v, want %v", got, []string{ts[2]})
	}
	got = historyTSs(t, form(t, srv, "conversations.history", url.Values{"channel": {ch}, "oldest": {ts[1]}, "latest": {ts[3]}, "inclusive": {"true"}}))
	if want := newestFirst(ts[1], ts[2], ts[3]); !slices.Equal(got, want) {
		t.Errorf("inclusive window: %v, want %v", got, want)
	}
	got = historyTSs(t, form(t, srv, "conversations.history", url.Values{"channel": {ch}, "oldest": {"9999999999"}}))
	if len(got) != 0 {
		t.Errorf("oldest in the future: %v", got)
	}
	latest := form(t, srv, "conversations.history", url.Values{"channel": {ch}, "latest": {ts[1]}})
	if got := historyTSs(t, latest); !slices.Equal(got, []string{ts[0]}) || latest["latest"] != ts[1] {
		t.Errorf("latest only: %v, echoed latest %v", got, latest["latest"])
	}
	wantErrors(t, srv, "conversations.history", map[string]errCase{
		"bad oldest": {url.Values{"channel": {ch}, "oldest": {"yesterday"}}, "invalid_ts_oldest"},
		"bad latest": {url.Values{"channel": {ch}, "latest": {"1.2.3"}}, "invalid_ts_latest"},
	})
}

// conversations.replies takes the same window over a thread.
func TestRepliesTimeWindow(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, parent := postIn(t, srv, "thread-window")
	var replies []string
	for _, text := range []string{"r1", "r2"} {
		replies = append(replies, form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {text}, "thread_ts": {parent}})["ts"].(string))
	}
	got := historyTSs(t, form(t, srv, "conversations.replies", url.Values{"channel": {ch}, "ts": {parent}, "oldest": {parent}}))
	if !slices.Equal(got, replies) {
		t.Errorf("replies after the parent: %v, want %v", got, replies)
	}
	wantErrors(t, srv, "conversations.replies", map[string]errCase{
		"bad latest": {url.Values{"channel": {ch}, "ts": {parent}, "latest": {"x"}}, "invalid_ts_latest"},
	})
}
