package api_test

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

// walk follows next_cursor from the first page to the last and returns every
// page's values for field, reading arguments as the query string of a GET.
func walk(t *testing.T, srv *httptest.Server, method string, args url.Values, field string) (pages [][]string, last map[string]any) {
	t.Helper()
	cursor := ""
	for i := 0; i < 50; i++ {
		q := url.Values{}
		for k, v := range args {
			q[k] = v
		}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		status, m := call(t, srv, "GET", "/api/"+method+"?"+q.Encode(), "", "", true)
		mustOK(t, status, m)
		pages = append(pages, pick(t, m[field], field))
		meta, ok := m["response_metadata"].(map[string]any)
		if !ok {
			t.Fatalf("%s: a paginated answer always carries response_metadata, got %v", method, m)
		}
		next, _ := meta["next_cursor"].(string)
		if next == "" {
			return pages, m
		}
		cursor = next
	}
	t.Fatalf("%s: pagination does not terminate", method)
	return nil, nil
}

// pick reduces a page to comparable strings: ids, or a message's text.
func pick(t *testing.T, v any, field string) []string {
	t.Helper()
	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("%s should be a JSON array, got %#v", field, v)
	}
	out := []string{}
	for _, e := range arr {
		switch x := e.(type) {
		case string:
			out = append(out, x)
		case map[string]any:
			if txt, ok := x["text"].(string); ok && field == "messages" {
				out = append(out, txt)
			} else {
				out = append(out, x["id"].(string))
			}
		}
	}
	return out
}

func sizes(pages [][]string) []int {
	out := make([]int, len(pages))
	for i, p := range pages {
		out[i] = len(p)
	}
	return out
}

func flatten(pages [][]string) []string {
	var out []string
	for _, p := range pages {
		out = append(out, p...)
	}
	return out
}

func seedMessages(t *testing.T, tc *testutil.TwinClient, ch string, n int) []string {
	t.Helper()
	texts := make([]string, n)
	for i := range texts {
		texts[i] = fmt.Sprintf("m%d", i+1)
		slackPost(tc, "/api/chat.postMessage", map[string]any{"channel": ch, "text": texts[i]}).AssertStatus(200)
	}
	return texts
}

func TestConversationsListPages(t *testing.T) {
	srv, tc := setupSlack(t)
	var want []string
	for i := 0; i < 5; i++ {
		want = append(want, seedChannel(tc, fmt.Sprintf("chan-%d", i)))
	}

	pages, last := walk(t, srv, "conversations.list", url.Values{"limit": {"2"}}, "channels")
	if !slices.Equal(sizes(pages), []int{2, 2, 1}) {
		t.Errorf("page sizes = %v", sizes(pages))
	}
	if got := flatten(pages); !slices.Equal(got, want) {
		t.Errorf("pages should add up to every channel once, in id order: got %v want %v", got, want)
	}
	if meta := last["response_metadata"].(map[string]any); meta["next_cursor"] != "" {
		t.Errorf("the last page ends with an empty next_cursor, got %v", meta["next_cursor"])
	}

	pages, _ = walk(t, srv, "conversations.list", nil, "channels")
	if len(pages) != 1 || len(pages[0]) != 5 {
		t.Errorf("with no limit all five channels come back on one page, got %v", sizes(pages))
	}
}

func TestHistoryPagesNewestFirst(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")
	seedMessages(t, tc, ch, 5)

	pages, last := walk(t, srv, "conversations.history", url.Values{"channel": {ch}, "limit": {"2"}}, "messages")
	want := [][]string{{"m5", "m4"}, {"m3", "m2"}, {"m1"}}
	if fmt.Sprint(pages) != fmt.Sprint(want) {
		t.Errorf("pages = %v, want %v", pages, want)
	}
	if last["has_more"] != false {
		t.Errorf("has_more should be false on the last page, got %v", last["has_more"])
	}

	status, first := call(t, srv, "GET", "/api/conversations.history?channel="+ch+"&limit=2", "", "", true)
	mustOK(t, status, first)
	if first["has_more"] != true {
		t.Errorf("has_more should be true while a cursor is returned, got %v", first["has_more"])
	}
}

