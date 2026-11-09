package api_test

import "testing"

func TestTestAddressOutcomes(t *testing.T) {
	cases := []struct {
		to   any
		want string
	}{
		{"delivered@resend.dev", "delivered"},
		{"bounced@resend.dev", "bounced"},
		{"complained@resend.dev", "complained"},
		{"suppressed@resend.dev", "suppressed"},
		{"bounced+signup@resend.dev", "bounced"},
		{"complained+x@resend.dev", "complained"},
		// suppressed@ takes no label, so a labelled one is an ordinary address.
		{"suppressed+x@resend.dev", "delivered"},
		{"someone@acme.dev", "delivered"},
		{[]string{"someone@acme.dev", "bounced@resend.dev"}, "bounced"},
	}
	for _, c := range cases {
		_, tc := setupResend(t)
		sent := resendPost(tc, "/emails", map[string]any{
			"from": "a@acme.dev", "to": c.to, "subject": "hi", "html": "<p>hi</p>",
		}).AssertStatus(200)
		got := resendGet(tc, "/emails/"+sent.JSONMap()["id"].(string)).AssertStatus(200).JSONMap()
		if got["last_event"] != c.want {
			t.Errorf("to %v: last_event = %v, want %s", c.to, got["last_event"], c.want)
		}
	}
}

func TestTestAddressScheduledStaysScheduled(t *testing.T) {
	_, tc := setupResend(t)
	sent := resendPost(tc, "/emails", map[string]any{
		"from": "a@acme.dev", "to": "bounced@resend.dev", "subject": "hi", "html": "<p>hi</p>",
		"scheduled_at": "in 2 days",
	}).AssertStatus(200)
	got := resendGet(tc, "/emails/"+sent.JSONMap()["id"].(string)).AssertStatus(200).JSONMap()
	if got["last_event"] != "scheduled" {
		t.Errorf("last_event = %v, want scheduled", got["last_event"])
	}
}
