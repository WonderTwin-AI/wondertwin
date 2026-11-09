package api_test

import (
	"reflect"
	"testing"
)

// TestDomainEvents checks create, update, verify and delete each deliver a
// domain.* event carrying the domain.
func TestDomainEvents(t *testing.T) {
	_, tc := setupResend(t)
	rc := newReceiver(t)
	createWebhook(t, tc, rc.URL, "domain.created", "domain.updated", "domain.deleted")

	id := resendPost(tc, "/domains", map[string]any{"name": "acme.dev"}).AssertStatus(201).JSONMap()["id"].(string)
	rc.wait(t, 1)
	resendPatch(tc, "/domains/"+id, map[string]any{"click_tracking": true}).AssertStatus(200)
	rc.wait(t, 2)
	resendPost(tc, "/domains/"+id+"/verify", nil).AssertStatus(200)
	rc.wait(t, 3)
	tc.DoWithHeaders("DELETE", "/domains/"+id, nil, resendHeaders).AssertStatus(200)
	got := rc.wait(t, 4)

	want := []string{"domain.created", "domain.updated", "domain.updated", "domain.deleted"}
	if ts := types(got); !reflect.DeepEqual(ts, want) {
		t.Errorf("event types %v, want %v", ts, want)
	}
	for _, d := range got {
		data := d.event["data"].(map[string]any)
		if data["id"] != id || data["name"] != "acme.dev" || len(data["records"].([]any)) != 4 {
			t.Errorf("%s data %v", d.event["type"], data)
		}
	}
	if s := got[2].event["data"].(map[string]any)["status"]; s != "verified" {
		t.Errorf("status after verify %v", s)
	}
}

// TestAdminEmailTriggers checks each admin trigger sets last_event and
// delivers its event, and the refusals.
func TestAdminEmailTriggers(t *testing.T) {
	_, tc := setupResend(t)
	rc := newReceiver(t)
	createWebhook(t, tc, rc.URL, "email.opened", "email.clicked", "email.delivery_delayed", "email.failed")
	id := sendTo(tc, "delivered@resend.dev", nil)

	for i, event := range []string{"opened", "clicked", "delivery_delayed", "failed"} {
		var body any
		if event == "clicked" {
			body = map[string]any{"link": "https://example.com/welcome"}
		}
		m := tc.DoWithHeaders("POST", "/admin/emails/"+id+"/events/"+event, body, nil).AssertStatus(200).JSONMap()
		if m["last_event"] != event {
			t.Errorf("%s: answered %v", event, m)
		}
		if got := resendGet(tc, "/emails/"+id).AssertStatus(200).JSONMap()["last_event"]; got != event {
			t.Errorf("%s: last_event %v", event, got)
		}
		d := rc.wait(t, i+1)[i]
		if d.event["type"] != "email."+event || d.event["data"].(map[string]any)["email_id"] != id {
			t.Errorf("%s: delivered %v", event, d.event)
		}
		if event == "clicked" {
			if link := d.event["data"].(map[string]any)["click"].(map[string]any)["link"]; link != "https://example.com/welcome" {
				t.Errorf("click link %v", link)
			}
		}
	}

	tc.DoWithHeaders("POST", "/admin/emails/"+id+"/events/sent", nil, nil).AssertStatus(400)
	tc.DoWithHeaders("POST", "/admin/emails/00000000-0000-7000-8000-000000000000/events/opened", nil, nil).AssertStatus(404)
	scheduled := sendTo(tc, "delivered@resend.dev", map[string]any{"scheduled_at": "in 1 hour"})
	tc.DoWithHeaders("POST", "/admin/emails/"+scheduled+"/events/opened", nil, nil).AssertStatus(409)
}
