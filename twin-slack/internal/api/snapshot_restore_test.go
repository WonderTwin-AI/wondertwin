package api_test

import (
	"net/url"
	"testing"
)

// A state snapshot keeps deleted messages deleted, and an emulator restored
// from it keeps issuing timestamps after the ones it holds.
func TestSnapshotRestoreKeepsDeletionsAndTimestamps(t *testing.T) {
	srv, tc := setupSlack(t)
	ch, kept := postIn(t, srv, "restore")
	gone := form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {"gone"}})["ts"].(string)
	mustOK(t, 200, form(t, srv, "chat.delete", url.Values{"channel": {ch}, "ts": {gone}}))

	state := tc.Get("/admin/state").JSONMap()
	tc.Post("/admin/reset", nil).AssertStatus(200)
	tc.Post("/admin/state", state).AssertStatus(200)

	msgs := form(t, srv, "conversations.history", url.Values{"channel": {ch}})["messages"].([]any)
	if len(msgs) != 1 || msgs[0].(map[string]any)["ts"] != kept {
		t.Errorf("history after restore: %v, want only %s", msgs, kept)
	}
	next := form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {"after"}})["ts"].(string)
	if next == kept || next == gone {
		t.Errorf("a timestamp issued after restore repeats one already used: %s", next)
	}
	if got := len(form(t, srv, "conversations.history", url.Values{"channel": {ch}})["messages"].([]any)); got != 2 {
		t.Errorf("messages after posting again: %d, want 2", got)
	}
}
