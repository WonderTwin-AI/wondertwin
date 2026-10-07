package api_test

import (
	"net/url"
	"slices"
	"testing"
)

// An archived channel refuses ephemeral messages, joins and leaves with
// is_archived, as each method's docs list, and its membership is unchanged.
func TestArchivedChannelRefusesMembershipAndEphemeralCalls(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "museum")
	mustOK(t, 200, form(t, srv, "conversations.archive", url.Values{"channel": {ch}}))

	if m := form(t, srv, "chat.postEphemeral", url.Values{"channel": {ch}, "user": {"U_BOT"}, "text": {"x"}}); m["error"] != "is_archived" {
		t.Errorf("postEphemeral: %v", m)
	}
	if m := formAs(t, srv, "xoxp-visitor", "conversations.join", url.Values{"channel": {ch}}); m["error"] != "is_archived" {
		t.Errorf("join: %v", m)
	}
	if m := form(t, srv, "conversations.leave", url.Values{"channel": {ch}}); m["error"] != "is_archived" {
		t.Errorf("leave: %v", m)
	}
	info := form(t, srv, "conversations.info", url.Values{"channel": {ch}})["channel"].(map[string]any)
	members, _ := info["members"].([]any)
	if !slices.Contains(members, any("U_BOT")) || len(members) != 1 {
		t.Errorf("membership of an archived channel changed: %v", members)
	}
}

// An ephemeral message goes only to a user in the channel; anyone else,
// including a user who does not exist, is user_not_in_channel.
func TestEphemeralNeedsTheUserInTheChannel(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, guestUser)
	ch, _ := postIn(t, srv, "whisper")
	for name, user := range map[string]string{"a member of the workspace": "U_GUEST", "an unknown user": "U_GHOST"} {
		if m := form(t, srv, "chat.postEphemeral", url.Values{"channel": {ch}, "user": {user}, "text": {"x"}}); m["error"] != "user_not_in_channel" {
			t.Errorf("ephemeral to %s outside the channel: %v", name, m)
		}
	}
	mustOK(t, 200, form(t, srv, "conversations.invite", url.Values{"channel": {ch}, "users": {"U_GUEST"}}))
	mustOK(t, 200, form(t, srv, "chat.postEphemeral", url.Values{"channel": {ch}, "user": {"U_GUEST"}, "text": {"x"}}))
}
