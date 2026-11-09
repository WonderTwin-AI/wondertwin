package api_test

import (
	"reflect"
	"testing"
)

// TestUpdateWebhook checks PATCH answers {object, id} and changes only the
// fields it names (probes o02, o05, o06 of 2026-11-08).
func TestUpdateWebhook(t *testing.T) {
	_, tc := setupResend(t)
	created := createWebhook(t, tc, "https://example.com/hook", "email.sent", "email.delivered")
	id := created["id"].(string)

	m := resendPatch(tc, "/webhooks/"+id, map[string]any{"status": "disabled"}).AssertStatus(200).JSONMap()
	if !reflect.DeepEqual(m, map[string]any{"object": "webhook", "id": id}) {
		t.Errorf("PATCH answered %v", m)
	}
	got := resendGet(tc, "/webhooks/"+id).AssertStatus(200).JSONMap()
	if got["status"] != "disabled" || !reflect.DeepEqual(got["events"], []any{"email.sent", "email.delivered"}) {
		t.Errorf("after disable: %v", got)
	}

	resendPatch(tc, "/webhooks/"+id, map[string]any{"endpoint": "https://example.com/hook-2", "events": []string{"email.bounced"}}).AssertStatus(200)
	got = resendGet(tc, "/webhooks/"+id).AssertStatus(200).JSONMap()
	if got["status"] != "disabled" || got["endpoint"] != "https://example.com/hook-2" ||
		!reflect.DeepEqual(got["events"], []any{"email.bounced"}) || got["signing_secret"] != created["signing_secret"] {
		t.Errorf("after endpoint and events: %v", got)
	}

	for _, body := range []map[string]any{
		{"events": []string{"email.nonsense"}},
		{"endpoint": "ftp://example.com/hook"},
		{"status": "paused"},
	} {
		resendPatch(tc, "/webhooks/"+id, body).AssertStatus(422)
	}
	m = resendPatch(tc, "/webhooks/00000000-0000-4000-8000-000000000000", map[string]any{"status": "enabled"}).AssertStatus(404).JSONMap()
	assertResendError(t, m, "not_found", "Webhook endpoint not found")
}

// TestDeleteWebhook checks DELETE answers {object, id, deleted: true} and
// the webhook then reads 404 (probes o11, o12 of 2026-11-08).
func TestDeleteWebhook(t *testing.T) {
	_, tc := setupResend(t)
	id := createWebhook(t, tc, "https://example.com/hook", "email.sent")["id"].(string)

	m := tc.DoWithHeaders("DELETE", "/webhooks/"+id, nil, resendHeaders).AssertStatus(200).JSONMap()
	if !reflect.DeepEqual(m, map[string]any{"object": "webhook", "id": id, "deleted": true}) {
		t.Errorf("DELETE answered %v", m)
	}
	m = resendGet(tc, "/webhooks/"+id).AssertStatus(404).JSONMap()
	assertResendError(t, m, "not_found", "Webhook endpoint not found")
	tc.DoWithHeaders("DELETE", "/webhooks/"+id, nil, resendHeaders).AssertStatus(404)
}
