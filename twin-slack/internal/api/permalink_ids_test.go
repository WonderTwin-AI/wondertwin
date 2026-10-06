package api_test

import (
	"net/url"
	"strings"
	"testing"
)

// chat.getPermalink builds Slack's link: p and the ts without its dot, with
// thread_ts and cid for a reply, and message_not_found for an unknown ts.
func TestPermalinkFormat(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, parent := postIn(t, srv, "links")
	reply := form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {"r"}, "thread_ts": {parent}})["ts"].(string)

	top := form(t, srv, "chat.getPermalink", url.Values{"channel": {ch}, "message_ts": {parent}})["permalink"].(string)
	if want := "/archives/" + ch + "/p" + strings.ReplaceAll(parent, ".", ""); !strings.HasSuffix(top, want) {
		t.Errorf("permalink %q, want suffix %q", top, want)
	}
	threaded := form(t, srv, "chat.getPermalink", url.Values{"channel": {ch}, "message_ts": {reply}})["permalink"].(string)
	if want := "/p" + strings.ReplaceAll(reply, ".", "") + "?thread_ts=" + parent + "&cid=" + ch; !strings.HasSuffix(threaded, want) {
		t.Errorf("reply permalink %q, want suffix %q", threaded, want)
	}
	if m := form(t, srv, "chat.getPermalink", url.Values{"channel": {ch}, "message_ts": {"1.000001"}}); m["error"] != "message_not_found" {
		t.Errorf("unknown ts: %v", m)
	}
}

// Bookmark ids start Bk and user group ids start S, as Slack's do.
func TestBookmarkAndUsergroupIDPrefixes(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "ids")
	bm := form(t, srv, "bookmarks.add", url.Values{"channel_id": {ch}, "title": {"t"}, "type": {"link"}, "link": {"https://example.com"}})
	if id := bm["bookmark"].(map[string]any)["id"].(string); !strings.HasPrefix(id, "Bk") {
		t.Errorf("bookmark id %q", id)
	}
	ug := form(t, srv, "usergroups.create", url.Values{"name": {"Ops"}})
	if id := ug["usergroup"].(map[string]any)["id"].(string); !strings.HasPrefix(id, "S") {
		t.Errorf("usergroup id %q", id)
	}
}
