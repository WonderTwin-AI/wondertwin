package api_test

import (
	"net/url"
	"testing"
)

// pins.list shows a pinned message as it is now: an edit shows, and a
// deleted message is no longer listed.
func TestPinsListShowsTheCurrentMessage(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, ts := postIn(t, srv, "pinned")
	mustOK(t, 200, form(t, srv, "pins.add", url.Values{"channel": {ch}, "timestamp": {ts}}))
	mustOK(t, 200, form(t, srv, "chat.update", url.Values{"channel": {ch}, "ts": {ts}, "text": {"edited"}}))

	items := form(t, srv, "pins.list", url.Values{"channel": {ch}})["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["message"].(map[string]any)["text"] != "edited" {
		t.Errorf("pin after an edit: %v", items)
	}

	mustOK(t, 200, form(t, srv, "chat.delete", url.Values{"channel": {ch}, "ts": {ts}}))
	if items := form(t, srv, "pins.list", url.Values{"channel": {ch}})["items"].([]any); len(items) != 0 {
		t.Errorf("a deleted message is still pinned: %v", items)
	}
}

// pins.list requires a channel that exists.
func TestPinsListNeedsAChannel(t *testing.T) {
	srv, _ := setupSlack(t)
	wantErrors(t, srv, "pins.list", map[string]errCase{
		"no channel":      {url.Values{}, "channel_not_found"},
		"unknown channel": {url.Values{"channel": {"CNOPE"}}, "channel_not_found"},
	})
}
