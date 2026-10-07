package api_test

import (
	"net/url"
	"testing"
	"time"
)

// A message event carries the message as history holds it, so subscribers
// that branch on subtype see a file share, a /me message and a thread
// broadcast for what they are (the message event docs, "Message subtypes").
func TestMessageEventsKeepTheirSubtypeAndContent(t *testing.T) {
	srv, _ := setupSlack(t)
	rc := newReceiver(t)
	ch, parent := postIn(t, srv, "shapes")
	configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL, "signing_secret": "sh4pes"})

	u, id := uploadTicket(t, srv, "plan.txt", 4)
	postBytes(t, u, "", []byte("plan"))
	mustOK(t, 200, form(t, srv, "files.completeUploadExternal", url.Values{
		"files": {`[{"id":"` + id + `","title":"Plan"}]`}, "channel_id": {ch}, "initial_comment": {"look"}}))
	mustOK(t, 200, form(t, srv, "chat.meMessage", url.Values{"channel": {ch}, "text": {"waves"}}))
	mustOK(t, 200, form(t, srv, "chat.postMessage", url.Values{
		"channel": {ch}, "text": {"also here"}, "thread_ts": {parent}, "reply_broadcast": {"true"},
		"attachments": {`[{"text":"att"}]`}}))

	ev := innerEvents(t, rc.wait(t, 3), "sh4pes")

	share := ev[0]
	if share["subtype"] != "file_share" || share["text"] != "look" {
		t.Errorf("file share event: %v", share)
	}
	files, _ := share["files"].([]any)
	if len(files) != 1 || files[0].(map[string]any)["id"] != id {
		t.Errorf("file share event files: %v", share["files"])
	}

	if me := ev[1]; me["subtype"] != "me_message" || me["text"] != "waves" || me["bot_id"] == nil {
		t.Errorf("me_message event: %v", me)
	}

	b := ev[2]
	if b["subtype"] != "thread_broadcast" || b["thread_ts"] != parent {
		t.Errorf("thread broadcast event: %v", b)
	}
	if atts, _ := b["attachments"].([]any); len(atts) != 1 || atts[0].(map[string]any)["text"] != "att" {
		t.Errorf("thread broadcast attachments: %v", b["attachments"])
	}
	for i, e := range ev {
		if e["channel"] != ch || e["channel_type"] != "channel" || e["event_ts"] != e["ts"] {
			t.Errorf("event %d envelope fields: %v", i, e)
		}
	}
}

// A scheduled message is delivered with what was scheduled: its metadata and
// attachments reach the event as they reach history.
func TestScheduledDeliveryEventKeepsTheMessage(t *testing.T) {
	srv, _ := setupSlack(t)
	rc := newReceiver(t)
	ch, _ := postIn(t, srv, "scheduled-shape")
	configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL, "signing_secret": "l4ter"})

	mustOK(t, 200, form(t, srv, "chat.scheduleMessage", url.Values{"channel": {ch}, "text": {"later"},
		"post_at": {inSeconds(10 * time.Minute)}, "attachments": {`[{"text":"att"}]`}}))
	advanceClock(t, srv, "15m")
	historyTexts(t, srv, ch) // any call delivers what is due

	e := innerEvents(t, rc.wait(t, 1), "l4ter")[0]
	if e["text"] != "later" || e["bot_id"] == nil {
		t.Errorf("scheduled delivery event: %v", e)
	}
	if atts, _ := e["attachments"].([]any); len(atts) != 1 {
		t.Errorf("scheduled delivery attachments: %v", e["attachments"])
	}
}

// An archived channel takes no /me message, and stores none.
func TestMeMessageToAnArchivedChannel(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "archived-me")
	mustOK(t, 200, form(t, srv, "conversations.archive", url.Values{"channel": {ch}}))
	if m := form(t, srv, "chat.meMessage", url.Values{"channel": {ch}, "text": {"waves"}}); m["error"] != "is_archived" {
		t.Errorf("meMessage to an archived channel: %v", m)
	}
	for _, text := range historyTexts(t, srv, ch) {
		if text == "waves" {
			t.Error("a refused /me message was stored")
		}
	}
}
