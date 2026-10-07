package api_test

import (
	"net/url"
	"testing"
)

// innerEvents decodes the inner event of each delivery, checking every
// delivery's signature on the way.
func innerEvents(t *testing.T, got []received, secret string) []map[string]any {
	t.Helper()
	out := make([]map[string]any, len(got))
	for i, r := range got {
		verify(t, r, secret)
		out[i] = envelope(t, r)["event"].(map[string]any)
	}
	return out
}

// Joining, inviting, leaving and removing are delivered as
// member_joined_channel and member_left_channel, naming the member.
func TestMembershipChangesAreDeliveredAsEvents(t *testing.T) {
	srv, _ := setupSlack(t)
	rc := newReceiver(t)
	ch := form(t, srv, "conversations.create", url.Values{"name": {"members"}})["channel"].(map[string]any)["id"].(string)
	configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL, "signing_secret": "m3mb3rs"})

	mustOK(t, 200, formAs(t, srv, "xoxp-joiner", "conversations.join", url.Values{"channel": {ch}}))
	mustOK(t, 200, form(t, srv, "conversations.invite", url.Values{"channel": {ch}, "users": {"U_GUEST"}}))
	mustOK(t, 200, formAs(t, srv, "xoxp-joiner", "conversations.leave", url.Values{"channel": {ch}}))
	mustOK(t, 200, form(t, srv, "conversations.kick", url.Values{"channel": {ch}, "user": {"U_GUEST"}}))

	ev := innerEvents(t, rc.wait(t, 4), "m3mb3rs")
	want := []struct{ typ, user, inviter string }{
		{"member_joined_channel", "U_USER", ""},
		{"member_joined_channel", "U_GUEST", "U_BOT"},
		{"member_left_channel", "U_USER", ""},
		{"member_left_channel", "U_GUEST", ""},
	}
	for i, w := range want {
		e := ev[i]
		if e["type"] != w.typ || e["user"] != w.user || e["channel"] != ch || e["channel_type"] != "C" || e["team"] != "T0001" {
			t.Errorf("event %d: %v, want %s for %s", i, e, w.typ, w.user)
		}
		if inviter, ok := e["inviter"]; (w.inviter == "" && ok) || (w.inviter != "" && inviter != w.inviter) {
			t.Errorf("event %d inviter: %v, want %q", i, e["inviter"], w.inviter)
		}
		if ts, _ := e["event_ts"].(string); ts == "" {
			t.Errorf("event %d has no event_ts: %v", i, e)
		}
	}
}

// A call that changes no membership sends no member event: joining a channel
// the caller is already in, inviting a member, or leaving a channel the caller
// is not in.
func TestNoMemberEventWithoutAMembershipChange(t *testing.T) {
	srv, _ := setupSlack(t)
	rc := newReceiver(t)
	ch := form(t, srv, "conversations.create", url.Values{"name": {"steady"}})["channel"].(map[string]any)["id"].(string)
	configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL})

	mustOK(t, 200, form(t, srv, "conversations.join", url.Values{"channel": {ch}}))
	if invited := form(t, srv, "conversations.invite", url.Values{"channel": {ch}, "users": {"U_BOT"}}); invited["ok"] != false || invited["error"] != "already_in_channel" {
		t.Errorf("inviting a member: %v, want already_in_channel", invited)
	}
	mustOK(t, 200, formAs(t, srv, "xoxp-outsider", "conversations.leave", url.Values{"channel": {ch}}))

	// A message after them is the first delivery.
	form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {"marker"}})
	if e := envelope(t, rc.wait(t, 1)[0])["event"].(map[string]any); e["type"] != "message" {
		t.Errorf("first delivery: %v", e)
	}
}

// A private channel's member events carry channel_type G.
func TestPrivateChannelMemberEventsAreTypeG(t *testing.T) {
	srv, _ := setupSlack(t)
	rc := newReceiver(t)
	ch := form(t, srv, "conversations.create", url.Values{"name": {"hush"}, "is_private": {"true"}})["channel"].(map[string]any)["id"].(string)
	configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL})

	mustOK(t, 200, form(t, srv, "conversations.invite", url.Values{"channel": {ch}, "users": {"U_GUEST"}}))
	if e := envelope(t, rc.wait(t, 1)[0])["event"].(map[string]any); e["channel_type"] != "G" {
		t.Errorf("private channel member event: %v", e)
	}
}

// Slack refuses an invite naming only members with already_in_channel, and an
// invite naming members and newcomers adds only the newcomers.
func TestInviteOfExistingMembers(t *testing.T) {
	srv, _ := setupSlack(t)
	ch := form(t, srv, "conversations.create", url.Values{"name": {"guests"}})["channel"].(map[string]any)["id"].(string)

	if m := form(t, srv, "conversations.invite", url.Values{"channel": {ch}, "users": {"U_BOT"}}); m["ok"] != false || m["error"] != "already_in_channel" {
		t.Errorf("inviting only a member: %v", m)
	}
	mixed := form(t, srv, "conversations.invite", url.Values{"channel": {ch}, "users": {"U_BOT,U_GUEST"}})
	mustOK(t, 200, mixed)
	if n := mixed["channel"].(map[string]any)["num_members"]; n != float64(2) {
		t.Errorf("num_members after a mixed invite: %v, want 2", n)
	}
	if m := form(t, srv, "conversations.invite", url.Values{"channel": {ch}, "users": {"U_GUEST,U_BOT"}}); m["error"] != "already_in_channel" {
		t.Errorf("inviting two members: %v", m)
	}
}

// Joining a channel the caller is already in succeeds with Slack's
// already_in_channel warning, in warning and in response_metadata.warnings.
// A first join carries no warning.
func TestJoinWhenAlreadyInChannelWarns(t *testing.T) {
	srv, _ := setupSlack(t)
	ch := form(t, srv, "conversations.create", url.Values{"name": {"warned"}})["channel"].(map[string]any)["id"].(string)

	first := formAs(t, srv, "xoxp-joiner", "conversations.join", url.Values{"channel": {ch}})
	mustOK(t, 200, first)
	if _, ok := first["warning"]; ok {
		t.Errorf("a first join warns: %v", first)
	}
	again := formAs(t, srv, "xoxp-joiner", "conversations.join", url.Values{"channel": {ch}})
	mustOK(t, 200, again)
	if again["warning"] != "already_in_channel" {
		t.Errorf("warning: %v", again["warning"])
	}
	meta, _ := again["response_metadata"].(map[string]any)
	if ws, _ := meta["warnings"].([]any); len(ws) != 1 || ws[0] != "already_in_channel" {
		t.Errorf("response_metadata.warnings: %v", again["response_metadata"])
	}
	if again["channel"].(map[string]any)["id"] != ch {
		t.Errorf("channel: %v", again["channel"])
	}
}
