package api_test

import (
	"net/url"
	"testing"
)

// The error answers of methods whose success paths are tested elsewhere.
func TestUsersMethodErrors(t *testing.T) {
	srv, _ := setupSlack(t)
	wantErrors(t, srv, "users.info", map[string]errCase{
		"unknown user": {url.Values{"user": {"U-nope"}}, "user_not_found"},
		"no user":      {url.Values{}, "user_not_found"},
	})
	wantErrors(t, srv, "users.lookupByEmail", map[string]errCase{
		"unknown email": {url.Values{"email": {"nobody@example.com"}}, "users_not_found"},
		"no email":      {url.Values{}, "users_not_found"},
	})
	wantErrors(t, srv, "users.profile.get", map[string]errCase{
		"unknown user": {url.Values{"user": {"U-nope"}}, "user_not_found"},
	})
	wantErrors(t, srv, "users.profile.set", map[string]errCase{
		"unknown user": {url.Values{"user": {"U-nope"}, "name": {"status_text"}, "value": {"x"}}, "user_not_found"},
	})
	wantErrors(t, srv, "users.list", map[string]errCase{
		"bad cursor": {url.Values{"cursor": {"not-a-cursor"}}, "invalid_cursor"},
	})
	wantErrors(t, srv, "users.conversations", map[string]errCase{
		"bad types": {url.Values{"types": {"nonsense"}}, "invalid_types"},
	})
}

// Presence round trips are in TestProfileAndPresenceAreTheCallers
// (attribution_test.go) and TestPresenceIsTheCallers (coverage_fixes_test.go).

func TestConversationsMethodErrors(t *testing.T) {
	srv, _ := setupSlack(t)
	for _, method := range []string{"conversations.setTopic", "conversations.setPurpose", "conversations.members", "conversations.info"} {
		wantErrors(t, srv, method, map[string]errCase{
			"unknown channel": {url.Values{"channel": {"CNOPE"}, "topic": {"x"}, "purpose": {"x"}}, "channel_not_found"},
			"no channel":      {url.Values{"topic": {"x"}, "purpose": {"x"}}, "channel_not_found"},
		})
	}
	wantErrors(t, srv, "conversations.list", map[string]errCase{
		"bad cursor": {url.Values{"cursor": {"not-a-cursor"}}, "invalid_cursor"},
		"bad types":  {url.Values{"types": {"nonsense"}}, "invalid_types"},
	})
	wantErrors(t, srv, "conversations.create", map[string]errCase{
		"no name": {url.Values{}, "invalid_name_required"},
	})
}
