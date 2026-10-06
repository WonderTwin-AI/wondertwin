package api_test

import (
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
)

// A user token's writes are the user's, not the bot's.
func TestWritesBelongToTheCaller(t *testing.T) {
	srv, _ := setupSlack(t)
	const user = "xoxp-writer"

	created := formAs(t, srv, user, "conversations.create", url.Values{"name": {"theirs"}})
	ch := created["channel"].(map[string]any)
	if ch["creator"] != "U_USER" || !slices.Equal(anyStrings(ch["members"]), []string{"U_USER"}) {
		t.Errorf("conversations.create by a user: %v", ch)
	}
	id := ch["id"].(string)

	posted := formAs(t, srv, user, "chat.postMessage", url.Values{"channel": {id}, "text": {"hi"}})
	if posted["message"].(map[string]any)["user"] != "U_USER" {
		t.Errorf("chat.postMessage by a user: %v", posted)
	}
	ts := posted["ts"].(string)
	updated := formAs(t, srv, user, "chat.update", url.Values{"channel": {id}, "ts": {ts}, "text": {"hi!"}})
	if by := updated["message"].(map[string]any)["edited"].(map[string]any)["user"]; by != "U_USER" {
		t.Errorf("chat.update records the editor: %v", by)
	}
	me := formAs(t, srv, user, "chat.meMessage", url.Values{"channel": {id}, "text": {"waves"}})
	mustOK(t, 200, me)

	topic := formAs(t, srv, user, "conversations.setTopic", url.Values{"channel": {id}, "topic": {"t"}})
	purpose := formAs(t, srv, user, "conversations.setPurpose", url.Values{"channel": {id}, "purpose": {"p"}})
	mustOK(t, 200, topic)
	mustOK(t, 200, purpose)
	info := form(t, srv, "conversations.info", url.Values{"channel": {id}})["channel"].(map[string]any)
	if info["topic"].(map[string]any)["creator"] != "U_USER" || info["purpose"].(map[string]any)["creator"] != "U_USER" {
		t.Errorf("topic and purpose record who set them: %v", info)
	}

	for _, m := range historyMessages(t, srv, id) {
		if m["user"] != "U_USER" {
			t.Errorf("a user's message in history is attributed to %v", m["user"])
		}
	}
}

// Joining and leaving change the caller's membership, not the bot's.
func TestJoinAndLeaveAreTheCallers(t *testing.T) {
	srv, _ := setupSlack(t)
	ch := form(t, srv, "conversations.create", url.Values{"name": {"botroom"}})["channel"].(map[string]any)["id"].(string)

	joined := formAs(t, srv, "xoxp-joiner", "conversations.join", url.Values{"channel": {ch}})
	members := anyStrings(joined["channel"].(map[string]any)["members"])
	slices.Sort(members)
	if !slices.Equal(members, []string{"U_BOT", "U_USER"}) {
		t.Errorf("a user joining a channel the bot is in: %v", members)
	}
	again := formAs(t, srv, "xoxp-joiner", "conversations.join", url.Values{"channel": {ch}})
	if n := len(anyStrings(again["channel"].(map[string]any)["members"])); n != 2 {
		t.Errorf("joining twice added the user twice: %d members", n)
	}

	mustOK(t, 200, formAs(t, srv, "xoxp-joiner", "conversations.leave", url.Values{"channel": {ch}}))
	left := anyStrings(form(t, srv, "conversations.info", url.Values{"channel": {ch}})["channel"].(map[string]any)["members"])
	if !slices.Equal(left, []string{"U_BOT"}) {
		t.Errorf("a user leaving removed the wrong member: %v", left)
	}
}

// is_member is the caller's membership, computed per request from Members.
func TestIsMemberIsTheCallers(t *testing.T) {
	srv, _ := setupSlack(t)
	const user, bot = "xoxp-member", "xoxb-test-token"
	isMember := func(token, method, channel string) bool {
		t.Helper()
		m := formAs(t, srv, token, method, url.Values{"channel": {channel}})
		mustOK(t, 200, m)
		return m["channel"].(map[string]any)["is_member"].(bool)
	}

	bots := form(t, srv, "conversations.create", url.Values{"name": {"bots"}})["channel"].(map[string]any)
	if bots["is_member"] != true {
		t.Errorf("conversations.create answers the creator's membership: %v", bots)
	}
	botsID := bots["id"].(string)
	if isMember(user, "conversations.info", botsID) {
		t.Error("a user is a member of a channel only the bot created")
	}

	joined := formAs(t, srv, user, "conversations.join", url.Values{"channel": {botsID}})
	if joined["channel"].(map[string]any)["is_member"] != true {
		t.Errorf("conversations.join answers the joiner's membership: %v", joined)
	}
	if !isMember(user, "conversations.info", botsID) || !isMember(bot, "conversations.info", botsID) {
		t.Error("after a user joins, both the user and the bot are members")
	}

	theirs := formAs(t, srv, user, "conversations.create", url.Values{"name": {"theirs"}})["channel"].(map[string]any)["id"].(string)
	if isMember(bot, "conversations.info", theirs) {
		t.Error("the bot is a member of a channel a user created without it")
	}
	formAs(t, srv, "xoxp-other", "conversations.join", url.Values{"channel": {theirs}})
	if isMember(bot, "conversations.info", theirs) {
		t.Error("a user joining made the bot a member")
	}

	mustOK(t, 200, formAs(t, srv, user, "conversations.leave", url.Values{"channel": {botsID}}))
	if isMember(user, "conversations.info", botsID) {
		t.Error("a user who left is still a member")
	}
	if !isMember(bot, "conversations.info", botsID) {
		t.Error("a user leaving removed the bot's membership")
	}

	for _, method := range []string{"conversations.list", "users.conversations"} {
		byCaller := map[string]map[string]bool{}
		for _, token := range []string{bot, user} {
			byCaller[token] = map[string]bool{}
			for _, c := range formAs(t, srv, token, method, nil)["channels"].([]any) {
				ch := c.(map[string]any)
				byCaller[token][ch["id"].(string)] = ch["is_member"].(bool)
			}
		}
		if got, ok := byCaller[bot][botsID]; !ok || !got {
			t.Errorf("%s: the bot is a member of its own channel: %v", method, byCaller[bot])
		}
		if got, ok := byCaller[bot][theirs]; ok && got {
			t.Errorf("%s: the bot is a member of the user's channel: %v", method, byCaller[bot])
		}
		if got, ok := byCaller[user][theirs]; !ok || !got {
			t.Errorf("%s: the user is a member of their own channel: %v", method, byCaller[user])
		}
		if got, ok := byCaller[user][botsID]; ok && got {
			t.Errorf("%s: the user is a member of a channel they left: %v", method, byCaller[user])
		}
	}
}

func anyStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func historyMessages(t *testing.T, srv *httptest.Server, channel string) []map[string]any {
	t.Helper()
	m := form(t, srv, "conversations.history", url.Values{"channel": {channel}})
	mustOK(t, 200, m)
	var out []map[string]any
	for _, v := range m["messages"].([]any) {
		out = append(out, v.(map[string]any))
	}
	if len(out) == 0 {
		t.Fatalf("no messages in %s", channel)
	}
	return out
}

// "Mine" defaults resolve to the caller: the uploader of a file, and the
// conversations and reactions listed when no user is named.
func TestDefaultsAreTheCallers(t *testing.T) {
	srv, _ := setupSlack(t)
	const user = "xoxp-me"

	up := formAs(t, srv, user, "files.getUploadURLExternal", url.Values{"filename": {"a.txt"}, "length": {"1"}})
	info := form(t, srv, "files.info", url.Values{"file": {up["file_id"].(string)}})
	if by := info["file"].(map[string]any)["user"]; by != "U_USER" {
		t.Errorf("files are the uploader's: %v", by)
	}

	mine := formAs(t, srv, user, "conversations.create", url.Values{"name": {"mine"}})["channel"].(map[string]any)["id"].(string)
	form(t, srv, "conversations.create", url.Values{"name": {"bots"}})
	listed := formAs(t, srv, user, "users.conversations", nil)["channels"].([]any)
	if len(listed) != 1 || listed[0].(map[string]any)["id"] != mine {
		t.Errorf("users.conversations with no user lists the caller's: %v", listed)
	}

	ts := formAs(t, srv, user, "chat.postMessage", url.Values{"channel": {mine}, "text": {"x"}})["ts"].(string)
	mustOK(t, 200, formAs(t, srv, user, "reactions.add", url.Values{"channel": {mine}, "timestamp": {ts}, "name": {"eyes"}}))
	if items := formAs(t, srv, user, "reactions.list", nil)["items"].([]any); len(items) != 1 {
		t.Errorf("reactions.list with no user lists the caller's: %v", items)
	}
	if items := form(t, srv, "reactions.list", nil)["items"].([]any); len(items) != 0 {
		t.Errorf("the bot sees the user's reactions as its own: %v", items)
	}
}

// A user's profile and presence changes are their own.
func TestProfileAndPresenceAreTheCallers(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, `"U_USER":{"id":"U_USER","name":"user","profile":{"real_name":"User"}},
		"U_BOT":{"id":"U_BOT","name":"bot","profile":{"real_name":"Bot"}}`)
	const user = "xoxp-profile"

	set := formAs(t, srv, user, "users.profile.set", url.Values{"profile": {`{"status_text":"lunch"}`}})
	mustOK(t, 200, set)
	mine := formAs(t, srv, user, "users.profile.get", nil)["profile"].(map[string]any)
	if mine["status_text"] != "lunch" || mine["real_name"] != "User" {
		t.Errorf("users.profile.get with no user is the caller's own profile, updated: %v", mine)
	}
	if got := form(t, srv, "users.profile.get", nil)["profile"].(map[string]any)["status_text"]; got == "lunch" {
		t.Error("a user's status was set on the bot")
	}

	mustOK(t, 200, formAs(t, srv, user, "users.setPresence", url.Values{"presence": {"away"}}))
	if got := form(t, srv, "users.getPresence", url.Values{"user": {"U_USER"}})["presence"]; got != "away" {
		t.Errorf("users.setPresence by a user: U_USER presence %v", got)
	}
	if got := form(t, srv, "users.getPresence", url.Values{"user": {"U_BOT"}})["presence"]; got == "away" {
		t.Error("a user's presence was set on the bot")
	}
	if got := form(t, srv, "users.info", url.Values{"user": {"U_USER"}})["user"].(map[string]any)["name"]; got != "user" {
		t.Errorf("users.setPresence replaced the user's record: name %v", got)
	}
}
