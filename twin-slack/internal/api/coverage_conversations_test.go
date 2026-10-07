package api_test

import (
	"net/url"
	"slices"
	"testing"
)

func channelInfo(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	ch, _ := m["channel"].(map[string]any)
	if ch == nil {
		t.Fatalf("no channel in %v", m)
	}
	return ch
}

// conversations.join adds the caller once; leave removes them, and leaving a
// channel the caller is not in still answers ok.
func TestConversationsJoinAndLeave(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "joinable")

	joined := formAs(t, srv, "xoxp-member", "conversations.join", url.Values{"channel": {ch}})
	mustOK(t, 200, joined)
	if c := channelInfo(t, joined); c["id"] != ch || c["is_member"] != true {
		t.Errorf("join answer: %v", c)
	}
	again := formAs(t, srv, "xoxp-member", "conversations.join", url.Values{"channel": {ch}})
	mustOK(t, 200, again)
	if n := channelInfo(t, again)["num_members"]; n != joined["channel"].(map[string]any)["num_members"] {
		t.Errorf("joining twice changed num_members: %v", n)
	}

	mustOK(t, 200, formAs(t, srv, "xoxp-member", "conversations.leave", url.Values{"channel": {ch}}))
	info := formAs(t, srv, "xoxp-member", "conversations.info", url.Values{"channel": {ch}})
	if channelInfo(t, info)["is_member"] != false {
		t.Errorf("still a member after leave: %v", info)
	}
	mustOK(t, 200, formAs(t, srv, "xoxp-member", "conversations.leave", url.Values{"channel": {ch}}))

	for _, method := range []string{"conversations.join", "conversations.leave"} {
		wantErrors(t, srv, method, map[string]errCase{
			"no channel":      {url.Values{}, "channel_not_found"},
			"unknown channel": {url.Values{"channel": {"CNOPE"}}, "channel_not_found"},
		})
	}
}

// conversations.close and conversations.mark acknowledge a known channel.
func TestConversationsCloseAndMark(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, ts := postIn(t, srv, "markme")
	mustOK(t, 200, form(t, srv, "conversations.mark", url.Values{"channel": {ch}, "ts": {ts}}))

	seedUsers(t, srv, twoUsers)
	dm := open(t, srv, url.Values{"users": {"U1"}})
	mustOK(t, 200, dm)
	id := channelInfo(t, dm)["id"].(string)
	mustOK(t, 200, form(t, srv, "conversations.close", url.Values{"channel": {id}}))
}

// Archiving twice is already_archived, unarchiving an open channel is
// not_archived, and an unknown channel is channel_not_found for both.
func TestConversationsArchiveAndUnarchive(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "archivable")

	mustOK(t, 200, form(t, srv, "conversations.archive", url.Values{"channel": {ch}}))
	if c := channelInfo(t, form(t, srv, "conversations.info", url.Values{"channel": {ch}})); c["is_archived"] != true {
		t.Errorf("not archived: %v", c)
	}
	wantErrors(t, srv, "conversations.archive", map[string]errCase{
		"twice":           {url.Values{"channel": {ch}}, "already_archived"},
		"unknown channel": {url.Values{"channel": {"CNOPE"}}, "channel_not_found"},
		"no channel":      {url.Values{}, "channel_not_found"},
	})

	mustOK(t, 200, form(t, srv, "conversations.unarchive", url.Values{"channel": {ch}}))
	if c := channelInfo(t, form(t, srv, "conversations.info", url.Values{"channel": {ch}})); c["is_archived"] != false {
		t.Errorf("still archived: %v", c)
	}
	wantErrors(t, srv, "conversations.unarchive", map[string]errCase{
		"open channel":    {url.Values{"channel": {ch}}, "not_archived"},
		"unknown channel": {url.Values{"channel": {"CNOPE"}}, "channel_not_found"},
	})
}

// Renaming to a taken name is name_taken; a free name is applied.
func TestConversationsRenameErrors(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "before")
	seedChannel(tc, "taken")

	m := form(t, srv, "conversations.rename", url.Values{"channel": {ch}, "name": {"after"}})
	mustOK(t, 200, m)
	if c := channelInfo(t, m); c["name"] != "after" || c["id"] != ch {
		t.Errorf("rename answer: %v", c)
	}
	wantErrors(t, srv, "conversations.rename", map[string]errCase{
		"taken name":      {url.Values{"channel": {ch}, "name": {"taken"}}, "name_taken"},
		"unknown channel": {url.Values{"channel": {"CNOPE"}, "name": {"x"}}, "channel_not_found"},
	})
}

// Kicking a member removes them; kicking a non-member is not_in_channel.
func TestConversationsKick(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "kickable")
	mustOK(t, 200, form(t, srv, "conversations.invite", url.Values{"channel": {ch}, "users": {"U_GUEST"}}))

	mustOK(t, 200, form(t, srv, "conversations.kick", url.Values{"channel": {ch}, "user": {"U_GUEST"}}))
	members := form(t, srv, "conversations.members", url.Values{"channel": {ch}})
	if ids := anyStrings(members["members"]); slices.Contains(ids, "U_GUEST") {
		t.Errorf("kicked user still a member: %v", ids)
	}
	wantErrors(t, srv, "conversations.kick", map[string]errCase{
		"not a member":    {url.Values{"channel": {ch}, "user": {"U_GUEST"}}, "not_in_channel"},
		"unknown channel": {url.Values{"channel": {"CNOPE"}, "user": {"U_GUEST"}}, "channel_not_found"},
	})
}
