package api_test

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

var whsec = regexp.MustCompile(`^whsec_[A-Za-z0-9+/]{32}$`)

func createWebhook(t *testing.T, tc *testutil.TwinClient, endpoint string, events ...string) map[string]any {
	t.Helper()
	return resendPost(tc, "/webhooks", map[string]any{"endpoint": endpoint, "events": events}).AssertStatus(201).JSONMap()
}

// TestCreateWebhook checks POST /webhooks answers 201 {object, id,
// signing_secret} (probe o01 of 2026-11-08).
func TestCreateWebhook(t *testing.T) {
	_, tc := setupResend(t)
	m := createWebhook(t, tc, "https://example.com/hook", "email.sent", "email.delivered")
	if got := sortedKeys(m); !reflect.DeepEqual(got, []string{"id", "object", "signing_secret"}) {
		t.Errorf("create keys %v", got)
	}
	if m["object"] != "webhook" || !whsec.MatchString(m["signing_secret"].(string)) {
		t.Errorf("create got %v", m)
	}
	// Local receivers run on http; the emulator accepts them (divergences.json).
	createWebhook(t, tc, "http://localhost:9000/hook", "email.sent")
}

// TestCreateWebhookErrors checks the recorded refusals (probes o07 to o09
// of 2026-11-08).
func TestCreateWebhookErrors(t *testing.T) {
	_, tc := setupResend(t)

	m := resendPost(tc, "/webhooks", map[string]any{"endpoint": "https://example.com/hook"}).AssertStatus(422).JSONMap()
	assertResendError(t, m, "missing_required_field", "Missing `events` field.")
	m = resendPost(tc, "/webhooks", map[string]any{"events": []string{"email.sent"}}).AssertStatus(422).JSONMap()
	assertResendError(t, m, "missing_required_field", "Missing `endpoint` field.")

	m = resendPost(tc, "/webhooks", map[string]any{"endpoint": "https://example.com/hook", "events": []string{"email.nonsense"}}).
		AssertStatus(422).JSONMap()
	msg, _ := m["message"].(string)
	if m["name"] != "validation_error" || !strings.HasPrefix(msg, "`email.nonsense` is not a webhook event type. Use one of email.bounced, email.clicked,") ||
		!strings.HasSuffix(msg, "inbox.draft.sent, inbox.draft.deleted.") || strings.Count(msg, ", ") != 36 {
		t.Errorf("bad event: got %v", m)
	}

	m = resendPost(tc, "/webhooks", map[string]any{"endpoint": "ftp://example.com/hook", "events": []string{"email.sent"}}).AssertStatus(422).JSONMap()
	assertResendError(t, m, "validation_error", "Endpoint URL scheme must be https")
}

// TestGetWebhook checks get returns the secret and an unknown id answers
// 404 (probes o03, o12 of 2026-11-08).
func TestGetWebhook(t *testing.T) {
	_, tc := setupResend(t)
	created := createWebhook(t, tc, "https://example.com/hook", "email.bounced")
	id := created["id"].(string)

	got := resendGet(tc, "/webhooks/"+id).AssertStatus(200).JSONMap()
	want := []string{"created_at", "endpoint", "events", "id", "object", "signing_secret", "status"}
	if keys := sortedKeys(got); !reflect.DeepEqual(keys, want) {
		t.Errorf("get keys %v, want %v", keys, want)
	}
	if got["status"] != "enabled" || got["signing_secret"] != created["signing_secret"] ||
		!reflect.DeepEqual(got["events"], []any{"email.bounced"}) || got["endpoint"] != "https://example.com/hook" {
		t.Errorf("get %v", got)
	}
	if !pgTime.MatchString(got["created_at"].(string)) {
		t.Errorf("created_at %v, want the Postgres form", got["created_at"])
	}

	m := resendGet(tc, "/webhooks/00000000-0000-4000-8000-000000000000").AssertStatus(404).JSONMap()
	assertResendError(t, m, "not_found", "Webhook endpoint not found")
}

// TestListWebhooks checks list items carry no object or secret, newest
// first, with Resend's pagination errors (probes o04, o10 of 2026-11-08).
func TestListWebhooks(t *testing.T) {
	_, tc := setupResend(t)
	if !strings.Contains(string(resendGet(tc, "/webhooks").AssertStatus(200).Body), `"data":[]`) {
		t.Error("empty list: want data []")
	}
	first := createWebhook(t, tc, "https://example.com/one", "email.sent")["id"].(string)
	second := createWebhook(t, tc, "https://example.com/two", "email.sent")["id"].(string)

	m := resendGet(tc, "/webhooks").AssertStatus(200).JSONMap()
	assertPage(t, m, []string{second, first}, false)
	item := m["data"].([]any)[0].(map[string]any)
	if keys := sortedKeys(item); !reflect.DeepEqual(keys, []string{"created_at", "endpoint", "events", "id", "status"}) {
		t.Errorf("list item keys %v", keys)
	}
	assertPage(t, resendGet(tc, "/webhooks?limit=1").AssertStatus(200).JSONMap(), []string{second}, true)
	m = resendGet(tc, "/webhooks?limit=0").AssertStatus(422).JSONMap()
	assertResendError(t, m, "validation_error", "The pagination limit must be a number between 1 and 100. See https://resend.com/docs/api-reference/pagination for more information.")
}

// TestWebhookState checks webhooks round-trip /admin/state, reset clears
// them, and a seed repeats the signing secret.
func TestWebhookState(t *testing.T) {
	tc, s := setupResendWithStore(t)
	s.Seed(11)
	adm := testutil.NewAdminClient(tc)
	created := createWebhook(t, tc, "https://example.com/hook", "email.sent")
	id := created["id"].(string)
	before := resendGet(tc, "/webhooks/"+id).AssertStatus(200).JSONMap()

	snap := adm.GetState().AssertStatus(200).Body
	adm.Reset().AssertStatus(200)
	resendGet(tc, "/webhooks/"+id).AssertStatus(404)
	adm.LoadState(json.RawMessage(snap)).AssertStatus(200)
	if after := resendGet(tc, "/webhooks/"+id).AssertStatus(200).JSONMap(); !reflect.DeepEqual(after, before) {
		t.Errorf("after load %v, want %v", after, before)
	}

	adm.Reset().AssertStatus(200)
	if again := createWebhook(t, tc, "https://example.com/hook", "email.sent"); again["signing_secret"] != created["signing_secret"] {
		t.Error("the signing secret did not repeat for the same seed after reset")
	}
}
