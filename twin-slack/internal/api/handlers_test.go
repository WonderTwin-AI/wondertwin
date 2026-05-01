package api_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

func setupSlack(t *testing.T) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-slack-test"}
	twin := twincore.New(cfg)
	handler := api.NewHandler(memStore, twin.Middleware())
	handler.Routes(twin.Router)
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.WireFromTwin(twin)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	tc := testutil.NewTwinClient(t, srv)
	return srv, tc
}

var slackHeaders = map[string]string{
	"Authorization": "Bearer xoxb-test-token",
	"Content-Type":  "application/json",
}

func slackPost(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("POST", path, body, slackHeaders)
}

func seedChannel(tc *testutil.TwinClient, name string) string {
	resp := slackPost(tc, "/api/conversations.create", map[string]any{"name": name})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	ch := m["channel"].(map[string]any)
	return ch["id"].(string)
}

// --- Auth Tests ---

func TestAuthTest(t *testing.T) {
	_, tc := setupSlack(t)
	resp := slackPost(tc, "/api/auth.test", nil)
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["ok"] != true {
		t.Error("expected ok=true")
	}
	if m["team_id"] == nil {
		t.Error("expected team_id")
	}
}

func TestAuthRequired(t *testing.T) {
	_, tc := setupSlack(t)
	resp := tc.Post("/api/auth.test", nil)
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["ok"] != false {
		t.Error("expected ok=false")
	}
	if m["error"] != "not_authed" {
		t.Errorf("expected error=not_authed, got %v", m["error"])
	}
}

// --- Chat Tests ---

