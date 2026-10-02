package api_test

import (
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
)

// listIDs returns the ids a list method answers with for the given arguments,
// called with token.
func listIDs(t *testing.T, srv *httptest.Server, token, method string, v url.Values) []string {
	t.Helper()
	m := formAs(t, srv, token, method, v)
	mustOK(t, 200, m)
	var ids []string
	for _, c := range m["channels"].([]any) {
		ids = append(ids, c.(map[string]any)["id"].(string))
	}
	return ids
}

// fourKinds makes one conversation of each type, as the bot, and returns
// their ids by type.
func fourKinds(t *testing.T, srv *httptest.Server) map[string]string {
	t.Helper()
	seedUsers(t, srv, `"U1":{"id":"U1","name":"ana"},"U2":{"id":"U2","name":"bo"}`)
	id := func(m map[string]any) string { return m["channel"].(map[string]any)["id"].(string) }
	return map[string]string{
		"public_channel":  id(form(t, srv, "conversations.create", url.Values{"name": {"pub"}})),
		"private_channel": id(form(t, srv, "conversations.create", url.Values{"name": {"priv"}, "is_private": {"true"}})),
		"im":              id(form(t, srv, "conversations.open", url.Values{"users": {"U1"}})),
		"mpim":            id(form(t, srv, "conversations.open", url.Values{"users": {"U1,U2"}})),
	}
}

func TestConversationsListTypes(t *testing.T) {
	srv, _ := setupSlack(t)
	ids := fourKinds(t, srv)
	const bot = "xoxb-lister"

	if got := listIDs(t, srv, bot, "conversations.list", nil); !slices.Equal(got, []string{ids["public_channel"]}) {
		t.Errorf("default lists public channels only: %v (made %v)", got, ids)
	}
	for _, typ := range []string{"public_channel", "private_channel", "im", "mpim"} {
		if got := listIDs(t, srv, bot, "conversations.list", url.Values{"types": {typ}}); !slices.Equal(got, []string{ids[typ]}) {
			t.Errorf("types=%s: %v", typ, got)
		}
	}
	all := listIDs(t, srv, bot, "conversations.list", url.Values{"types": {"public_channel, private_channel,mpim,im"}})
	if len(all) != 4 {
		t.Errorf("every type: %v", all)
	}
	if m := form(t, srv, "conversations.list", url.Values{"types": {"public_channel,channels"}}); m["error"] != "invalid_types" {
		t.Errorf("an unknown type: %v", m)
	}
}

// A caller who is not a member does not see private conversations.
func TestConversationsListHidesOthersPrivateConversations(t *testing.T) {
	srv, _ := setupSlack(t)
	ids := fourKinds(t, srv)
	got := listIDs(t, srv, "xoxp-outsider", "conversations.list", url.Values{"types": {"public_channel,private_channel,mpim,im"}})
	if !slices.Equal(got, []string{ids["public_channel"]}) {
		t.Errorf("a non-member listed %v", got)
	}
}

func TestUsersConversationsTypes(t *testing.T) {
	srv, _ := setupSlack(t)
	ids := fourKinds(t, srv)
	const bot = "xoxb-lister"

	if got := listIDs(t, srv, bot, "users.conversations", nil); !slices.Equal(got, []string{ids["public_channel"]}) {
		t.Errorf("default lists public channels only: %v", got)
	}
	got := listIDs(t, srv, bot, "users.conversations", url.Values{"types": {"im,mpim"}})
	want := []string{ids["im"], ids["mpim"]}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("types=im,mpim: %v", got)
	}
	if m := form(t, srv, "users.conversations", url.Values{"types": {"dm"}}); m["error"] != "invalid_types" {
		t.Errorf("an unknown type: %v", m)
	}

	// U1 is in the DM and the multi-person message, but a caller outside them
	// does not see them in U1's list.
	if got := listIDs(t, srv, "xoxp-outsider", "users.conversations", url.Values{"user": {"U1"}, "types": {"im,mpim"}}); len(got) != 0 {
		t.Errorf("U1's DMs listed to an outsider: %v", got)
	}
	if got := listIDs(t, srv, bot, "users.conversations", url.Values{"user": {"U1"}, "types": {"im,mpim"}}); len(got) != 2 {
		t.Errorf("U1's DMs listed to the bot, a member: %v", got)
	}
}
