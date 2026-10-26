package api_test

import "testing"

func TestAuthGetReturnsTheCheckingAccountsNumbersOnly(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)

	res := tc.Post("/auth/get", creds(map[string]any{"access_token": token})).AssertStatus(200).JSONMap()
	accounts := res["accounts"].([]any)
	if len(accounts) != 1 || accounts[0].(map[string]any)["name"] != "Plaid Checking" {
		t.Fatalf("accounts = %v, want only the checking account", accounts)
	}
	numbers := res["numbers"].(map[string]any)
	ach := numbers["ach"].([]any)
	if len(ach) != 1 {
		t.Fatalf("ach = %v", ach)
	}
	n := ach[0].(map[string]any)
	if n["account_id"] != accounts[0].(map[string]any)["account_id"] || n["account"] != "1111222233330000" || n["routing"] != "021000021" ||
		n["is_tokenized_account_number"] != false {
		t.Fatalf("ach entry = %v", n)
	}
	if v, ok := n["wire_routing"]; !ok || v != nil {
		t.Fatalf("wire_routing must be present as null when not published: %v", n)
	}
	for _, k := range []string{"can_transfer_in", "can_transfer_out"} {
		if _, ok := n[k]; ok {
			t.Fatalf("optional %s must be omitted: %v", k, n)
		}
	}
	for _, k := range []string{"eft", "international", "bacs"} {
		if arr, ok := numbers[k].([]any); !ok || len(arr) != 0 {
			t.Fatalf("%s must be an empty array for a US Item: %v", k, numbers[k])
		}
	}
	if res["item"] == nil || res["request_id"] == nil {
		t.Fatalf("response = %v", res)
	}
}

func TestAuthGetAccountIDsAndTheNoAuthAccountsError(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	all := accountsOf(tc, token) // checking, 401k, student loan
	auth := func(ids ...any) map[string]any {
		return tc.Post("/auth/get", creds(map[string]any{"access_token": token, "options": map[string]any{"account_ids": ids}})).JSONMap()
	}

	if m := auth(all[0]["account_id"]); len(m["accounts"].([]any)) != 1 {
		t.Fatalf("the checking account by id = %v", m)
	}
	if m := auth(all[1]["account_id"]); m["error_code"] != "NO_AUTH_ACCOUNTS" || m["error_type"] != "ITEM_ERROR" || m["display_message"] != nil {
		t.Fatalf("an account Auth does not serve = %v", m)
	}
	if m := auth("not-an-account"); m["error_code"] != "INVALID_ACCOUNT_ID" {
		t.Fatalf("unknown id = %v", m)
	}
	if m := tc.Post("/auth/get", creds(map[string]any{"access_token": token, "options": "x"})).JSONMap(); m["error_code"] != "INVALID_BODY" {
		t.Fatalf("malformed options = %v", m)
	}
}

func TestAuthGetFollowsTheItemsState(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	broken, gone := linkedItem(tc), linkedItem(tc)
	tc.Post("/sandbox/item/reset_login", creds(map[string]any{"access_token": broken})).AssertStatus(200)
	tc.Post("/item/remove", creds(map[string]any{"access_token": gone})).AssertStatus(200)
	for token, code := range map[string]string{broken: "ITEM_LOGIN_REQUIRED", gone: "ITEM_NOT_FOUND", "access-sandbox-nope": "INVALID_ACCESS_TOKEN"} {
		if m := tc.Post("/auth/get", creds(map[string]any{"access_token": token})).AssertStatus(400).JSONMap(); m["error_code"] != code {
			t.Fatalf("%s: %v", token, m)
		}
	}
}
