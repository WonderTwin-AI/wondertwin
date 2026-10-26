package api_test

import "testing"

func TestIdentityGetReturnsTheDocumentedHolderOnEveryAccount(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	res := tc.Post("/identity/get", creds(map[string]any{"access_token": linkedItem(tc)})).AssertStatus(200).JSONMap()
	accounts := res["accounts"].([]any)
	if len(accounts) != 3 || res["item"] == nil || res["request_id"] == nil {
		t.Fatalf("response = %v", res)
	}
	for _, a := range accounts {
		acct := a.(map[string]any)
		owners := acct["owners"].([]any)
		if len(owners) != 1 || acct["balances"] == nil || acct["account_id"] == nil {
			t.Fatalf("account = %v", acct)
		}
		o := owners[0].(map[string]any)
		if o["names"].([]any)[0] != "Alberta Bobbeth Charleson" || len(o["phone_numbers"].([]any)) != 3 || len(o["emails"].([]any)) != 3 {
			t.Fatalf("owner = %v", o)
		}
		addrs := o["addresses"].([]any)
		first := addrs[0].(map[string]any)
		if len(addrs) != 2 || first["primary"] != true || first["data"].(map[string]any)["street"] != "2992 Cameron Road" ||
			first["data"].(map[string]any)["postal_code"] != "14236" {
			t.Fatalf("addresses = %v", addrs)
		}
	}
}

func TestIdentityGetFiltersAndFollowsTheItemsState(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	one := accountsOf(tc, token)[1]["account_id"]
	got := tc.Post("/identity/get", creds(map[string]any{"access_token": token, "options": map[string]any{"account_ids": []any{one}}})).
		AssertStatus(200).JSONMap()["accounts"].([]any)
	if len(got) != 1 || got[0].(map[string]any)["account_id"] != one {
		t.Fatalf("filtered = %v", got)
	}
	broken, gone := linkedItem(tc), linkedItem(tc)
	tc.Post("/sandbox/item/reset_login", creds(map[string]any{"access_token": broken})).AssertStatus(200)
	tc.Post("/item/remove", creds(map[string]any{"access_token": gone})).AssertStatus(200)
	for tok, code := range map[string]string{broken: "ITEM_LOGIN_REQUIRED", gone: "ITEM_NOT_FOUND", "access-sandbox-nope": "INVALID_ACCESS_TOKEN"} {
		if m := tc.Post("/identity/get", creds(map[string]any{"access_token": tok})).AssertStatus(400).JSONMap(); m["error_code"] != code {
			t.Fatalf("%s: %v", tok, m)
		}
	}
	if m := tc.Post("/identity/get", creds(map[string]any{"access_token": token, "options": map[string]any{"account_ids": []any{"nope"}}})).
		AssertStatus(400).JSONMap(); m["error_code"] != "INVALID_ACCOUNT_ID" {
		t.Fatalf("unknown id = %v", m)
	}
}

func TestIdentityMatchScoresTheGivenUserAgainstTheHolder(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	run := func(user map[string]any) map[string]any {
		res := tc.Post("/identity/match", creds(map[string]any{"access_token": token, "user": user})).AssertStatus(200).JSONMap()
		return res["accounts"].([]any)[0].(map[string]any)
	}

	perfect := run(map[string]any{
		"legal_name": "alberta bobbeth charleson", "phone_number": "+1 (202) 555-0123", "email_address": "AccountHolder0@Example.com",
		"address": map[string]any{"street": "2992 Cameron Road", "city": "Malakoff", "region": "NY", "postal_code": "14236", "country": "US"},
	})
	if perfect["legal_name"].(map[string]any)["score"] != 100.0 || perfect["phone_number"].(map[string]any)["score"] != 100.0 ||
		perfect["email_address"].(map[string]any)["score"] != 100.0 || perfect["address"].(map[string]any)["score"] != 100.0 ||
		perfect["address"].(map[string]any)["is_postal_code_match"] != true {
		t.Fatalf("a perfect match = %v", perfect)
	}
	for k, v := range perfect["balances"].(map[string]any) {
		if v != nil {
			t.Fatalf("balances.%s must be null in a match response, as Plaid documents: %v", k, v)
		}
	}

	wrong := run(map[string]any{"legal_name": "Jane Smith", "phone_number": "+14155552671", "email_address": "jane@example.com"})
	if n := wrong["legal_name"].(map[string]any); n["score"] != 0.0 || n["is_first_name_or_last_name_match"] != false {
		t.Fatalf("a different name = %v", n)
	}
	if wrong["phone_number"].(map[string]any)["score"] != 0.0 || wrong["email_address"].(map[string]any)["score"] != 0.0 {
		t.Fatalf("a different phone and email = %v", wrong)
	}
	if a := wrong["address"].(map[string]any); a["score"] != nil || a["is_postal_code_match"] != nil {
		t.Fatalf("no address given must score null: %v", a)
	}
	if n := run(map[string]any{"legal_name": "Alberta Charleson"})["legal_name"].(map[string]any); n["score"] != 90.0 {
		t.Fatalf("a name missing its middle name = %v", n)
	}
	if n := run(map[string]any{"legal_name": "Acme Holdings LLC"})["legal_name"].(map[string]any); n["is_business_name_detected"] != true {
		t.Fatalf("a business name = %v", n)
	}
}

func TestIdentityMatchChecksItsRequest(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	cases := map[string]map[string]any{
		"MISSING_FIELDS": {"access_token": token},
		"INVALID_BODY":   {"access_token": token, "user": map[string]any{"legal_name": 5}},
	}
	for code, body := range cases {
		if m := tc.Post("/identity/match", creds(body)).AssertStatus(400).JSONMap(); m["error_code"] != code {
			t.Fatalf("%s: %v", code, m)
		}
	}
	bad := map[string]any{"access_token": token, "user": map[string]any{"address": "x"}}
	if m := tc.Post("/identity/match", creds(bad)).AssertStatus(400).JSONMap(); m["error_code"] != "INVALID_BODY" {
		t.Fatalf("address of the wrong type: %v", m)
	}
}
