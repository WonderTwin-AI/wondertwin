package api_test

import (
	"net/url"
	"testing"
)

// reactions.list lists a message once for each of the user's reactions on it,
// with its channel, pages with a cursor, and leaves out deleted messages.
func TestReactionsListPagesAndSkipsDeleted(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, first := postIn(t, srv, "reacted")
	second := form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {"second"}})["ts"].(string)
	gone := form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {"gone"}})["ts"].(string)
	for _, rx := range []struct{ ts, name string }{{first, "eyes"}, {first, "tada"}, {second, "eyes"}, {gone, "eyes"}} {
		mustOK(t, 200, form(t, srv, "reactions.add", url.Values{"channel": {ch}, "timestamp": {rx.ts}, "name": {rx.name}}))
	}
	mustOK(t, 200, form(t, srv, "chat.delete", url.Values{"channel": {ch}, "ts": {gone}}))

	all := form(t, srv, "reactions.list", nil)
	mustOK(t, 200, all)
	items := all["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("items: %d, want 3 (one per reaction, none for the deleted message): %v", len(items), items)
	}
	var ts []string
	for _, it := range items {
		item := it.(map[string]any)
		if item["type"] != "message" || item["channel"] != ch {
			t.Errorf("item: %v", item)
		}
		ts = append(ts, item["message"].(map[string]any)["ts"].(string))
	}
	if ts[0] != first || ts[1] != first || ts[2] != second {
		t.Errorf("items' messages: %v", ts)
	}

	page := form(t, srv, "reactions.list", url.Values{"limit": {"2"}})
	next := page["response_metadata"].(map[string]any)["next_cursor"].(string)
	if len(page["items"].([]any)) != 2 || next == "" {
		t.Fatalf("first page: %v", page)
	}
	rest := form(t, srv, "reactions.list", url.Values{"limit": {"2"}, "cursor": {next}})
	if n := len(rest["items"].([]any)); n != 1 || rest["response_metadata"].(map[string]any)["next_cursor"] != "" {
		t.Errorf("last page: %v", rest)
	}
	wantErrors(t, srv, "reactions.list", map[string]errCase{
		"unknown user": {url.Values{"user": {"U-nope"}}, "user_not_found"},
		"bad cursor":   {url.Values{"cursor": {"nope"}}, "invalid_cursor"},
	})
}