func TestChatPostMessage(t *testing.T) {
	_, tc := setupSlack(t)
	chID := seedChannel(tc, "general")

	resp := slackPost(tc, "/api/chat.postMessage", map[string]any{
		"channel": chID,
		"text":    "Hello, world!",
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["ok"] != true {
		t.Error("expected ok=true")
	}
	if m["ts"] == nil {
		t.Error("expected ts in response")
	}
}

func TestChatPostMessageNoText(t *testing.T) {
	_, tc := setupSlack(t)
	chID := seedChannel(tc, "general")

	resp := slackPost(tc, "/api/chat.postMessage", map[string]any{
		"channel": chID,
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["error"] != "no_text" {
		t.Errorf("expected error=no_text, got %v", m["error"])
	}
}

func TestChatPostMessageChannelNotFound(t *testing.T) {
	_, tc := setupSlack(t)

	resp := slackPost(tc, "/api/chat.postMessage", map[string]any{
		"channel": "C_NONEXISTENT",
		"text":    "hello",
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["error"] != "channel_not_found" {
		t.Errorf("expected channel_not_found, got %v", m["error"])
	}
}

func TestChatUpdateAndDelete(t *testing.T) {
	_, tc := setupSlack(t)
	chID := seedChannel(tc, "general")

	// Post a message
	resp := slackPost(tc, "/api/chat.postMessage", map[string]any{
		"channel": chID,
		"text":    "original",
	})
	ts := resp.JSONMap()["ts"].(string)

	// Update it
	resp = slackPost(tc, "/api/chat.update", map[string]any{
		"channel": chID,
		"ts":      ts,
		"text":    "updated",
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["ok"] != true {
		t.Error("expected ok=true on update")
	}

	// Delete it
	resp = slackPost(tc, "/api/chat.delete", map[string]any{
		"channel": chID,
		"ts":      ts,
	})
	resp.AssertStatus(200)
	if resp.JSONMap()["ok"] != true {
		t.Error("expected ok=true on delete")
	}

	// Should not appear in history
	resp = slackPost(tc, "/api/conversations.history", map[string]any{
		"channel": chID,
	})
	msgsRaw := resp.JSONMap()["messages"]
	if msgsRaw != nil {
		msgs := msgsRaw.([]any)
		if len(msgs) != 0 {
			t.Errorf("expected 0 messages after delete, got %d", len(msgs))
		}
	}
}

func TestChatScheduleMessage(t *testing.T) {
	_, tc := setupSlack(t)
	chID := seedChannel(tc, "general")

	resp := slackPost(tc, "/api/chat.scheduleMessage", map[string]any{
		"channel": chID,
		"text":    "scheduled",
		"post_at": 9999999999,
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["scheduled_message_id"] == nil {
		t.Error("expected scheduled_message_id")
	}

	// List scheduled
	resp = slackPost(tc, "/api/chat.scheduledMessages.list", nil)
	msgs := resp.JSONMap()["scheduled_messages"].([]any)
	if len(msgs) != 1 {
		t.Errorf("expected 1 scheduled message, got %d", len(msgs))
	}

	// Delete scheduled
	slackPost(tc, "/api/chat.deleteScheduledMessage", map[string]any{
		"channel":              chID,
		"scheduled_message_id": m["scheduled_message_id"],
	}).AssertStatus(200)
}

// --- Conversations Tests ---

func TestConversationsCreateAndList(t *testing.T) {
	_, tc := setupSlack(t)

	chID := seedChannel(tc, "test-channel")
	if chID == "" {
		t.Fatal("expected channel ID")
	}

	resp := slackPost(tc, "/api/conversations.list", nil)
	resp.AssertStatus(200)
	channels := resp.JSONMap()["channels"].([]any)
	if len(channels) != 1 {
		t.Errorf("expected 1 channel, got %d", len(channels))
	}
}

func TestConversationsCreateDuplicate(t *testing.T) {
	_, tc := setupSlack(t)
	seedChannel(tc, "dupe")

	resp := slackPost(tc, "/api/conversations.create", map[string]any{"name": "dupe"})
	m := resp.JSONMap()
	if m["error"] != "name_taken" {
		t.Errorf("expected name_taken, got %v", m["error"])
	}
}

func TestConversationsInfo(t *testing.T) {
	_, tc := setupSlack(t)
	chID := seedChannel(tc, "info-test")

	resp := slackPost(tc, "/api/conversations.info", map[string]any{"channel": chID})
	resp.AssertStatus(200)
	ch := resp.JSONMap()["channel"].(map[string]any)
	if ch["name"] != "info-test" {
		t.Errorf("expected name=info-test, got %v", ch["name"])
	}
}

func TestConversationsHistory(t *testing.T) {
	_, tc := setupSlack(t)
	chID := seedChannel(tc, "history-test")

	slackPost(tc, "/api/chat.postMessage", map[string]any{"channel": chID, "text": "msg1"})
	slackPost(tc, "/api/chat.postMessage", map[string]any{"channel": chID, "text": "msg2"})

	resp := slackPost(tc, "/api/conversations.history", map[string]any{"channel": chID})
	msgs := resp.JSONMap()["messages"].([]any)
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
}

func TestConversationsArchiveUnarchive(t *testing.T) {
	_, tc := setupSlack(t)
	chID := seedChannel(tc, "archive-test")

	slackPost(tc, "/api/conversations.archive", map[string]any{"channel": chID}).AssertStatus(200)

	resp := slackPost(tc, "/api/conversations.info", map[string]any{"channel": chID})
	ch := resp.JSONMap()["channel"].(map[string]any)
	if ch["is_archived"] != true {
		t.Error("expected is_archived=true")
	}

	slackPost(tc, "/api/conversations.unarchive", map[string]any{"channel": chID}).AssertStatus(200)

	resp = slackPost(tc, "/api/conversations.info", map[string]any{"channel": chID})
	ch = resp.JSONMap()["channel"].(map[string]any)
	if ch["is_archived"] != false {
		t.Error("expected is_archived=false")
	}
}

func TestConversationsRename(t *testing.T) {
	_, tc := setupSlack(t)
	chID := seedChannel(tc, "old-name")

	resp := slackPost(tc, "/api/conversations.rename", map[string]any{
		"channel": chID,
		"name":    "new-name",
	})
	resp.AssertStatus(200)
	ch := resp.JSONMap()["channel"].(map[string]any)
	if ch["name"] != "new-name" {
		t.Errorf("expected name=new-name, got %v", ch["name"])
	}
}

func TestConversationsSetTopicAndPurpose(t *testing.T) {
	_, tc := setupSlack(t)
	chID := seedChannel(tc, "topic-test")

	slackPost(tc, "/api/conversations.setTopic", map[string]any{
		"channel": chID,
		"topic":   "test topic",
	}).AssertStatus(200)

	slackPost(tc, "/api/conversations.setPurpose", map[string]any{
		"channel": chID,
		"purpose": "test purpose",
	}).AssertStatus(200)

	resp := slackPost(tc, "/api/conversations.info", map[string]any{"channel": chID})
	ch := resp.JSONMap()["channel"].(map[string]any)
	topic := ch["topic"].(map[string]any)
	purpose := ch["purpose"].(map[string]any)
	if topic["value"] != "test topic" {
		t.Errorf("expected topic=test topic, got %v", topic["value"])
	}
	if purpose["value"] != "test purpose" {
		t.Errorf("expected purpose=test purpose, got %v", purpose["value"])
	}
}

func TestConversationsInviteAndKick(t *testing.T) {
	_, tc := setupSlack(t)
	chID := seedChannel(tc, "invite-test")

	// Seed a user
	tc.Post("/admin/state", json.RawMessage(`{"users":{"U001":{"id":"U001","name":"alice","real_name":"Alice"}}}`)).AssertStatus(200)

	slackPost(tc, "/api/conversations.invite", map[string]any{
		"channel": chID,
		"users":   "U001",
	}).AssertStatus(200)

	resp := slackPost(tc, "/api/conversations.members", map[string]any{"channel": chID})
	members := resp.JSONMap()["members"].([]any)
	if len(members) != 2 {
		t.Errorf("expected 2 members, got %d", len(members))
	}

	slackPost(tc, "/api/conversations.kick", map[string]any{
		"channel": chID,
		"user":    "U001",
	}).AssertStatus(200)

	resp = slackPost(tc, "/api/conversations.members", map[string]any{"channel": chID})
	members = resp.JSONMap()["members"].([]any)
	if len(members) != 1 {
		t.Errorf("expected 1 member after kick, got %d", len(members))
	}
}

// --- Users Tests ---

func TestUsersListAndInfo(t *testing.T) {
	_, tc := setupSlack(t)

	tc.Post("/admin/state", json.RawMessage(`{"users":{
		"U001":{"id":"U001","name":"alice","real_name":"Alice","profile":{"email":"alice@example.com"}},
		"U002":{"id":"U002","name":"bob","real_name":"Bob","profile":{"email":"bob@example.com"}}
	}}`)).AssertStatus(200)

	resp := slackPost(tc, "/api/users.list", nil)
	members := resp.JSONMap()["members"].([]any)
	if len(members) != 2 {
		t.Errorf("expected 2 users, got %d", len(members))
	}

	resp = slackPost(tc, "/api/users.info", map[string]any{"user": "U001"})
	u := resp.JSONMap()["user"].(map[string]any)
	if u["name"] != "alice" {
		t.Errorf("expected name=alice, got %v", u["name"])
	}
}

func TestUsersLookupByEmail(t *testing.T) {
	_, tc := setupSlack(t)

	tc.Post("/admin/state", json.RawMessage(`{"users":{
		"U001":{"id":"U001","name":"alice","profile":{"email":"alice@example.com"}}
	}}`)).AssertStatus(200)

	resp := slackPost(tc, "/api/users.lookupByEmail", map[string]any{"email": "alice@example.com"})
	resp.AssertStatus(200)
	u := resp.JSONMap()["user"].(map[string]any)
	if u["id"] != "U001" {
		t.Errorf("expected id=U001, got %v", u["id"])
	}
}

// --- Reactions Tests ---

func TestReactionsAddAndRemove(t *testing.T) {
	_, tc := setupSlack(t)
	chID := seedChannel(tc, "reactions-test")

	// Post a message
	resp := slackPost(tc, "/api/chat.postMessage", map[string]any{
		"channel": chID,
		"text":    "react to me",
	})
	ts := resp.JSONMap()["ts"].(string)

	// Add reaction
	slackPost(tc, "/api/reactions.add", map[string]any{
		"channel":   chID,
		"timestamp": ts,
		"name":      "thumbsup",
	}).AssertStatus(200)

	// Get reactions
	resp = slackPost(tc, "/api/reactions.get", map[string]any{
		"channel":   chID,
		"timestamp": ts,
	})
	msg := resp.JSONMap()["message"].(map[string]any)
	reactions := msg["reactions"].([]any)
	if len(reactions) != 1 {
		t.Fatalf("expected 1 reaction, got %d", len(reactions))
	}

	// Remove reaction
	slackPost(tc, "/api/reactions.remove", map[string]any{
		"channel":   chID,
		"timestamp": ts,
		"name":      "thumbsup",
	}).AssertStatus(200)

	resp = slackPost(tc, "/api/reactions.get", map[string]any{
		"channel":   chID,
		"timestamp": ts,
	})
	msg = resp.JSONMap()["message"].(map[string]any)
	if msg["reactions"] != nil {
		reactions = msg["reactions"].([]any)
		if len(reactions) != 0 {
			t.Errorf("expected 0 reactions after remove, got %d", len(reactions))
		}
	}
}

func TestReactionsAlreadyReacted(t *testing.T) {
	_, tc := setupSlack(t)
	chID := seedChannel(tc, "double-react")

	resp := slackPost(tc, "/api/chat.postMessage", map[string]any{
		"channel": chID,
		"text":    "hi",
	})
	ts := resp.JSONMap()["ts"].(string)

	slackPost(tc, "/api/reactions.add", map[string]any{
		"channel":   chID,
		"timestamp": ts,
		"name":      "fire",
	}).AssertStatus(200)

	resp = slackPost(tc, "/api/reactions.add", map[string]any{
		"channel":   chID,
		"timestamp": ts,
		"name":      "fire",
	})
	if resp.JSONMap()["error"] != "already_reacted" {
		t.Error("expected already_reacted")
	}
}

// --- Pins Tests ---

func TestPinsAddAndRemove(t *testing.T) {
	_, tc := setupSlack(t)
	chID := seedChannel(tc, "pins-test")

	resp := slackPost(tc, "/api/chat.postMessage", map[string]any{
		"channel": chID,
		"text":    "pin me",
	})
	ts := resp.JSONMap()["ts"].(string)

	slackPost(tc, "/api/pins.add", map[string]any{
		"channel":   chID,
		"timestamp": ts,
	}).AssertStatus(200)

	resp = slackPost(tc, "/api/pins.list", map[string]any{"channel": chID})
	items := resp.JSONMap()["items"].([]any)
	if len(items) != 1 {
		t.Errorf("expected 1 pin, got %d", len(items))
	}

	slackPost(tc, "/api/pins.remove", map[string]any{
		"channel":   chID,
		"timestamp": ts,
	}).AssertStatus(200)

	resp = slackPost(tc, "/api/pins.list", map[string]any{"channel": chID})
	itemsRaw := resp.JSONMap()["items"]
	if itemsRaw != nil {
		items = itemsRaw.([]any)
		if len(items) != 0 {
			t.Errorf("expected 0 pins after remove, got %d", len(items))
		}
	}
}

// --- Files Tests ---

func TestFilesUploadFlow(t *testing.T) {
	_, tc := setupSlack(t)
	chID := seedChannel(tc, "files-test")

	// Get upload URL
	resp := slackPost(tc, "/api/files.getUploadURLExternal", map[string]any{
		"filename": "test.txt",
		"length":   1024,
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	fileID := m["file_id"].(string)
	if m["upload_url"] == nil {
		t.Error("expected upload_url")
	}

	// Complete upload
	resp = slackPost(tc, "/api/files.completeUploadExternal", map[string]any{
		"files":      []map[string]any{{"id": fileID, "title": "Test File"}},
		"channel_id": chID,
	})
	resp.AssertStatus(200)

	// List files
	resp = slackPost(tc, "/api/files.list", nil)
	files := resp.JSONMap()["files"].([]any)
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}

	// Get file info
	resp = slackPost(tc, "/api/files.info", map[string]any{"file": fileID})
	resp.AssertStatus(200)
	file := resp.JSONMap()["file"].(map[string]any)
	if file["title"] != "Test File" {
		t.Errorf("expected title=Test File, got %v", file["title"])
	}

	// Delete file
	slackPost(tc, "/api/files.delete", map[string]any{"file": fileID}).AssertStatus(200)

	resp = slackPost(tc, "/api/files.list", nil)
	files = resp.JSONMap()["files"].([]any)
	if len(files) != 0 {
		t.Errorf("expected 0 files after delete, got %d", len(files))
	}
}

// --- Admin Tests ---

func TestAdminHealth(t *testing.T) {
	_, tc := setupSlack(t)
	tc.Get("/admin/health").AssertStatus(200)
}

func TestAdminReset(t *testing.T) {
	_, tc := setupSlack(t)
	seedChannel(tc, "will-be-reset")

	tc.Post("/admin/reset", nil).AssertStatus(200)

	resp := tc.Get("/admin/channels")
	m := resp.JSONMap()
	if m["total"].(float64) != 0 {
		t.Error("expected 0 channels after reset")
	}
}
