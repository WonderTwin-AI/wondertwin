package api_test

import (
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func inSeconds(d time.Duration) string {
	return strconv.FormatInt(time.Now().Add(d).Unix(), 10)
}

func advanceClock(t *testing.T, srv *httptest.Server, d string) {
	t.Helper()
	status, m := call(t, srv, "POST", "/admin/time/advance", jsonType, `{"duration":"`+d+`"}`, false)
	if status != 200 {
		t.Fatalf("advance clock: %d %v", status, m)
	}
}

// chat.scheduleMessage checks when and what it schedules, with the errors the
// docs list, and answers the message as a delayed_message.
func TestScheduleMessageChecks(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "schedule-checks")
	soon := inSeconds(time.Hour)

	wantErrors(t, srv, "chat.scheduleMessage", map[string]errCase{
		"no post_at":        {url.Values{"channel": {ch}, "text": {"x"}}, "invalid_time"},
		"in the past":       {url.Values{"channel": {ch}, "text": {"x"}, "post_at": {inSeconds(-time.Minute)}}, "time_in_past"},
		"past 120 days":     {url.Values{"channel": {ch}, "text": {"x"}, "post_at": {inSeconds(121 * 24 * time.Hour)}}, "time_too_far"},
		"no text":           {url.Values{"channel": {ch}, "post_at": {soon}}, "no_text"},
		"markdown and text": {url.Values{"channel": {ch}, "post_at": {soon}, "text": {"a"}, "markdown_text": {"*a*"}}, "markdown_text_conflict"},
	})

	s := form(t, srv, "chat.scheduleMessage", url.Values{"channel": {ch}, "post_at": {soon},
		"text": {"fallback"}, "blocks": {`[{"type":"divider"}]`}, "attachments": {`[{"text":"att"}]`}})
	mustOK(t, 200, s)
	msg := s["message"].(map[string]any)
	if msg["type"] != "delayed_message" || msg["text"] != "fallback" || msg["bot_id"] != "B_BOT" || msg["blocks"] == nil || msg["attachments"] == nil {
		t.Errorf("scheduled message: %v", msg)
	}

	mustOK(t, 200, form(t, srv, "conversations.archive", url.Values{"channel": {ch}}))
	wantErrors(t, srv, "chat.scheduleMessage", map[string]errCase{
		"archived": {url.Values{"channel": {ch}, "text": {"x"}, "post_at": {soon}}, "is_archived"},
	})
}

// A scheduled message is posted when post_at passes on the app emulator's
// clock, in its thread if it has one, and leaves the scheduled list. One that
// carries metadata never posts, as the docs warn of Slack.
func TestScheduledMessagesPostWhenDue(t *testing.T) {
	srv, _ := setupSlack(t)
	rc := newReceiver(t)
	ch, parent := postIn(t, srv, "schedule-due")
	configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL, "signing_secret": "sch3d"})

	mustOK(t, 200, form(t, srv, "chat.scheduleMessage", url.Values{"channel": {ch}, "text": {"on time"}, "post_at": {inSeconds(10 * time.Minute)}}))
	mustOK(t, 200, form(t, srv, "chat.scheduleMessage", url.Values{"channel": {ch}, "text": {"in thread"}, "thread_ts": {parent}, "post_at": {inSeconds(20 * time.Minute)}}))
	mustOK(t, 200, form(t, srv, "chat.scheduleMessage", url.Values{"channel": {ch}, "text": {"with metadata"}, "post_at": {inSeconds(10 * time.Minute)},
		"metadata": {`{"event_type":"e","event_payload":{"k":"v"}}`}}))

	for _, text := range historyTexts(t, srv, ch) {
		if text == "on time" {
			t.Fatal("a scheduled message posted before post_at")
		}
	}

	advanceClock(t, srv, "30m")
	texts := historyTexts(t, srv, ch)
	if len(texts) != 2 || texts[0] != "on time" {
		t.Errorf("history after post_at: %v", texts)
	}
	replies := form(t, srv, "conversations.replies", url.Values{"channel": {ch}, "ts": {parent}})["messages"].([]any)
	if len(replies) != 2 || replies[1].(map[string]any)["text"] != "in thread" {
		t.Errorf("scheduled reply: %v", replies)
	}
	left := form(t, srv, "chat.scheduledMessages.list", nil)["scheduled_messages"].([]any)
	if len(left) != 1 || left[0].(map[string]any)["text"] != "with metadata" {
		t.Errorf("still scheduled: %v", left)
	}

	ev := innerEvents(t, rc.wait(t, 2), "sch3d")
	if ev[0]["text"] != "on time" || ev[1]["text"] != "in thread" {
		t.Errorf("scheduled messages' events: %v", ev)
	}
}

