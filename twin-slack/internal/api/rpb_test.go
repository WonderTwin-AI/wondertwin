package api_test

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

// postIn creates a channel with one message and returns both.
func postIn(t *testing.T, srv *httptest.Server, name string) (string, string) {
	t.Helper()
	_, created := call(t, srv, "POST", "/api/conversations.create", formType, "name="+name, true)
	ch := created["channel"].(map[string]any)["id"].(string)
	_, posted := call(t, srv, "POST", "/api/chat.postMessage", formType, url.Values{"channel": {ch}, "text": {"x"}}.Encode(), true)
	return ch, posted["ts"].(string)
}

func form(t *testing.T, srv *httptest.Server, method string, v url.Values) map[string]any {
	t.Helper()
	_, m := call(t, srv, "POST", "/api/"+method, formType, v.Encode(), true)
	return m
}

func formAs(t *testing.T, srv *httptest.Server, token, method string, v url.Values) map[string]any {
	t.Helper()
	_, _, m := callAs(t, srv, "POST", "/api/"+method, formType, v.Encode(), "Bearer "+token)
	return m
}

func wantErrors(t *testing.T, srv *httptest.Server, method string, cases map[string]struct {
	v    url.Values
	want string
}) {
	t.Helper()
	for name, tc := range cases {
		if m := form(t, srv, method, tc.v); m["ok"] != false || m["error"] != tc.want {
			t.Errorf("%s %s: want %s, got %v", method, name, tc.want, m)
		}
	}
}

type errCase = struct {
	v    url.Values
	want string
}

func TestReactionsAddErrors(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, ts := postIn(t, srv, "rx")
	wantErrors(t, srv, "reactions.add", map[string]errCase{
		"no channel":      {url.Values{"timestamp": {ts}, "name": {"x"}}, "no_item_specified"},
		"no timestamp":    {url.Values{"channel": {ch}, "name": {"x"}}, "no_item_specified"},
		"no name":         {url.Values{"channel": {ch}, "timestamp": {ts}}, "invalid_name"},
		"unknown channel": {url.Values{"channel": {"CNOPE"}, "timestamp": {ts}, "name": {"x"}}, "channel_not_found"},
		"unknown ts":      {url.Values{"channel": {ch}, "timestamp": {"1.2"}, "name": {"x"}}, "message_not_found"},
	})
	wantErrors(t, srv, "reactions.get", map[string]errCase{
		"unknown channel": {url.Values{"channel": {"CNOPE"}, "timestamp": {ts}}, "channel_not_found"},
	})
}

// Each token reacts as its own user, so two users can add the same reaction.
func TestReactionsAreTheCallers(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, ts := postIn(t, srv, "who")
	add := url.Values{"channel": {ch}, "timestamp": {ts}, "name": {"tada"}}
	mustOK(t, 200, formAs(t, srv, "xoxb-one", "reactions.add", add))
	if m := formAs(t, srv, "xoxb-one", "reactions.add", add); m["error"] != "already_reacted" {
		t.Errorf("the same user twice: %v", m)
	}
	mustOK(t, 200, formAs(t, srv, "xoxp-two", "reactions.add", add))

	got := form(t, srv, "reactions.get", url.Values{"channel": {ch}, "timestamp": {ts}})
	rx := got["message"].(map[string]any)["reactions"].([]any)[0].(map[string]any)
	if rx["count"] != float64(2) || len(rx["users"].([]any)) != 2 {
		t.Errorf("reaction from a bot and a user: %v", rx)
	}

	mustOK(t, 200, formAs(t, srv, "xoxp-two", "reactions.remove", add))
	got = form(t, srv, "reactions.get", url.Values{"channel": {ch}, "timestamp": {ts}})
	rx = got["message"].(map[string]any)["reactions"].([]any)[0].(map[string]any)
	if users := rx["users"].([]any); len(users) != 1 || users[0] != "U_BOT" {
		t.Errorf("reactions.remove took the wrong user's reaction: %v", rx)
	}
}

func TestPinsAdd(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, ts := postIn(t, srv, "pins")
	mustOK(t, 200, formAs(t, srv, "xoxp-pinner", "pins.add", url.Values{"channel": {ch}, "timestamp": {ts}}))
	wantErrors(t, srv, "pins.add", map[string]errCase{
		"twice":           {url.Values{"channel": {ch}, "timestamp": {ts}}, "already_pinned"},
		"no channel":      {url.Values{"timestamp": {ts}}, "channel_not_found"},
		"no timestamp":    {url.Values{"channel": {ch}}, "no_item_specified"},
		"unknown channel": {url.Values{"channel": {"CNOPE"}, "timestamp": {ts}}, "channel_not_found"},
		"unknown ts":      {url.Values{"channel": {ch}, "timestamp": {"1.2"}}, "message_not_found"},
	})
	items := form(t, srv, "pins.list", url.Values{"channel": {ch}})["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("one pin, listed %d times: %v", len(items), items)
	}
	if by := items[0].(map[string]any)["created_by"]; by != "U_USER" {
		t.Errorf("created_by is the pinning user: %v", by)
	}
}

