package api_test

import "testing"

func TestAudiencesAndContacts(t *testing.T) {
	_, tc := setupResend(t)

	aud := resendPost(tc, "/audiences", map[string]any{"name": "General"}).AssertStatus(201).JSONMap()
	audienceID := aud["id"].(string)
	if aud["object"] != "audience" || aud["name"] != "General" {
		t.Errorf("create audience: %v", aud)
	}
	resendGet(tc, "/audiences/"+audienceID).AssertStatus(200).AssertBodyContains(`"name":"General"`)
	if n := len(resendGet(tc, "/audiences").JSONMap()["data"].([]any)); n != 1 {
		t.Errorf("list audiences: want 1, got %d", n)
	}

	contacts := "/audiences/" + audienceID + "/contacts"
	c := resendPost(tc, contacts, map[string]any{"email": "c@acme.dev", "first_name": "Ada"}).AssertStatus(201).JSONMap()
	contactID := c["id"].(string)
	if c["object"] != "contact" || c["unsubscribed"] != false {
		t.Errorf("create contact: %v", c)
	}
	upd := tc.DoWithHeaders("PATCH", contacts+"/"+contactID, map[string]any{"unsubscribed": true}, resendHeaders).
		AssertStatus(200).JSONMap()
	if upd["unsubscribed"] != true || upd["first_name"] != "Ada" {
		t.Errorf("patch contact: %v", upd)
	}
	resendGet(tc, contacts+"/"+contactID).AssertStatus(200).AssertBodyContains(`"unsubscribed":true`)
	if n := len(resendGet(tc, contacts).JSONMap()["data"].([]any)); n != 1 {
		t.Errorf("list contacts: want 1, got %d", n)
	}
	tc.DoWithHeaders("DELETE", contacts+"/"+contactID, nil, resendHeaders).
		AssertStatus(200).AssertBodyContains(`"deleted":true`)
	resendGet(tc, contacts+"/"+contactID).AssertStatus(404)

	// Deleting an audience deletes its contacts.
	other := resendPost(tc, contacts, map[string]any{"email": "d@acme.dev"}).AssertStatus(201).JSONMap()["id"].(string)
	tc.DoWithHeaders("DELETE", "/audiences/"+audienceID, nil, resendHeaders).
		AssertStatus(200).AssertBodyContains(`"object":"audience"`)
	resendGet(tc, "/audiences/"+audienceID).AssertStatus(404)
	resendGet(tc, contacts+"/"+other).AssertStatus(404)
	resendGet(tc, contacts).AssertStatus(404)
	resendPost(tc, contacts, map[string]any{"email": "e@acme.dev"}).AssertStatus(404)
}

func TestBroadcasts(t *testing.T) {
	_, tc := setupResend(t)
	audienceID := resendPost(tc, "/audiences", map[string]any{"name": "General"}).AssertStatus(201).JSONMap()["id"].(string)

	resendPost(tc, "/broadcasts", map[string]any{
		"audience_id": "missing", "from": "a@acme.dev", "subject": "s",
	}).AssertStatus(404)

	bc := resendPost(tc, "/broadcasts", map[string]any{
		"name": "Launch", "audience_id": audienceID, "from": "a@acme.dev", "subject": "news", "html": "<p>hi</p>",
	}).AssertStatus(201).JSONMap()
	id := bc["id"].(string)
	if bc["object"] != "broadcast" || bc["status"] != "draft" {
		t.Errorf("create broadcast: %v", bc)
	}
	resendGet(tc, "/broadcasts/"+id).AssertStatus(200).AssertBodyContains(`"subject":"news"`)

	sent := resendPost(tc, "/broadcasts/"+id+"/send", nil).AssertStatus(200).JSONMap()
	if sent["status"] != "sent" || sent["sent_at"] == "" {
		t.Errorf("send broadcast: %v", sent)
	}
	resendPost(tc, "/broadcasts/"+id+"/send", nil).AssertStatus(400)

	tc.DoWithHeaders("DELETE", "/broadcasts/"+id, nil, resendHeaders).
		AssertStatus(200).AssertBodyContains(`"deleted":true`)
	resendGet(tc, "/broadcasts/"+id).AssertStatus(404)
}

func TestOlderRoutesNeedAKey(t *testing.T) {
	_, tc := setupResend(t)
	for _, path := range []string{"/domains", "/api-keys", "/audiences", "/broadcasts"} {
		tc.Get(path).AssertStatus(401).AssertBodyContains("missing_api_key")
	}
}
