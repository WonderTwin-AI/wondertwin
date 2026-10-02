package api_test

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// seedUsers loads users into the emulator's state.
func seedUsers(t *testing.T, srv *httptest.Server, users string) {
	t.Helper()
	status, m := call(t, srv, "POST", "/admin/state", jsonType, `{"users":{`+users+`}}`, false)
	if status != 200 || m["status"] != "loaded" {
		t.Fatalf("seed users: %d %v", status, m)
	}
}

const twoUsers = `"U1":{"id":"U1","name":"ana","profile":{"email":"ana@example.com"}},
	"U2":{"id":"U2","name":"bo","profile":{"email":"bo@example.com"}},
	"U3":{"id":"U3","name":"gone","deleted":true}`

func open(t *testing.T, srv *httptest.Server, form url.Values) map[string]any {
	t.Helper()
	_, m := call(t, srv, "POST", "/api/conversations.open", formType, form.Encode(), true)
	return m
}

func TestConversationsOpenResumesTheSameDM(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, twoUsers)

	first := open(t, srv, url.Values{"users": {"U1"}})
	if first["ok"] != true {
		t.Fatalf("open: %v", first)
	}
	ch := first["channel"].(map[string]any)
	id := ch["id"].(string)
	if !strings.HasPrefix(id, "D") || len(ch) != 1 {
		t.Errorf("without return_im the channel is its D id only: %v", ch)
	}
	if _, has := first["already_open"]; has {
		t.Errorf("a new DM is not already open: %v", first)
	}

	again := open(t, srv, url.Values{"users": {"U1"}, "return_im": {"true"}})
	full := again["channel"].(map[string]any)
	if full["id"] != id || again["already_open"] != true || again["no_op"] != true {
		t.Errorf("a second open resumes the DM: %v", again)
	}
	if full["is_im"] != true || full["user"] != "U1" {
		t.Errorf("return_im answers the IM object: %v", full)
	}

	byChannel := open(t, srv, url.Values{"channel": {id}})
	if byChannel["channel"].(map[string]any)["id"] != id || byChannel["already_open"] != true {
		t.Errorf("open by channel: %v", byChannel)
	}

	// Naming the caller as well is the same conversation.
	if m := open(t, srv, url.Values{"users": {"U1,U_BOT"}}); m["channel"].(map[string]any)["id"] != id {
		t.Errorf("naming the caller opened a different conversation: %v", m)
	}
}

func TestConversationsOpenMultiPerson(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, twoUsers)
	m := open(t, srv, url.Values{"users": {"U1,U2"}, "return_im": {"true"}})
	ch := m["channel"].(map[string]any)
	if ch["is_mpim"] != true || ch["is_im"] == true || ch["num_members"] != float64(3) {
		t.Errorf("two users open a multi-person message: %v", ch)
	}
	if !strings.HasPrefix(ch["name"].(string), "mpdm-") {
		t.Errorf("mpim name: %v", ch["name"])
	}
	again := open(t, srv, url.Values{"users": {"U2,U1"}})
	if again["channel"].(map[string]any)["id"] != ch["id"] {
		t.Errorf("the same users in another order: %v", again)
	}
	dm := open(t, srv, url.Values{"users": {"U1"}})
	if dm["channel"].(map[string]any)["id"] == ch["id"] {
		t.Error("a DM with one of the users resumed the multi-person message")
	}
}

func TestConversationsOpenErrors(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, twoUsers)
	_, created := call(t, srv, "POST", "/api/conversations.create", formType, "name=general", true)
	channelID := created["channel"].(map[string]any)["id"].(string)

	for name, tc := range map[string]struct {
		form url.Values
		want string
	}{
		"nothing":         {url.Values{}, "users_list_not_supplied"},
		"unknown user":    {url.Values{"users": {"U1,UNOPE"}}, "user_not_found"},
		"deleted user":    {url.Values{"users": {"U3"}}, "user_disabled"},
		"nine users":      {url.Values{"users": {"U1,U2,U3,U4,U5,U6,U7,U8,U9"}}, "too_many_users"},
		"unknown channel": {url.Values{"channel": {"D_NOPE"}}, "channel_not_found"},
		"public channel":  {url.Values{"channel": {channelID}}, "method_not_supported_for_channel_type"},
	} {
		if m := open(t, srv, tc.form); m["ok"] != false || m["error"] != tc.want {
			t.Errorf("%s: want %s, got %v", name, tc.want, m)
		}
	}
}

func TestConversationsOpenPreventCreation(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, twoUsers)
	m := open(t, srv, url.Values{"users": {"U1"}, "prevent_creation": {"true"}})
	if m["ok"] != true || m["channel"] != nil {
		t.Errorf("prevent_creation with no DM: %v", m)
	}
	opened := open(t, srv, url.Values{"users": {"U1"}})
	m = open(t, srv, url.Values{"users": {"U1"}, "prevent_creation": {"true"}})
	if m["channel"].(map[string]any)["id"] != opened["channel"].(map[string]any)["id"] {
		t.Errorf("prevent_creation with a DM: %v", m)
	}
}

// The done definition of slack-dm-user, end to end.
func TestDirectMessageUseCase(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, twoUsers)
	_, found := call(t, srv, "POST", "/api/users.lookupByEmail", formType, "email=ana%40example.com", true)
	user := found["user"].(map[string]any)["id"].(string)
	dm := open(t, srv, url.Values{"users": {user}})["channel"].(map[string]any)["id"].(string)
	status, posted := call(t, srv, "POST", "/api/chat.postMessage", formType, url.Values{"channel": {dm}, "text": {"hello"}}.Encode(), true)
	mustOK(t, status, posted)
	if texts := historyTexts(t, srv, dm); len(texts) != 1 || texts[0] != "hello" {
		t.Errorf("DM history: %v", texts)
	}
}

// A channel that happens to have the same members is not a DM to resume.
func TestConversationsOpenIgnoresChannelsWithTheSameMembers(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, twoUsers)
	_, created := call(t, srv, "POST", "/api/conversations.create", formType, "name=pair", true)
	channelID := created["channel"].(map[string]any)["id"].(string)
	_, invited := call(t, srv, "POST", "/api/conversations.invite", formType, url.Values{"channel": {channelID}, "users": {"U1"}}.Encode(), true)
	members, _ := invited["channel"].(map[string]any)["members"].([]any)
	if len(members) != 2 {
		t.Fatalf("setup: the channel should have the bot and U1, has %v", invited)
	}
	m := open(t, srv, url.Values{"users": {"U1"}})
	if m["channel"].(map[string]any)["id"] == channelID {
		t.Errorf("conversations.open resumed a public channel: %v", m)
	}
}
