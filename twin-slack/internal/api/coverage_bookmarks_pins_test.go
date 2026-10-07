package api_test

import (
	"net/url"
	"testing"
)

// bookmarks.edit changes only the fields given; bookmarks.remove takes the
// bookmark off the channel's list.
func TestBookmarksEditAndRemove(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "bmedit")
	added := form(t, srv, "bookmarks.add", url.Values{"channel_id": {ch}, "title": {"Docs"}, "type": {"link"}, "link": {"https://example.com"}})
	mustOK(t, 200, added)
	id := added["bookmark"].(map[string]any)["id"].(string)

	edited := form(t, srv, "bookmarks.edit", url.Values{"channel_id": {ch}, "bookmark_id": {id}, "title": {"Guide"}})
	mustOK(t, 200, edited)
	bm := edited["bookmark"].(map[string]any)
	if bm["id"] != id || bm["title"] != "Guide" || bm["link"] != "https://example.com" {
		t.Errorf("edit answer: %v", bm)
	}
	wantErrors(t, srv, "bookmarks.edit", map[string]errCase{
		"unknown bookmark": {url.Values{"channel_id": {ch}, "bookmark_id": {"BM-nope"}, "title": {"x"}}, "bookmark_not_found"},
	})

	mustOK(t, 200, form(t, srv, "bookmarks.remove", url.Values{"channel_id": {ch}, "bookmark_id": {id}}))
	list := form(t, srv, "bookmarks.list", url.Values{"channel_id": {ch}})
	mustOK(t, 200, list)
	if bms, ok := list["bookmarks"].([]any); !ok || len(bms) != 0 {
		t.Errorf("a removed bookmark is still listed: %v", list["bookmarks"])
	}
}

// pins.remove unpins a pinned message once; a second remove is no_pin.
func TestPinsRemoveErrors(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, ts := postIn(t, srv, "unpin")
	mustOK(t, 200, form(t, srv, "pins.add", url.Values{"channel": {ch}, "timestamp": {ts}}))
	mustOK(t, 200, form(t, srv, "pins.remove", url.Values{"channel": {ch}, "timestamp": {ts}}))
	list := form(t, srv, "pins.list", url.Values{"channel": {ch}})
	if items, ok := list["items"].([]any); !ok || len(items) != 0 {
		t.Errorf("an unpinned message is still listed: %v", list["items"])
	}
	wantErrors(t, srv, "pins.remove", map[string]errCase{
		"removed twice": {url.Values{"channel": {ch}, "timestamp": {ts}}, "no_pin"},
		"never pinned":  {url.Values{"channel": {ch}, "timestamp": {"1.2"}}, "no_pin"},
	})
}
