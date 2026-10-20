package api_test

import (
	"testing"
	"time"
)

func linkBody(extra map[string]any) map[string]any {
	b := creds(map[string]any{
		"client_name": "Acme", "language": "en", "country_codes": []string{"US"},
		"user": map[string]any{"client_user_id": "user-1"},
	})
	for k, v := range extra {
		b[k] = v
	}
	return b
}

func TestLinkTokenCreateThenGetReturnsItsConfiguration(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	created := tc.Post("/link/token/create", linkBody(map[string]any{
		"products": []string{"transactions"}, "webhook": "http://hooks.test/plaid",
		"user": map[string]any{"client_user_id": "u1", "email_address": "a@b.test", "legal_name": "A B"},
	})).AssertStatus(200).JSONMap()
	token := created["link_token"].(string)
	if !regexpLink.MatchString(token) || created["request_id"] == nil {
		t.Fatalf("created = %v", created)
	}

	got := tc.Post("/link/token/get", creds(map[string]any{"link_token": token})).AssertStatus(200).JSONMap()
	made, _ := time.Parse(time.RFC3339, got["created_at"].(string))
	expires, _ := time.Parse(time.RFC3339, got["expiration"].(string))
	if expires.Sub(made) != 4*time.Hour || got["expiration"] != created["expiration"] {
		t.Fatalf("expiration %v is not 4 hours after %v", got["expiration"], got["created_at"])
	}
	md := got["metadata"].(map[string]any)
	if md["client_name"] != "Acme" || md["webhook"] != "http://hooks.test/plaid" ||
		md["initial_products"].([]any)[0] != "transactions" || md["country_codes"].([]any)[0] != "US" {
		t.Fatalf("metadata = %v", md)
	}
	if v, ok := md["redirect_uri"]; !ok || v != nil {
		t.Fatalf("redirect_uri must be present as null: %v", md)
	}
}

func TestLinkTokenCreateChecksItsFields(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	m := tc.Post("/link/token/create", creds(map[string]any{})).AssertStatus(400).JSONMap()
	if m["error_code"] != "MISSING_FIELDS" ||
		m["error_message"] != "the following required fields are missing: client_name, language, country_codes, user" {
		t.Fatalf("body = %v", m)
	}
	for _, bad := range []map[string]any{
		{"client_name": 1}, {"country_codes": "US"}, {"user": "u1"}, {"user": map[string]any{"client_user_id": 1}},
	} {
		if m := tc.Post("/link/token/create", linkBody(bad)).AssertStatus(400).JSONMap(); m["error_code"] != "INVALID_BODY" {
			t.Fatalf("%v: %v", bad, m)
		}
	}
}

func TestLinkTokenUpdateModeLastsThirtyMinutes(t *testing.T) {
	tc, _, mem := setupPlaid(t)
	pub := tc.Post("/sandbox/public_token/create", creds(map[string]any{
		"institution_id": "ins_1", "initial_products": []string{"auth"},
	})).JSONMap()["public_token"]
	access := tc.Post("/item/public_token/exchange", creds(map[string]any{"public_token": pub})).JSONMap()["access_token"]

	update := tc.Post("/link/token/create", linkBody(map[string]any{"access_token": access})).AssertStatus(200).JSONMap()["link_token"]
	normal := tc.Post("/link/token/create", linkBody(nil)).AssertStatus(200).JSONMap()["link_token"]
	if m := tc.Post("/link/token/create", linkBody(map[string]any{"access_token": "access-sandbox-nope"})).
		AssertStatus(400).JSONMap(); m["error_code"] != "INVALID_ACCESS_TOKEN" {
		t.Fatalf("unknown access token = %v", m)
	}

	state := func(token any) int {
		return tc.Post("/link/token/get", creds(map[string]any{"link_token": token})).StatusCode
	}
	mem.Clock.Advance(31 * time.Minute)
	if state(update) != 400 || state(normal) != 200 {
		t.Fatalf("31 minutes in: update %d, normal %d; want 400 and 200", state(update), state(normal))
	}
	mem.Clock.Advance(4 * time.Hour)
	if state(normal) != 400 {
		t.Fatal("a normal link token outlived 4 hours")
	}
}

func TestLinkTokenGetRefusesMissingAndUnknownTokens(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	if m := tc.Post("/link/token/get", creds(map[string]any{})).AssertStatus(400).JSONMap(); m["error_code"] != "MISSING_FIELDS" {
		t.Fatalf("missing = %v", m)
	}
	if m := tc.Post("/link/token/get", creds(map[string]any{"link_token": "link-sandbox-nope"})).AssertStatus(400).JSONMap(); m["error_code"] != "INVALID_LINK_TOKEN" {
		t.Fatalf("unknown = %v", m)
	}
}
