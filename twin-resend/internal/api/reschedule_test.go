package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

func resendPatch(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("PATCH", path, body, resendHeaders)
}

func scheduleOne(t *testing.T, tc *testutil.TwinClient) string {
	t.Helper()
	return resendPost(tc, "/emails", baseSend(map[string]any{"scheduled_at": "in 1 hour"})).
		AssertStatus(200).JSONMap()["id"].(string)
}

func assertEmailRef(t *testing.T, m map[string]any, id string) {
	t.Helper()
	if m["object"] != "email" || m["id"] != id || len(m) != 2 {
		t.Errorf("got %v, want {object: email, id: %s}", m, id)
	}
}

func assertResendError(t *testing.T, m map[string]any, name, message string) {
	t.Helper()
	if m["name"] != name || m["message"] != message {
		t.Errorf("got %v, want %s %q", m, name, message)
	}
}

// TestUpdateEmail checks PATCH /emails/{id} moves a scheduled email, in ISO
// 8601 or natural language, and answers {object, id} (quirk envelope-009;
// probes l03 and l05 of 2026-11-08).
func TestUpdateEmail(t *testing.T) {
	_, tc := setupResend(t)
	id := scheduleOne(t, tc)
	before := resendGet(tc, "/emails/"+id).JSONMap()["scheduled_at"]

	at := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)
	assertEmailRef(t, resendPatch(tc, "/emails/"+id, map[string]any{"scheduled_at": at.Format(time.RFC3339)}).AssertStatus(200).JSONMap(), id)
	got := resendGet(tc, "/emails/"+id).JSONMap()
	if got["scheduled_at"] != at.Format("2006-01-02 15:04:05.000000+00") || got["last_event"] != "scheduled" {
		t.Errorf("after ISO reschedule: scheduled_at %v (was %v), last_event %v", got["scheduled_at"], before, got["last_event"])
	}

	assertEmailRef(t, resendPatch(tc, "/emails/"+id, map[string]any{"scheduled_at": "in 2 days"}).AssertStatus(200).JSONMap(), id)
	if got := resendGet(tc, "/emails/"+id).JSONMap()["scheduled_at"]; got == at.Format("2006-01-02 15:04:05.000000+00") {
		t.Error("natural-language reschedule did not move scheduled_at")
	}

	assertResendError(t, resendPatch(tc, "/emails/"+id, map[string]any{"scheduled_at": "in 31 days"}).AssertStatus(422).JSONMap(),
		"validation_error", "The `scheduled_at` field must be within 30 days")
}

// TestCancelEmail checks POST /emails/{id}/cancel: the email reads canceled,
// a second cancel and a reschedule answer Resend's 422s (probes l07 to l10 of
// 2026-11-08).
func TestCancelEmail(t *testing.T) {
	_, tc := setupResend(t)
	id := scheduleOne(t, tc)

	assertEmailRef(t, resendPost(tc, "/emails/"+id+"/cancel", nil).AssertStatus(200).JSONMap(), id)
	if got := resendGet(tc, "/emails/"+id).JSONMap()["last_event"]; got != "canceled" {
		t.Errorf("last_event %v, want canceled", got)
	}
	assertResendError(t, resendPost(tc, "/emails/"+id+"/cancel", nil).AssertStatus(422).JSONMap(),
		"validation_error", "Email is not scheduled")
	assertResendError(t, resendPatch(tc, "/emails/"+id, map[string]any{"scheduled_at": "in 1 hour"}).AssertStatus(422).JSONMap(),
		"validation_error", "You can not edit an email that is not scheduled")
}

// TestCancelAndUpdateUnscheduled checks an email that was never scheduled,
// or whose time has passed, cannot be cancelled or rescheduled, and unknown
// or malformed ids answer as GET does.
func TestCancelAndUpdateUnscheduled(t *testing.T) {
	srv, tc := setupResend(t)
	sent := resendPost(tc, "/emails", baseSend(nil)).AssertStatus(200).JSONMap()["id"].(string)
	assertResendError(t, resendPost(tc, "/emails/"+sent+"/cancel", nil).AssertStatus(422).JSONMap(),
		"validation_error", "Email is not scheduled")

	due := scheduleOne(t, tc)
	resp, err := http.Post(srv.URL+"/admin/time/advance", "application/json", strings.NewReader(`{"duration":"2h"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	assertResendError(t, resendPost(tc, "/emails/"+due+"/cancel", nil).AssertStatus(422).JSONMap(),
		"validation_error", "Email is not scheduled")

	unknown := "01a1a9bc-0000-7000-8000-000000000000"
	assertResendError(t, resendPatch(tc, "/emails/"+unknown, map[string]any{"scheduled_at": "in 1 hour"}).AssertStatus(404).JSONMap(),
		"not_found", "Email not found")
	assertResendError(t, resendPost(tc, "/emails/"+unknown+"/cancel", nil).AssertStatus(404).JSONMap(),
		"not_found", "Email not found")
	resendPost(tc, "/emails/not-a-uuid/cancel", nil).AssertStatus(422)
}
