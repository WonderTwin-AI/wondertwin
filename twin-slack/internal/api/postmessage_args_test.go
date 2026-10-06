package api_test

import (
	"net/url"
	"strings"
	"testing"
)

// chat.postMessage reflects username, icon_emoji and attachments in the
// returned message, as Slack's response example does.
func TestPostMessageIdentityAndAttachments(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "styled")
	m := form(t, srv, "chat.postMessage", url.Values{
		"channel":     {ch},
		"username":    {"ecto1"},
		"icon_emoji":  {":ghost:"},
		"attachments": {`[{"pretext":"pre-hello","text":"text-world"}]`},
	})
	mustOK(t, 200, m)
	msg := m["message"].(map[string]any)
	if msg["username"] != "ecto1" {
		t.Errorf("username: %v", msg["username"])
	}
	if icons, _ := msg["icons"].(map[string]any); icons["emoji"] != ":ghost:" {
		t.Errorf("icons: %v", msg["icons"])
	}
	atts, _ := msg["attachments"].([]any)
	if len(atts) != 1 || atts[0].(map[string]any)["text"] != "text-world" || atts[0].(map[string]any)["id"] != float64(1) {
		t.Errorf("attachments: %v", msg["attachments"])
	}

	many := "[" + strings.TrimSuffix(strings.Repeat(`{"text":"a"},`, 101), ",") + "]"
	wantErrors(t, srv, "chat.postMessage", map[string]errCase{
		"101 attachments":        {url.Values{"channel": {ch}, "attachments": {many}}, "too_many_attachments"},
		"attachments not a list": {url.Values{"channel": {ch}, "attachments": {`{"text":"a"}`}}, "invalid_arguments"},
		"metadata no type":       {url.Values{"channel": {ch}, "text": {"x"}, "metadata": {`{"event_payload":{}}`}}, "invalid_metadata_format"},
		"metadata no payload":    {url.Values{"channel": {ch}, "text": {"x"}, "metadata": {`{"event_type":"task_created"}`}}, "invalid_metadata_format"},
	})
}

// Metadata is kept, and conversations.history returns it only with
// include_all_metadata.
func TestPostMessageMetadataInHistory(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "meta")
	mustOK(t, 200, form(t, srv, "chat.postMessage", url.Values{
		"channel": {ch}, "text": {"task"},
		"metadata": {`{"event_type":"task_created","event_payload":{"id":"11223"}}`},
	}))
	latest := func(include string) map[string]any {
		v := url.Values{"channel": {ch}}
		if include != "" {
			v.Set("include_all_metadata", include)
		}
		return form(t, srv, "conversations.history", v)["messages"].([]any)[0].(map[string]any)
	}
	if _, ok := latest("")["metadata"]; ok {
		t.Errorf("metadata returned without include_all_metadata: %v", latest(""))
	}
	meta, _ := latest("true")["metadata"].(map[string]any)
	if meta["event_type"] != "task_created" || meta["event_payload"].(map[string]any)["id"] != "11223" {
		t.Errorf("metadata with include_all_metadata: %v", meta)
	}
}

// A reply with reply_broadcast is a thread_broadcast, shown in the channel's
// history as well as the thread; a plain reply is in the thread only.
func TestReplyBroadcastShowsInChannel(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, parent := postIn(t, srv, "threads")
	mustOK(t, 200, form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {"quiet"}, "thread_ts": {parent}}))
	b := form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {"loud"}, "thread_ts": {parent}, "reply_broadcast": {"true"}})
	mustOK(t, 200, b)
	if b["message"].(map[string]any)["subtype"] != "thread_broadcast" {
		t.Errorf("broadcast subtype: %v", b["message"])
	}
	var texts []string
	for _, m := range form(t, srv, "conversations.history", url.Values{"channel": {ch}})["messages"].([]any) {
		texts = append(texts, m.(map[string]any)["text"].(string))
	}
	if strings.Join(texts, ",") != "loud,x" {
		t.Errorf("channel history: %v, want the broadcast and the parent", texts)
	}
	if n := len(form(t, srv, "conversations.replies", url.Values{"channel": {ch}, "ts": {parent}})["messages"].([]any)); n != 3 {
		t.Errorf("thread: %d messages, want parent and two replies", n)
	}
}
