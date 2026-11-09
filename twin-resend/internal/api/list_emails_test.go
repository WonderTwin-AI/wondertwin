package api_test

import (
	"fmt"
	"testing"
)

// TestListEmails checks GET /emails: newest first, the listed shape without
// object, html, text and tags, bcc and cc null, and the default page of 20
// (quirks pagination-001, pagination-002).
func TestListEmails(t *testing.T) {
	_, tc := setupResend(t)
	var ids []string
	for i := range 22 {
		m := resendPost(tc, "/emails", map[string]any{
			"from": "onboarding@resend.dev", "to": "delivered@resend.dev", "cc": "delivered+cc@resend.dev",
			"subject": fmt.Sprintf("list %d", i), "html": "<p>x</p>", "tags": []map[string]string{{"name": "k", "value": "v"}},
		}).AssertStatus(200).JSONMap()
		ids = append(ids, m["id"].(string))
	}
	newest := make([]string, len(ids))
	for i, id := range ids {
		newest[len(ids)-1-i] = id
	}

	page := resendGet(tc, "/emails").AssertStatus(200).JSONMap()
	assertPage(t, page, newest[:20], true)
	item := page["data"].([]any)[0].(map[string]any)
	for _, k := range []string{"object", "html", "text", "tags"} {
		if _, ok := item[k]; ok {
			t.Errorf("listed email carries %q", k)
		}
	}
	for _, k := range []string{"id", "to", "from", "created_at", "subject", "bcc", "cc", "reply_to", "last_event", "scheduled_at", "message_id"} {
		if _, ok := item[k]; !ok {
			t.Errorf("listed email lacks %q", k)
		}
	}
	if item["cc"] != nil || item["bcc"] != nil {
		t.Errorf("cc and bcc should list as null, got %v and %v", item["cc"], item["bcc"])
	}

	assertPage(t, resendGet(tc, "/emails?after="+newest[19]).AssertStatus(200).JSONMap(), newest[20:], false)
	assertPage(t, resendGet(tc, "/emails?limit=2&after="+newest[0]).AssertStatus(200).JSONMap(), newest[1:3], true)
	assertPage(t, resendGet(tc, "/emails?limit=1&before="+newest[1]).AssertStatus(200).JSONMap(), newest[:1], false)
	resendGet(tc, "/emails?limit=101").AssertStatus(422)
}

// TestListEmailsEmpty checks a fresh emulator lists data [] for emails.
func TestListEmailsEmpty(t *testing.T) {
	_, tc := setupResend(t)
	assertPage(t, resendGet(tc, "/emails").AssertStatus(200).JSONMap(), nil, false)
}
