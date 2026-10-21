package api_test

import "testing"

func createWithOptions(opts map[string]any) map[string]any {
	return creds(map[string]any{"institution_id": "ins_109508", "initial_products": []string{"auth"}, "options": opts})
}

func TestSandboxErrorPasswordFailsTheCreateWithTheDocumentedError(t *testing.T) {
	tc, _, mem := setupPlaid(t)
	for _, tcase := range []struct {
		code, errType string
		status        int
	}{
		{"ITEM_LOCKED", "ITEM_ERROR", 400},
		{"INSTITUTION_NOT_RESPONDING", "INSTITUTION_ERROR", 400},
		{"PAYMENT_REJECTED", "PAYMENT_ERROR", 400},
		{"INTERNAL_SERVER_ERROR", "API_ERROR", 500},
	} {
		m := tc.Post("/sandbox/public_token/create", createWithOptions(map[string]any{
			"override_username": "user_good", "override_password": "error_" + tcase.code,
		})).AssertStatus(tcase.status).JSONMap()
		if m["error_code"] != tcase.code || m["error_type"] != tcase.errType || m["public_token"] != nil || m["request_id"] == nil {
			t.Fatalf("%s: %v", tcase.code, m)
		}
	}
	if mem.PublicTokens.Count() != 0 {
		t.Fatal("a failed create left a public token")
	}
}

func TestSandboxErrorPasswordIsIgnoredWhenNotSimulated(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	for name, opts := range map[string]map[string]any{
		"another username":                 {"override_username": "user_other", "override_password": "error_ITEM_LOCKED"},
		"an ordinary password":             {"override_username": "user_good", "override_password": "pass_good"},
		"login required is not a password": {"override_password": "error_ITEM_LOGIN_REQUIRED"},
		"a listed code with no sample":     {"override_password": "error_COUNTRY_NOT_SUPPORTED"},
		"a code that is not listed":        {"override_password": "error_NOT_A_CODE"},
	} {
		if m := tc.Post("/sandbox/public_token/create", createWithOptions(opts)).AssertStatus(200).JSONMap(); m["public_token"] == nil {
			t.Fatalf("%s: %v", name, m)
		}
	}
	if m := tc.Post("/sandbox/public_token/create", createWithOptions(map[string]any{"override_password": 5})).
		AssertStatus(400).JSONMap(); m["error_code"] != "INVALID_BODY" {
		t.Fatalf("wrong type = %v", m)
	}
}

func TestSandboxResetLoginPutsTheItemInLoginRequired(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	body := func() map[string]any { return creds(map[string]any{"access_token": token}) }

	if m := tc.Post("/sandbox/item/reset_login", body()).AssertStatus(200).JSONMap(); m["reset_login"] != true || m["request_id"] == nil {
		t.Fatalf("reset_login = %v", m)
	}
	item := tc.Post("/item/get", body()).AssertStatus(200).JSONMap()["item"].(map[string]any)
	e, _ := item["error"].(map[string]any)
	if e["error_code"] != "ITEM_LOGIN_REQUIRED" || e["error_type"] != "ITEM_ERROR" || e["display_message"] != nil || e["request_id"] != nil {
		t.Fatalf("item.error = %v", item["error"])
	}
	m := tc.Post("/accounts/get", body()).AssertStatus(400).JSONMap()
	if m["error_code"] != "ITEM_LOGIN_REQUIRED" || m["error_type"] != "ITEM_ERROR" {
		t.Fatalf("accounts/get = %v", m)
	}
	tc.Post("/link/token/create", linkBody(map[string]any{"access_token": token})).AssertStatus(200)
	tc.Post("/item/access_token/invalidate", body()).AssertStatus(200)
}

func TestSandboxResetLoginLeavesOtherItemsAlone(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	bad, good := linkedItem(tc), linkedItem(tc)
	tc.Post("/sandbox/item/reset_login", creds(map[string]any{"access_token": bad})).AssertStatus(200)
	if len(accountsOf(tc, good)) != 3 {
		t.Fatal("another Item lost its accounts")
	}
	if m := tc.Post("/sandbox/item/reset_login", creds(map[string]any{"access_token": "access-sandbox-nope"})).
		AssertStatus(400).JSONMap(); m["error_code"] != "INVALID_ACCESS_TOKEN" {
		t.Fatalf("unknown token = %v", m)
	}
}
