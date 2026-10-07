package api_test

import (
	"net/url"
	"testing"
)

// A message is text, blocks, attachments or markdown_text. markdown_text
// stands alone, on the methods that post or edit a message.
func TestMarkdownTextAndNoText(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "markdown")

	m := form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "markdown_text": {"**bold**"}})
	mustOK(t, 200, m)
	if msg := m["message"].(map[string]any); msg["text"] != "**bold**" {
		t.Errorf("markdown_text message: %v", msg)
	}
	ts := m["ts"].(string)

	for _, method := range []string{"chat.postMessage", "chat.postEphemeral"} {
		v := url.Values{"channel": {ch}, "user": {"U_BOT"}}
		wantErrors(t, srv, method, map[string]errCase{
			"with text":   {withArgs(v, "markdown_text", "**a**", "text", "a"), "markdown_text_conflict"},
			"with blocks": {withArgs(v, "markdown_text", "**a**", "blocks", `[{"type":"divider"}]`), "markdown_text_conflict"},
			"nothing":     {v, "no_text"},
		})
	}
	mustOK(t, 200, form(t, srv, "chat.postEphemeral", url.Values{"channel": {ch}, "user": {"U_BOT"}, "markdown_text": {"_hi_"}}))

	wantErrors(t, srv, "chat.update", map[string]errCase{
		"nothing":   {url.Values{"channel": {ch}, "ts": {ts}}, "no_text"},
		"with text": {url.Values{"channel": {ch}, "ts": {ts}, "markdown_text": {"x"}, "text": {"y"}}, "markdown_text_conflict"},
	})
	up := form(t, srv, "chat.update", url.Values{"channel": {ch}, "ts": {ts}, "markdown_text": {"*new*"}})
	mustOK(t, 200, up)
	if up["text"] != "*new*" {
		t.Errorf("update with markdown_text: %v", up)
	}
}

func withArgs(base url.Values, kv ...string) url.Values {
	out := url.Values{}
	for k, v := range base {
		out[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		out.Set(kv[i], kv[i+1])
	}
	return out
}

// Only a message's author updates it. Given text and no blocks, the message's
// blocks are dropped; attachments stay unless replaced, and [] removes them.
func TestChatUpdateRules(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "updates")
	posted := form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {"v1"},
		"blocks": {`[{"type":"divider"}]`}, "attachments": {`[{"text":"att"}]`}})
	ts := posted["ts"].(string)

	if m := formAs(t, srv, "xoxp-other", "chat.update", url.Values{"channel": {ch}, "ts": {ts}, "text": {"hijack"}}); m["error"] != "cant_update_message" {
		t.Errorf("another user's update: %v", m)
	}
	wantErrors(t, srv, "chat.update", map[string]errCase{
		"unknown channel": {url.Values{"channel": {"CNOPE"}, "ts": {ts}, "text": {"x"}}, "channel_not_found"},
		"unknown ts":      {url.Values{"channel": {ch}, "ts": {"1.000001"}, "text": {"x"}}, "message_not_found"},
	})

	up := form(t, srv, "chat.update", url.Values{"channel": {ch}, "ts": {ts}, "text": {"v2"}})
	mustOK(t, 200, up)
	msg := up["message"].(map[string]any)
	if msg["text"] != "v2" || msg["blocks"] != nil || msg["attachments"] == nil {
		t.Errorf("text-only update: %v", msg)
	}
	up = form(t, srv, "chat.update", url.Values{"channel": {ch}, "ts": {ts}, "text": {"v3"}, "attachments": {"[]"}})
	if msg := up["message"].(map[string]any); msg["attachments"] != nil {
		t.Errorf("attachments [] did not remove them: %v", msg)
	}

	mustOK(t, 200, form(t, srv, "conversations.archive", url.Values{"channel": {ch}}))
	wantErrors(t, srv, "chat.update", map[string]errCase{
		"archived": {url.Values{"channel": {ch}, "ts": {ts}, "text": {"x"}}, "is_inactive"},
	})
}

// A bot deletes only its own messages, a user their own, and an admin anyone's.
func TestChatDeleteRules(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, `"U_ADMIN":{"id":"U_ADMIN","name":"admin","is_admin":true}`)
	seedTokens(t, srv, `"xoxp-admin":{"token":"xoxp-admin","type":"user","user_id":"U_ADMIN"}`)
	ch, botTS := postIn(t, srv, "deletes")
	userTS := formAs(t, srv, "xoxp-author", "chat.postMessage", url.Values{"channel": {ch}, "text": {"mine"}})["ts"].(string)

	if m := formAs(t, srv, "xoxp-author", "chat.delete", url.Values{"channel": {ch}, "ts": {botTS}}); m["error"] != "cant_delete_message" {
		t.Errorf("a user deleting the bot's message: %v", m)
	}
	if m := form(t, srv, "chat.delete", url.Values{"channel": {ch}, "ts": {userTS}}); m["error"] != "cant_delete_message" {
		t.Errorf("the bot deleting a user's message: %v", m)
	}
	mustOK(t, 200, form(t, srv, "chat.delete", url.Values{"channel": {ch}, "ts": {botTS}}))
	mustOK(t, 200, formAs(t, srv, "xoxp-admin", "chat.delete", url.Values{"channel": {ch}, "ts": {userTS}}))
	wantErrors(t, srv, "chat.delete", map[string]errCase{
		"deleted twice":   {url.Values{"channel": {ch}, "ts": {botTS}}, "message_not_found"},
		"unknown channel": {url.Values{"channel": {"CNOPE"}, "ts": {botTS}}, "channel_not_found"},
	})
}

// A post to an archived channel is refused, and stores nothing.
func TestPostToArchivedChannel(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "archived-post")
	mustOK(t, 200, form(t, srv, "conversations.archive", url.Values{"channel": {ch}}))
	if m := form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {"late"}}); m["error"] != "is_archived" {
		t.Errorf("post to an archived channel: %v", m)
	}
	for _, text := range historyTexts(t, srv, ch) {
		if text == "late" {
			t.Error("a refused post was stored")
		}
	}
}

// A bot's post carries the bot, its app and its bot_profile, in the answer,
// the history and the message event. A user's post carries none of them.
func TestBotPostsCarryTheBotIdentity(t *testing.T) {
	srv, _ := setupSlack(t)
	rc := newReceiver(t)
	ch, _ := postIn(t, srv, "bot-identity")
	configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL, "signing_secret": "b0t"})

	msg := form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {"from the bot"}})["message"].(map[string]any)
	profile, _ := msg["bot_profile"].(map[string]any)
	if msg["bot_id"] != "B_BOT" || msg["app_id"] != "A_SIM" || profile["id"] != "B_BOT" || profile["app_id"] != "A_SIM" || profile["team_id"] != "T0001" {
		t.Errorf("bot post: %v", msg)
	}
	userMsg := formAs(t, srv, "xoxp-person", "chat.postMessage", url.Values{"channel": {ch}, "text": {"from a person"}})["message"].(map[string]any)
	if _, ok := userMsg["bot_id"]; ok {
		t.Errorf("a user's post carries a bot: %v", userMsg)
	}

	ev := innerEvents(t, rc.wait(t, 2), "b0t")
	if ev[0]["bot_id"] != "B_BOT" || ev[0]["app_id"] != "A_SIM" || ev[0]["bot_profile"] == nil {
		t.Errorf("bot message event: %v", ev[0])
	}
	if _, ok := ev[1]["bot_id"]; ok {
		t.Errorf("a user's message event carries a bot: %v", ev[1])
	}
}