func TestHistoryCursorIsStableWhileMessagesChange(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")
	seedMessages(t, tc, ch, 5)

	status, page1 := call(t, srv, "GET", "/api/conversations.history?channel="+ch+"&limit=2", "", "", true)
	mustOK(t, status, page1)
	cursor := page1["response_metadata"].(map[string]any)["next_cursor"].(string)
	msgs := page1["messages"].([]any)
	lastOfPage1 := msgs[len(msgs)-1].(map[string]any)

	// A new message arrives, and the last message of the first page is deleted.
	slackPost(tc, "/api/chat.postMessage", map[string]any{"channel": ch, "text": "m6"}).AssertStatus(200)
	slackPost(tc, "/api/chat.delete", map[string]any{"channel": ch, "ts": lastOfPage1["ts"]}).AssertStatus(200)

	status, page2 := call(t, srv, "GET", "/api/conversations.history?channel="+ch+"&limit=2&cursor="+url.QueryEscape(cursor), "", "", true)
	mustOK(t, status, page2)
	if got := pick(t, page2["messages"], "messages"); !slices.Equal(got, []string{"m3", "m2"}) {
		t.Errorf("second page = %v, want [m3 m2]: the new message must not appear and nothing may repeat", got)
	}
}

func TestRepliesPageOldestFirst(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")
	parent := slackPost(tc, "/api/chat.postMessage", map[string]any{"channel": ch, "text": "parent"}).JSONMap()
	for i := 1; i <= 4; i++ {
		slackPost(tc, "/api/chat.postMessage", map[string]any{"channel": ch, "text": fmt.Sprintf("r%d", i), "thread_ts": parent["ts"]}).AssertStatus(200)
	}

	pages, _ := walk(t, srv, "conversations.replies", url.Values{"channel": {ch}, "ts": {parent["ts"].(string)}, "limit": {"2"}}, "messages")
	want := [][]string{{"parent", "r1"}, {"r2", "r3"}, {"r4"}}
	if fmt.Sprint(pages) != fmt.Sprint(want) {
		t.Errorf("pages = %v, want %v", pages, want)
	}
}

func seedMembers(t *testing.T, tc *testutil.TwinClient, n int) string {
	t.Helper()
	members := make([]string, n)
	for i := range members {
		members[i] = fmt.Sprintf("U%02d", i+1)
	}
	testutil.NewAdminClient(tc).LoadState(map[string]any{
		"channels": map[string]any{
			"C_BIG": map[string]any{"id": "C_BIG", "name": "big", "is_channel": true, "members": members},
		},
	}).AssertStatus(200)
	return "C_BIG"
}

func TestMembersPage(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedMembers(t, tc, 5)
	pages, _ := walk(t, srv, "conversations.members", url.Values{"channel": {ch}, "limit": {"2"}}, "members")
	if !slices.Equal(sizes(pages), []int{2, 2, 1}) || !slices.Equal(flatten(pages), []string{"U01", "U02", "U03", "U04", "U05"}) {
		t.Errorf("pages = %v", pages)
	}
}

func TestUsersListPages(t *testing.T) {
	srv, tc := setupSlack(t)
	users := map[string]any{}
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("U%02d", i)
		users[id] = map[string]any{"id": id, "team_id": "T0001", "name": "user" + strconv.Itoa(i)}
	}
	testutil.NewAdminClient(tc).LoadState(map[string]any{"users": users}).AssertStatus(200)

	// No limit returns everyone, as Slack documents for users.list.
	pages, _ := walk(t, srv, "users.list", nil, "members")
	if len(pages) != 1 || len(pages[0]) != 5 {
		t.Errorf("no limit should return all five users on one page, got %v", sizes(pages))
	}
	pages, _ = walk(t, srv, "users.list", url.Values{"limit": {"2"}}, "members")
	if !slices.Equal(sizes(pages), []int{2, 2, 1}) {
		t.Errorf("page sizes = %v", sizes(pages))
	}
}

