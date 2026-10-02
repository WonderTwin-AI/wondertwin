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

	status, m = call(t, srv, "GET", "/api/conversations.replies?channel="+ch+"&ts=1.000000", "", "", true)
	mustOK(t, status, m)
	if msgs, ok := m["messages"].([]any); !ok || len(msgs) != 0 {
		t.Fatalf("replies to a thread with no messages should be [], got %#v", m["messages"])
	}
}
