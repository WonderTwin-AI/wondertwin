package api_test

import "testing"

// Slack returns an empty list as [], never null. The SDKs read null as a
// missing field and fail on it.
func TestEmptyMessageListsAreArrays(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")

	status, m := call(t, srv, "GET", "/api/conversations.history?channel="+ch, "", "", true)
	mustOK(t, status, m)
	if msgs, ok := m["messages"].([]any); !ok || len(msgs) != 0 {
		t.Fatalf("history of an empty channel should be [], got %#v", m["messages"])
	}

	posted := slackPost(tc, "/api/chat.postMessage", map[string]any{"channel": ch, "text": "gone soon"}).JSONMap()
	slackPost(tc, "/api/chat.delete", map[string]any{"channel": ch, "ts": posted["ts"]}).AssertStatus(200)

	status, m = call(t, srv, "GET", "/api/conversations.history?channel="+ch, "", "", true)
	mustOK(t, status, m)
	if msgs, ok := m["messages"].([]any); !ok || len(msgs) != 0 {
		t.Fatalf("history after the last message is deleted should be [], got %#v", m["messages"])
	}

}

// pins.list, reactions.list and the admin message listing return [] when there
// is nothing to list, never null.
func TestEmptyCollectionListsAreArrays(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")

	for _, c := range []struct{ name, path, field string }{
		{"pins.list", "/api/pins.list?channel=" + ch, "items"},
		{"reactions.list", "/api/reactions.list", "items"},
	} {
		t.Run(c.name, func(t *testing.T) {
			status, m := call(t, srv, "GET", c.path, "", "", true)
			mustOK(t, status, m)
			if v, ok := m[c.field].([]any); !ok || len(v) != 0 {
				t.Errorf("%s %s = %#v, want []", c.name, c.field, m[c.field])
			}
		})
	}

	resp := tc.Get("/admin/messages")
	resp.AssertStatus(200)
	if v, ok := resp.JSONMap()["messages"].([]any); !ok || len(v) != 0 {
		t.Errorf("/admin/messages messages = %#v, want []", resp.JSONMap()["messages"])
	}
}
