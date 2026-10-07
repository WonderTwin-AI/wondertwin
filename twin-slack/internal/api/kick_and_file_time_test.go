package api_test

import (
	"net/url"
	"slices"
	"testing"
)

// The caller cannot kick itself (cant_kick_self), and stays a member.
func TestKickRefusesTheCaller(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "self-kick")
	if m := form(t, srv, "conversations.kick", url.Values{"channel": {ch}, "user": {"U_BOT"}}); m["error"] != "cant_kick_self" {
		t.Errorf("kicking oneself: %v", m)
	}
	members := form(t, srv, "conversations.members", url.Values{"channel": {ch}})["members"].([]any)
	if !slices.Contains(members, any("U_BOT")) {
		t.Errorf("a refused self-kick removed the caller: %v", members)
	}
}

// A file's timestamp is its creation time, as created is.
func TestFileTimestampIsItsCreationTime(t *testing.T) {
	srv, _ := setupSlack(t)
	_, id := uploadTicket(t, srv, "t.txt", 1)
	f := fileInfo(t, srv, id)
	if f["timestamp"] == float64(0) || f["timestamp"] != f["created"] {
		t.Errorf("timestamp %v, created %v", f["timestamp"], f["created"])
	}
}
