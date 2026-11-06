package api_test

import (
	"regexp"
	"testing"
)

var (
	pgTime    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{6}\+00$`)
	sesMsgID  = regexp.MustCompile(`^<[0-9a-f]{16}-[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}-000000@email\.amazonses\.com>$`)
	readShape = []string{"object", "id", "to", "from", "created_at", "subject", "bcc", "cc", "reply_to",
		"last_event", "scheduled_at", "message_id", "html", "text"}
)

func TestGetEmailShape(t *testing.T) {
	_, tc := setupResend(t)

	t.Run("fields as Resend returns them", func(t *testing.T) {
		got := sendAndGet(t, tc, baseSend(map[string]any{"cc": "delivered+cc@resend.dev", "bcc": "delivered+bcc@resend.dev"}))
		for _, k := range readShape {
			if _, ok := got[k]; !ok {
				t.Errorf("missing %s", k)
			}
		}
		if len(got) != len(readShape) {
			t.Errorf("got %d fields, want %d: %v", len(got), len(readShape), got)
		}
		if got["cc"] != nil || got["bcc"] != nil {
			t.Errorf("cc %v, bcc %v; Resend returns null", got["cc"], got["bcc"])
		}
		if got["reply_to"] != nil || got["scheduled_at"] != nil {
			t.Errorf("reply_to %v, scheduled_at %v; want null when absent", got["reply_to"], got["scheduled_at"])
		}
		if s, _ := got["created_at"].(string); !pgTime.MatchString(s) {
			t.Errorf("created_at %q is not the Postgres form", s)
		}
		if s, _ := got["message_id"].(string); !sesMsgID.MatchString(s) {
			t.Errorf("message_id %q is not the SES form", s)
		}
	})

	t.Run("html null when only text was sent", func(t *testing.T) {
		body := baseSend(map[string]any{"text": "plain"})
		delete(body, "html")
		if got := sendAndGet(t, tc, body); got["html"] != nil || got["text"] != "plain" {
			t.Errorf("html %v, text %v", got["html"], got["text"])
		}
	})

	t.Run("tags only when sent", func(t *testing.T) {
		got := sendAndGet(t, tc, baseSend(map[string]any{"tags": []map[string]string{{"name": "a", "value": "b"}}}))
		if _, ok := got["tags"]; !ok {
			t.Error("tags missing after a send with tags")
		}
	})

	t.Run("malformed id answers 422", func(t *testing.T) {
		resp := resendGet(tc, "/emails/not-a-uuid")
		resp.AssertStatus(422)
		if m := resp.JSONMap(); m["message"] != "The `id` must be a valid UUID." || m["name"] != "validation_error" {
			t.Errorf("got %v", m)
		}
	})
}
