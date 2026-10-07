package api_test

import (
	"net/url"
	"slices"
	"testing"
)

// Slack refuses an invite naming a user that does not exist, or the caller,
// with each failure in errors, and invites nobody unless force is set
// (conversations.invite, "Invalid users"). A refused user is never a member
// and produces no member_joined_channel event.
func TestInviteRefusesUnknownUsersAndTheCaller(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, guestUser)
	rc := newReceiver(t)
	ch := form(t, srv, "conversations.create", url.Values{"name": {"door"}})["channel"].(map[string]any)["id"].(string)
	configureEvents(t, srv, map[string]any{"request_url": rc.srv.URL})

	m := form(t, srv, "conversations.invite", url.Values{"channel": {ch}, "users": {"U_GUEST,UNOPE"}})
	if m["ok"] != false || m["error"] != "user_not_found" {
		t.Fatalf("an unknown user among others: %v", m)
	}
	errs, _ := m["errors"].([]any)
	if len(errs) != 1 || errs[0].(map[string]any)["user"] != "UNOPE" || errs[0].(map[string]any)["error"] != "user_not_found" {
		t.Errorf("errors: %v", m["errors"])
	}
	if m := form(t, srv, "conversations.invite", url.Values{"channel": {ch}, "users": {"U_BOT"}}); m["error"] != "cant_invite_self" {
		t.Errorf("inviting oneself: %v", m)
	}
	if m := form(t, srv, "conversations.invite", url.Values{"channel": {ch}}); m["error"] != "no_user" {
		t.Errorf("no users: %v", m)
	}

	members := form(t, srv, "conversations.members", url.Values{"channel": {ch}})["members"].([]any)
	if slices.Contains(members, any("UNOPE")) || slices.Contains(members, any("U_GUEST")) {
		t.Errorf("a refused invite added members: %v", members)
	}
	if chans, _ := form(t, srv, "users.conversations", url.Values{"user": {"UNOPE"}})["channels"].([]any); len(chans) != 0 {
		t.Errorf("an unknown user is listed in channels: %v", chans)
	}

	// The first delivery is a message posted after the refused invites.
	form(t, srv, "chat.postMessage", url.Values{"channel": {ch}, "text": {"marker"}})
	if e := envelope(t, rc.wait(t, 1)[0])["event"].(map[string]any); e["type"] != "message" {
		t.Errorf("a refused invite was delivered: %v", e)
	}
}

// With force, the valid users are invited and the invalid ones disregarded; if
// none is valid, the invite is still refused.
func TestInviteWithForceSkipsInvalidUsers(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, guestUser)
	ch := form(t, srv, "conversations.create", url.Values{"name": {"forced"}})["channel"].(map[string]any)["id"].(string)

	m := form(t, srv, "conversations.invite", url.Values{"channel": {ch}, "users": {"UNOPE,U_GUEST"}, "force": {"true"}})
	mustOK(t, 200, m)
	members := form(t, srv, "conversations.members", url.Values{"channel": {ch}})["members"].([]any)
	if !slices.Contains(members, any("U_GUEST")) || slices.Contains(members, any("UNOPE")) {
		t.Errorf("members after a forced invite: %v", members)
	}
	if m := form(t, srv, "conversations.invite", url.Values{"channel": {ch}, "users": {"UNOPE"}, "force": {"true"}}); m["error"] != "user_not_found" {
		t.Errorf("a forced invite of only unknown users: %v", m)
	}
}

// An archived channel takes no invites.
func TestInviteToAnArchivedChannel(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, guestUser)
	ch := form(t, srv, "conversations.create", url.Values{"name": {"shut"}})["channel"].(map[string]any)["id"].(string)
	mustOK(t, 200, form(t, srv, "conversations.archive", url.Values{"channel": {ch}}))
	if m := form(t, srv, "conversations.invite", url.Values{"channel": {ch}, "users": {"U_GUEST"}}); m["error"] != "is_archived" {
		t.Errorf("invite to an archived channel: %v", m)
	}
}
