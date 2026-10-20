package api_test

import "testing"

func TestItemGetReturnsTheItemWithPlaidsPresenceRules(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	exchange := func(opts map[string]any) string {
		pub := tc.Post("/sandbox/public_token/create", creds(map[string]any{
			"institution_id": "ins_109508", "initial_products": []string{"auth"}, "options": opts,
		})).JSONMap()["public_token"]
		return tc.Post("/item/public_token/exchange", creds(map[string]any{"public_token": pub})).
			JSONMap()["access_token"].(string)
	}

	bare := tc.Post("/item/get", creds(map[string]any{"access_token": exchange(nil)})).AssertStatus(200).JSONMap()
	it := bare["item"].(map[string]any)
	for _, k := range []string{"webhook", "error", "consent_expiration_time"} {
		if v, ok := it[k]; !ok || v != nil {
			t.Fatalf("required nullable %s must be present as null: %v", k, it)
		}
	}
	for _, k := range []string{"consented_products", "institution_name", "auth_method"} {
		if _, ok := it[k]; ok {
			t.Fatalf("optional %s must be omitted, not sent: %v", k, it)
		}
	}
	if it["update_type"] != "background" || it["institution_id"] != "ins_109508" ||
		len(it["available_products"].([]any)) != 0 || it["billed_products"].([]any)[0] != "auth" {
		t.Fatalf("item = %v", it)
	}
	if _, ok := bare["status"]; ok || bare["request_id"] == nil {
		t.Fatalf("response = %v", bare)
	}

	hooked := tc.Post("/item/get", creds(map[string]any{
		"access_token": exchange(map[string]any{"webhook": "http://hooks.test/plaid"}),
	})).JSONMap()["item"].(map[string]any)
	if hooked["webhook"] != "http://hooks.test/plaid" {
		t.Fatalf("webhook = %v", hooked["webhook"])
	}
}

func TestItemGetRefusesBadAccessTokens(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	for _, tcase := range []struct {
		body map[string]any
		code string
	}{
		{map[string]any{}, "MISSING_FIELDS"},
		{map[string]any{"access_token": 7}, "INVALID_BODY"},
		{map[string]any{"access_token": "access-sandbox-nope"}, "INVALID_ACCESS_TOKEN"},
	} {
		if m := tc.Post("/item/get", creds(tcase.body)).AssertStatus(400).JSONMap(); m["error_code"] != tcase.code {
			t.Fatalf("%v: %v", tcase.body, m)
		}
	}
}
