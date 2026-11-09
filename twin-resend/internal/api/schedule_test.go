package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestScheduledAt checks scheduled_at on POST /emails: ISO 8601 and the
// relative natural language form are accepted and read back in the Postgres
// form; past, too distant and unparsable times answer Resend's 422 messages
// (quirk lifecycle-003; probes l01, l11 to l14 of 2026-11-08).
func TestScheduledAt(t *testing.T) {
	_, tc := setupResend(t)
	send := func(at string) map[string]any {
		return baseSend(map[string]any{"scheduled_at": at})
	}

	for _, at := range []string{"in 1 min", "in 5 minutes", "In 1 Hour", "in 2 days", "in 30 days",
		time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)} {
		got := sendAndGet(t, tc, send(at))
		if s, _ := got["scheduled_at"].(string); !pgTime.MatchString(s) || got["last_event"] != "scheduled" {
			t.Errorf("%q: scheduled_at %v, last_event %v", at, got["scheduled_at"], got["last_event"])
		}
	}

	for _, c := range []struct{ at, message string }{
		{"in 31 days", "The `scheduled_at` field must be within 30 days"},
		{time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), "The `scheduled_at` field must be a future date"},
		{"next blue moon", "The `scheduled_at` must be a valid date either in ISO 8601 format or a valid natural language input like \"in 1 min\""},
		{"tomorrow at 9am", "must be a valid date"},
	} {
		m := resendPost(tc, "/emails", send(c.at)).AssertStatus(422).JSONMap()
		if m["name"] != "validation_error" || !strings.Contains(m["message"].(string), c.message) {
			t.Errorf("%q: got %v, want %q", c.at, m, c.message)
		}
	}
}

// TestScheduledEmailSettles checks a scheduled email keeps last_event
// scheduled until the store clock reaches scheduled_at, then takes its
// recipient's outcome, on a read and in the list.
func TestScheduledEmailSettles(t *testing.T) {
	srv, tc := setupResend(t)
	id := resendPost(tc, "/emails", map[string]any{
		"from": "onboarding@resend.dev", "to": "bounced@resend.dev", "subject": "later", "html": "<p>x</p>",
		"scheduled_at": "in 1 hour",
	}).AssertStatus(200).JSONMap()["id"].(string)

	if got := resendGet(tc, "/emails/"+id).JSONMap()["last_event"]; got != "scheduled" {
		t.Fatalf("before the time: last_event %v, want scheduled", got)
	}
	resp, err := http.Post(srv.URL+"/admin/time/advance", "application/json", strings.NewReader(`{"duration":"61m"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := resendGet(tc, "/emails").JSONMap()["data"].([]any)[0].(map[string]any)["last_event"]; got != "bounced" {
		t.Errorf("listed after the time: last_event %v, want bounced", got)
	}
	if got := resendGet(tc, "/emails/"+id).JSONMap()["last_event"]; got != "bounced" {
		t.Errorf("read after the time: last_event %v, want bounced", got)
	}
}

// TestBatchScheduledAtValidated checks a batch item with an invalid
// scheduled_at fails the strict batch with the same message.
func TestBatchScheduledAtValidated(t *testing.T) {
	_, tc := setupResend(t)
	m := resendPost(tc, "/emails/batch", []map[string]any{baseSend(map[string]any{"scheduled_at": "in 31 days"})}).
		AssertStatus(422).JSONMap()
	if m["message"] != "The `scheduled_at` field must be within 30 days" {
		t.Errorf("got %v", m)
	}
}
