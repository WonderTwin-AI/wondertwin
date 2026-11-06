package api_test

import (
	"testing"
)

func withKey(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func TestAPIKeyCreateAndDeleteShapes(t *testing.T) {
	_, tc := setupResend(t)

	created := resendPost(tc, "/api-keys", map[string]any{"name": "ci"}).AssertStatus(201).JSONMap()
	if created["object"] != "api_key" {
		t.Errorf("create: want object api_key, got %v", created["object"])
	}
	id, _ := created["id"].(string)
	if token, _ := created["token"].(string); len(token) < 4 || token[:3] != "re_" {
		t.Errorf("create: want a re_ token, got %v", created["token"])
	}

	list := resendGet(tc, "/api-keys").AssertStatus(200).JSONMap()
	data := list["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("list: want 1 key, got %d", len(data))
	}
	if _, ok := data[0].(map[string]any)["token"]; ok {
		t.Error("list: token must never be listed")
	}

	del := tc.DoWithHeaders("DELETE", "/api-keys/"+id, nil, resendHeaders).AssertStatus(200).JSONMap()
	if del["object"] != "api_key" || del["id"] != id || del["deleted"] != true {
		t.Errorf("delete: want {object: api_key, id, deleted: true}, got %v", del)
	}
	tc.DoWithHeaders("DELETE", "/api-keys/"+id, nil, resendHeaders).AssertStatus(404)
}

func TestSendingAccessKeyOnlySends(t *testing.T) {
	_, tc := setupResend(t)
	token := resendPost(tc, "/api-keys", map[string]any{"name": "send", "permission": "sending_access"}).
		AssertStatus(201).JSONMap()["token"].(string)

	tc.DoWithHeaders("GET", "/domains", nil, withKey(token)).
		AssertStatus(401).AssertBodyContains(`"name":"restricted_api_key"`).
		AssertBodyContains("This API key is restricted to only send emails")
	tc.DoWithHeaders("POST", "/emails", map[string]any{
		"from": "onboarding@resend.dev", "to": "delivered@resend.dev", "subject": "hi", "html": "<p>hi</p>",
	}, withKey(token)).AssertStatus(200)
	tc.DoWithHeaders("POST", "/emails/batch", []map[string]any{{
		"from": "onboarding@resend.dev", "to": "delivered@resend.dev", "subject": "hi", "html": "<p>hi</p>",
	}}, withKey(token)).AssertStatus(200)

	keys := resendGet(tc, "/api-keys").JSONMap()["data"].([]any)
	if keys[0].(map[string]any)["last_used_at"] == nil {
		t.Error("last_used_at: want set after the key was used")
	}
}

func TestFullAccessKeyAndForeignTokens(t *testing.T) {
	_, tc := setupResend(t)
	token := resendPost(tc, "/api-keys", map[string]any{"name": "full"}).AssertStatus(201).JSONMap()["token"].(string)
	tc.DoWithHeaders("GET", "/domains", nil, withKey(token)).AssertStatus(200)
	// A token the emulator never issued is still accepted (OD-3).
	tc.DoWithHeaders("GET", "/domains", nil, withKey("re_not_issued")).AssertStatus(200)
}

// TestUserAgentRequired checks a keyed request with no User-Agent answers
// Resend's edge 403, while a request with no key still answers 401.
func TestUserAgentRequired(t *testing.T) {
	_, tc := setupResend(t)
	tc.DoWithHeaders("GET", "/domains", nil, map[string]string{
		"Authorization": "Bearer re_sim_test_123", "User-Agent": "",
	}).AssertStatus(403).AssertBodyContains("error code: 1010")
	tc.DoWithHeaders("GET", "/domains", nil, map[string]string{"User-Agent": ""}).
		AssertStatus(401).AssertBodyContains("missing_api_key")
	resendGet(tc, "/domains").AssertStatus(200)
}
