package api_test

import "testing"

func TestAccountsBalanceGetReturnsTheSameAccountsAsAccountsGet(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	body := creds(map[string]any{"access_token": token})

	bal := tc.Post("/accounts/balance/get", body).AssertStatus(200).JSONMap()
	get := tc.Post("/accounts/get", creds(map[string]any{"access_token": token})).JSONMap()
	b, g := bal["accounts"].([]any), get["accounts"].([]any)
	if len(b) != 3 || len(g) != 3 {
		t.Fatalf("balance %d accounts, get %d", len(b), len(g))
	}
	for i := range b {
		bb, gb := b[i].(map[string]any)["balances"].(map[string]any), g[i].(map[string]any)["balances"].(map[string]any)
		if bb["current"] != gb["current"] || bb["available"] != gb["available"] {
			t.Fatalf("balances disagree between the two routes for account %d: %v vs %v", i, bb, gb)
		}
	}
	checking := b[0].(map[string]any)["balances"].(map[string]any)
	if checking["available"] != 100.0 || checking["current"] != 110.0 || bal["item"] == nil || bal["request_id"] == nil {
		t.Fatalf("response = %v", bal)
	}
}

func TestAccountIDsFilterBothRoutesAndKeepPositionOrder(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	all := accountsOf(tc, token)
	want := []any{all[2]["account_id"], all[0]["account_id"]} // asked for out of order

	for _, path := range []string{"/accounts/get", "/accounts/balance/get"} {
		res := tc.Post(path, creds(map[string]any{"access_token": token, "options": map[string]any{"account_ids": want}})).
			AssertStatus(200).JSONMap()["accounts"].([]any)
		if len(res) != 2 || res[0].(map[string]any)["account_id"] != all[0]["account_id"] || res[1].(map[string]any)["account_id"] != all[2]["account_id"] {
			t.Fatalf("%s returned %v, want the two asked for in Position order", path, res)
		}
	}
}

func TestAccountIDsRefuseAnIDTheItemDoesNotHave(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	mine, other := linkedItem(tc), accountsOf(tc, linkedItem(tc))[0]["account_id"]
	for _, ids := range [][]any{{"not-an-account"}, {other}, {accountsOf(tc, mine)[0]["account_id"], other}} {
		m := tc.Post("/accounts/balance/get", creds(map[string]any{"access_token": mine, "options": map[string]any{"account_ids": ids}})).
			AssertStatus(400).JSONMap()
		if m["error_code"] != "INVALID_ACCOUNT_ID" || m["error_type"] != "INVALID_INPUT" {
			t.Fatalf("%v: %v", ids, m)
		}
	}
	for _, bad := range []map[string]any{{"options": "x"}, {"options": map[string]any{"account_ids": "a"}}, {"options": map[string]any{"account_ids": []any{1}}}} {
		bad["access_token"] = mine
		if m := tc.Post("/accounts/balance/get", creds(bad)).AssertStatus(400).JSONMap(); m["error_code"] != "INVALID_BODY" {
			t.Fatalf("%v: %v", bad, m)
		}
	}
}

func TestAccountsBalanceGetFollowsTheItemsState(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	broken, gone := linkedItem(tc), linkedItem(tc)
	tc.Post("/sandbox/item/reset_login", creds(map[string]any{"access_token": broken})).AssertStatus(200)
	tc.Post("/item/remove", creds(map[string]any{"access_token": gone})).AssertStatus(200)

	for token, code := range map[string]string{broken: "ITEM_LOGIN_REQUIRED", gone: "ITEM_NOT_FOUND", "access-sandbox-nope": "INVALID_ACCESS_TOKEN"} {
		if m := tc.Post("/accounts/balance/get", creds(map[string]any{"access_token": token})).AssertStatus(400).JSONMap(); m["error_code"] != code {
			t.Fatalf("%s: %v", token, m)
		}
	}
}