func TestUsersConversationsPages(t *testing.T) {
	srv, tc := setupSlack(t)
	channels := map[string]any{}
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("C%02d", i)
		channels[id] = map[string]any{"id": id, "name": "c" + strconv.Itoa(i), "is_channel": true, "members": []string{"U_BOT"}}
	}
	channels["C99"] = map[string]any{"id": "C99", "name": "elsewhere", "is_channel": true, "members": []string{"U_OTHER"}}
	testutil.NewAdminClient(tc).LoadState(map[string]any{"channels": channels}).AssertStatus(200)

	pages, _ := walk(t, srv, "users.conversations", url.Values{"limit": {"2"}}, "channels")
	if !slices.Equal(sizes(pages), []int{2, 2, 1}) || slices.Contains(flatten(pages), "C99") {
		t.Errorf("pages = %v: only the caller's channels, five of them", pages)
	}
}

func TestInvalidCursorIsRejected(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")
	seedMessages(t, tc, ch, 3)

	// A cursor for users, used on channels, does not compute.
	status, users := call(t, srv, "GET", "/api/users.list?limit=1", "", "", true)
	mustOK(t, status, users)
	testutil.NewAdminClient(tc).LoadState(map[string]any{"users": map[string]any{
		"U01": map[string]any{"id": "U01", "name": "a"}, "U02": map[string]any{"id": "U02", "name": "b"},
	}}).AssertStatus(200)
	_, usersPage := call(t, srv, "GET", "/api/users.list?limit=1", "", "", true)
	userCursor := usersPage["response_metadata"].(map[string]any)["next_cursor"].(string)

	for _, c := range []struct{ name, path string }{
		{"gibberish on channels", "/api/conversations.list?cursor=gibberish"},
		{"gibberish on history", "/api/conversations.history?channel=" + ch + "&cursor=gibberish"},
		{"gibberish on users", "/api/users.list?cursor=gibberish"},
		{"another method's cursor", "/api/conversations.list?cursor=" + url.QueryEscape(userCursor)},
	} {
		t.Run(c.name, func(t *testing.T) {
			status, h, m := callHeaders(t, srv, "GET", c.path, "", "", true)
			wantError(t, status, m, "invalid_cursor")
			if h.Get("X-Slack-Failure") != "invalid_cursor" {
				t.Errorf("x-slack-failure = %q", h.Get("X-Slack-Failure"))
			}
		})
	}
}

// Slack adjusts a limit it cannot use and never rejects it.
func TestUnusableLimitIsAdjusted(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")
	seedMessages(t, tc, ch, 3)
	for _, limit := range []string{"abc", "0", "-3", "100000", ""} {
		t.Run("limit="+limit, func(t *testing.T) {
			status, m := call(t, srv, "GET", "/api/conversations.history?channel="+ch+"&limit="+limit, "", "", true)
			mustOK(t, status, m)
			if got := len(m["messages"].([]any)); got != 3 {
				t.Errorf("an adjusted limit should return all 3 messages, got %d", got)
			}
		})
	}
}

// The cursor and limit mean the same in every encoding Slack accepts.
func TestCursorWorksInEveryEncoding(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")
	seedMessages(t, tc, ch, 5)

	_, p1 := call(t, srv, "GET", "/api/conversations.history?channel="+ch+"&limit=2", "", "", true)
	cursor := p1["response_metadata"].(map[string]any)["next_cursor"].(string)
	if !strings.HasSuffix(cursor, "=") {
		t.Errorf("cursors usually end with =, got %q", cursor)
	}

	form := url.Values{"channel": {ch}, "limit": {"2"}, "cursor": {cursor}}.Encode()
	body := `{"channel":"` + ch + `","limit":2,"cursor":"` + cursor + `"}`
	for name, r := range map[string]func() (int, map[string]any){
		"query": func() (int, map[string]any) {
			return call(t, srv, "GET", "/api/conversations.history?"+form, "", "", true)
		},
		"form": func() (int, map[string]any) {
			return call(t, srv, "POST", "/api/conversations.history", formType, form, true)
		},
		"JSON": func() (int, map[string]any) {
			return call(t, srv, "POST", "/api/conversations.history", jsonType, body, true)
		},
	} {
		t.Run(name, func(t *testing.T) {
			status, m := r()
			mustOK(t, status, m)
			if got := pick(t, m["messages"], "messages"); !slices.Equal(got, []string{"m3", "m2"}) {
				t.Errorf("second page = %v, want [m3 m2]", got)
			}
		})
	}
}
