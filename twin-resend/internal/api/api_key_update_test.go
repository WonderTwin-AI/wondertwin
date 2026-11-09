package api_test

import (
	"reflect"
	"testing"
)

// TestUpdateAPIKey checks PATCH /api-keys/{id} renames the key and answers
// {object, id}, and an unknown key answers 404 (probes n02, n03, n08 of
// 2026-11-08).
func TestUpdateAPIKey(t *testing.T) {
	_, tc := setupResend(t)
	id := resendPost(tc, "/api-keys", map[string]any{"name": "ci"}).AssertStatus(201).JSONMap()["id"].(string)

	m := resendPatch(tc, "/api-keys/"+id, map[string]any{"name": "ci-renamed"}).AssertStatus(200).JSONMap()
	if !reflect.DeepEqual(m, map[string]any{"object": "api_key", "id": id}) {
		t.Errorf("PATCH answered %v", m)
	}
	item := resendGet(tc, "/api-keys").AssertStatus(200).JSONMap()["data"].([]any)[0].(map[string]any)
	if item["name"] != "ci-renamed" {
		t.Errorf("listed name %v, want ci-renamed", item["name"])
	}

	m = resendPatch(tc, "/api-keys/00000000-0000-4000-8000-000000000000", map[string]any{"name": "x"}).AssertStatus(404).JSONMap()
	assertResendError(t, m, "not_found", "API key not found")
	m = tc.DoWithHeaders("DELETE", "/api-keys/00000000-0000-4000-8000-000000000000", nil, resendHeaders).AssertStatus(404).JSONMap()
	assertResendError(t, m, "not_found", "API key not found")
}

// TestDomainRestrictedKey checks a sending key bound to an unverified domain
// is refused 400 with no error name when it sends from that domain, and
// still sends from onboarding@resend.dev (quirk auth-007).
func TestDomainRestrictedKey(t *testing.T) {
	_, tc := setupResend(t)
	domainID := resendPost(tc, "/domains", map[string]any{"name": "acme.dev"}).AssertStatus(201).JSONMap()["id"].(string)
	token := resendPost(tc, "/api-keys", map[string]any{
		"name": "bound", "permission": "sending_access", "domain_id": domainID,
	}).AssertStatus(201).JSONMap()["token"].(string)
	send := func(from string) map[string]any {
		return map[string]any{"from": from, "to": "delivered@resend.dev", "subject": "s", "text": "t"}
	}

	m := tc.DoWithHeaders("POST", "/emails", send("a@acme.dev"), withKey(token)).AssertStatus(400).JSONMap()
	want := map[string]any{
		"message":    "The associated domain with your API key is not verified. Please, create a new API key with full access or with a verified domain.",
		"statusCode": float64(400),
	}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("own unverified domain: got %v, want %v", m, want)
	}
	tc.DoWithHeaders("POST", "/emails/batch", []any{send("onboarding@resend.dev"), send("a@acme.dev")}, withKey(token)).
		AssertStatus(400)

	tc.DoWithHeaders("POST", "/emails", send("onboarding@resend.dev"), withKey(token)).AssertStatus(200)

	// A key the emulator did not issue is not bound to the domain: the
	// ordinary sender-domain rule answers 403.
	resendPost(tc, "/emails", send("a@acme.dev")).AssertStatus(403)

	resendPost(tc, "/domains/"+domainID+"/verify", nil).AssertStatus(200)
	tc.DoWithHeaders("POST", "/emails", send("a@acme.dev"), withKey(token)).AssertStatus(200)
}
