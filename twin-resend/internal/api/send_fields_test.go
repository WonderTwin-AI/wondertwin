package api_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

func sendAndGet(t *testing.T, tc *testutil.TwinClient, body map[string]any) map[string]any {
	t.Helper()
	resp := resendPost(tc, "/emails", body)
	resp.AssertStatus(200)
	id, _ := resp.JSONMap()["id"].(string)
	got := resendGet(tc, "/emails/"+id)
	got.AssertStatus(200)
	return got.JSONMap()
}

func baseSend(over map[string]any) map[string]any {
	b := map[string]any{"from": "onboarding@resend.dev", "to": []string{"delivered@resend.dev"}, "subject": "s", "html": "<p>Hello</p>"}
	for k, v := range over {
		b[k] = v
	}
	return b
}

func TestSendEmailStringRecipients(t *testing.T) {
	_, tc := setupResend(t)
	got := sendAndGet(t, tc, baseSend(map[string]any{
		"to": "delivered@resend.dev", "cc": "delivered+cc@resend.dev",
		"bcc": "delivered+bcc@resend.dev", "reply_to": "delivered+reply@resend.dev",
	}))
	// A read returns cc and bcc as null (envelope-004); the stored values are
	// on the admin plane.
	stored := tc.Get("/admin/emails").JSONMap()["emails"].([]any)[0].(map[string]any)
	for field, want := range map[string]string{
		"to": "delivered@resend.dev", "cc": "delivered+cc@resend.dev",
		"bcc": "delivered+bcc@resend.dev", "reply_to": "delivered+reply@resend.dev",
	} {
		if !reflect.DeepEqual(stored[field], []any{want}) {
			t.Errorf("%s stored as %v, want [%s]", field, stored[field], want)
		}
	}
	if !reflect.DeepEqual(got["to"], []any{"delivered@resend.dev"}) || !reflect.DeepEqual(got["reply_to"], []any{"delivered+reply@resend.dev"}) {
		t.Errorf("read returned to %v, reply_to %v", got["to"], got["reply_to"])
	}
}

func TestSendEmailKeepsFields(t *testing.T) {
	_, tc := setupResend(t)
	got := sendAndGet(t, tc, baseSend(map[string]any{
		"tags":         []map[string]string{{"name": "category", "value": "welcome"}},
		"headers":      map[string]string{"X-Entity-Ref-ID": "123"},
		"attachments":  []map[string]string{{"filename": "a.txt", "content": "aGVsbG8="}},
		"scheduled_at": "2026-11-06T09:00:00Z",
	}))
	if !reflect.DeepEqual(got["tags"], []any{map[string]any{"name": "category", "value": "welcome"}}) {
		t.Errorf("tags %v", got["tags"])
	}
	if got["scheduled_at"] != "2026-11-06T09:00:00Z" || got["last_event"] != "scheduled" {
		t.Errorf("scheduled_at %v, last_event %v; want the time and scheduled", got["scheduled_at"], got["last_event"])
	}
	for _, f := range []string{"headers", "attachments"} {
		if _, ok := got[f]; ok {
			t.Errorf("%s returned on a read; Resend does not return it", f)
		}
	}
}

func TestSendEmailTextFromHTML(t *testing.T) {
	_, tc := setupResend(t)
	cases := []struct {
		name string
		over map[string]any
		want string
	}{
		{"omitted text is generated", map[string]any{"html": "<p>Hello &amp; welcome</p><p>Line <b>two</b></p>"}, "Hello & welcome\nLine two"},
		{"sent text is kept", map[string]any{"text": "Plain"}, "Plain"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sendAndGet(t, tc, baseSend(c.over))["text"]; got != c.want {
				t.Errorf("text %q, want %q", got, c.want)
			}
		})
	}
	t.Run("empty text opts out", func(t *testing.T) {
		if got, _ := sendAndGet(t, tc, baseSend(map[string]any{"text": ""}))["text"].(string); got != "" {
			t.Errorf("text %q, want none", got)
		}
	})
}

func TestSendEmailValidation(t *testing.T) {
	_, tc := setupResend(t)
	many := make([]string, 51)
	for i := range many {
		many[i] = "delivered@resend.dev"
	}
	cases := []struct {
		name    string
		body    map[string]any
		message string
	}{
		{"missing from", map[string]any{"to": "delivered@resend.dev", "subject": "s"}, "Missing `from` field."},
		{"missing to", map[string]any{"from": "a@example.com", "subject": "s"}, "Missing `to` field."},
		{"missing subject", map[string]any{"from": "a@example.com", "to": "delivered@resend.dev"}, "Missing `subject` field."},
		{"over 50 recipients", baseSend(map[string]any{"to": many}), "at most 50"},
		{"scheduled_at not ISO 8601", baseSend(map[string]any{"scheduled_at": "in 1 min"}), "scheduled_at"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := resendPost(tc, "/emails", c.body)
			resp.AssertStatus(422)
			m := resp.JSONMap()
			if m["name"] != "validation_error" || !strings.Contains(m["message"].(string), c.message) {
				t.Errorf("got %v, want validation_error containing %q", m, c.message)
			}
		})
	}
}