func TestBookmarksAdd(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "bm")
	link := func(v url.Values) url.Values {
		out := url.Values{"channel_id": {ch}, "title": {"Docs"}, "type": {"link"}, "link": {"https://example.com"}}
		for k, vs := range v {
			if vs[0] == "" {
				out.Del(k)
			} else {
				out[k] = vs
			}
		}
		return out
	}
	mustOK(t, 200, form(t, srv, "bookmarks.add", link(nil)))
	wantErrors(t, srv, "bookmarks.add", map[string]errCase{
		"no title":        {link(url.Values{"title": {""}}), "invalid_arguments"},
		"no type":         {link(url.Values{"type": {""}}), "invalid_arguments"},
		"file type":       {link(url.Values{"type": {"file"}}), "invalid_bookmark_type"},
		"no link":         {link(url.Values{"link": {""}}), "invalid_link"},
		"not a url":       {link(url.Values{"link": {"example.com"}}), "invalid_link"},
		"no channel":      {link(url.Values{"channel_id": {""}}), "channel_not_found"},
		"unknown channel": {link(url.Values{"channel_id": {"CNOPE"}}), "channel_not_found"},
	})
	bms := form(t, srv, "bookmarks.list", url.Values{"channel_id": {ch}})["bookmarks"].([]any)
	if len(bms) != 1 || bms[0].(map[string]any)["link"] != "https://example.com" {
		t.Errorf("bookmarks.list: %v", bms)
	}
}

func TestBookmarksList(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "empty")
	m := form(t, srv, "bookmarks.list", url.Values{"channel_id": {ch}})
	if bms, ok := m["bookmarks"].([]any); !ok || len(bms) != 0 {
		t.Errorf("a channel with no bookmarks answers []: %v", m)
	}
	if m := form(t, srv, "bookmarks.list", url.Values{"channel_id": {"CNOPE"}}); m["error"] != "channel_not_found" {
		t.Errorf("unknown channel: %v", m)
	}
}

// bookmarks.edit and bookmarks.remove answer the errors Slack's docs list:
// channel_not_found for an unknown channel, not_found for a bookmark that does
// not exist or belongs to another channel.
func TestBookmarkEditAndRemoveErrors(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "bm")
	other, _ := postIn(t, srv, "elsewhere")
	add := form(t, srv, "bookmarks.add", url.Values{"channel_id": {ch}, "title": {"Docs"}, "type": {"link"}, "link": {"https://example.com"}})
	mustOK(t, 200, add)
	id := add["bookmark"].(map[string]any)["id"].(string)

	for _, method := range []string{"bookmarks.edit", "bookmarks.remove"} {
		wantErrors(t, srv, method, map[string]errCase{
			"unknown channel":  {url.Values{"channel_id": {"CNOPE"}, "bookmark_id": {id}}, "channel_not_found"},
			"unknown bookmark": {url.Values{"channel_id": {ch}, "bookmark_id": {"BkNOPE"}}, "not_found"},
			"other channel":    {url.Values{"channel_id": {other}, "bookmark_id": {id}}, "not_found"},
		})
	}
	if m := form(t, srv, "bookmarks.edit", url.Values{"channel_id": {ch}, "bookmark_id": {id}, "link": {"example.com"}}); m["error"] != "invalid_link" {
		t.Errorf("edit to a link that is not a URL: %v", m)
	}

	edited := form(t, srv, "bookmarks.edit", url.Values{"channel_id": {ch}, "bookmark_id": {id}, "title": {"Guide"}})
	mustOK(t, 200, edited)
	if edited["bookmark"].(map[string]any)["title"] != "Guide" {
		t.Errorf("edit: %v", edited)
	}
	mustOK(t, 200, form(t, srv, "bookmarks.remove", url.Values{"channel_id": {ch}, "bookmark_id": {id}}))
	if m := form(t, srv, "bookmarks.remove", url.Values{"channel_id": {ch}, "bookmark_id": {id}}); m["error"] != "not_found" {
		t.Errorf("removing a removed bookmark: %v", m)
	}
	if bms := form(t, srv, "bookmarks.list", url.Values{"channel_id": {ch}})["bookmarks"].([]any); len(bms) != 0 {
		t.Errorf("after remove: %v", bms)
	}
}