// chat.scheduledMessages.list shows a token only the messages it scheduled,
// filtered by channel and by post_at, a page at a time.
func TestScheduledMessagesList(t *testing.T) {
	srv, _ := setupSlack(t)
	a, _ := postIn(t, srv, "list-a")
	b, _ := postIn(t, srv, "list-b")
	at := func(d time.Duration) string { return inSeconds(d) }
	for _, c := range []struct{ ch, when string }{{a, at(time.Hour)}, {a, at(2 * time.Hour)}, {b, at(3 * time.Hour)}} {
		mustOK(t, 200, form(t, srv, "chat.scheduleMessage", url.Values{"channel": {c.ch}, "text": {"x"}, "post_at": {c.when}}))
	}
	mustOK(t, 200, formAs(t, srv, "xoxb-other-app", "chat.scheduleMessage", url.Values{"channel": {a}, "text": {"theirs"}, "post_at": {at(time.Hour)}}))

	count := func(v url.Values) int {
		m := form(t, srv, "chat.scheduledMessages.list", v)
		mustOK(t, 200, m)
		return len(m["scheduled_messages"].([]any))
	}
	if n := count(nil); n != 3 {
		t.Errorf("own scheduled messages: %d, want 3", n)
	}
	if n := count(url.Values{"channel": {a}}); n != 2 {
		t.Errorf("in channel a: %d, want 2", n)
	}
	if n := count(url.Values{"oldest": {at(90 * time.Minute)}}); n != 2 {
		t.Errorf("after 90 minutes: %d, want 2", n)
	}
	if n := count(url.Values{"latest": {at(90 * time.Minute)}}); n != 1 {
		t.Errorf("before 90 minutes: %d, want 1", n)
	}
	first := form(t, srv, "chat.scheduledMessages.list", url.Values{"limit": {"2"}})
	next := first["response_metadata"].(map[string]any)["next_cursor"].(string)
	if len(first["scheduled_messages"].([]any)) != 2 || next == "" {
		t.Fatalf("first page: %v", first)
	}
	if n := count(url.Values{"limit": {"2"}, "cursor": {next}}); n != 1 {
		t.Errorf("second page: %d, want 1", n)
	}
	wantErrors(t, srv, "chat.scheduledMessages.list", map[string]errCase{
		"unknown channel": {url.Values{"channel": {"CNOPE"}}, "invalid_channel"},
		"bad cursor":      {url.Values{"cursor": {"nope"}}, "invalid_cursor"},
	})
}

// chat.deleteScheduledMessage needs the channel the message is scheduled in.
func TestDeleteScheduledMessageChecksTheChannel(t *testing.T) {
	srv, _ := setupSlack(t)
	a, _ := postIn(t, srv, "del-a")
	b, _ := postIn(t, srv, "del-b")
	id := form(t, srv, "chat.scheduleMessage", url.Values{"channel": {a}, "text": {"x"}, "post_at": {inSeconds(time.Hour)}})["scheduled_message_id"].(string)
	wantErrors(t, srv, "chat.deleteScheduledMessage", map[string]errCase{
		"unknown channel": {url.Values{"channel": {"CNOPE"}, "scheduled_message_id": {id}}, "channel_not_found"},
		"other channel":   {url.Values{"channel": {b}, "scheduled_message_id": {id}}, "invalid_scheduled_message_id"},
	})
	mustOK(t, 200, form(t, srv, "chat.deleteScheduledMessage", url.Values{"channel": {a}, "scheduled_message_id": {id}}))
}
