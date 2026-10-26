package api_test

import (
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

// linkedItem makes an Item through the sandbox shortcut and returns its access token.
func linkedItem(tc *testutil.TwinClient) string {
	pub := tc.Post("/sandbox/public_token/create", creds(map[string]any{
		"institution_id": "ins_109508", "initial_products": []string{"auth", "identity", "transactions"},
	})).JSONMap()["public_token"]
	return tc.Post("/item/public_token/exchange", creds(map[string]any{"public_token": pub})).
		JSONMap()["access_token"].(string)
}

func accountsOf(tc *testutil.TwinClient, token string) []map[string]any {
	var out []map[string]any
	for _, a := range tc.Post("/accounts/get", creds(map[string]any{"access_token": token})).
		AssertStatus(200).JSONMap()["accounts"].([]any) {
		out = append(out, a.(map[string]any))
	}
	return out
}

func TestAccountsGetReturnsTheDefaultAccountsWithPlaidsPresenceRules(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)

	res := tc.Post("/accounts/get", creds(map[string]any{"access_token": token})).AssertStatus(200).JSONMap()
	accounts := accountsOf(tc, token)
	var names []string
	for _, a := range accounts {
		names = append(names, a["name"].(string))
		if !idRE.MatchString(a["account_id"].(string)) {
			t.Fatalf("account_id %v is not 37 base62 characters", a["account_id"])
		}
		if _, ok := a["persistent_account_id"]; ok {
			t.Fatalf("persistent_account_id must be omitted, never null: %v", a)
		}
		for _, k := range []string{"mask", "official_name", "subtype", "type", "balances"} {
			if _, ok := a[k]; !ok {
				t.Fatalf("required field %s missing: %v", k, a)
			}
		}
		for _, k := range []string{"available", "current", "limit", "iso_currency_code", "unofficial_currency_code"} {
			if _, ok := a["balances"].(map[string]any)[k]; !ok {
				t.Fatalf("balances.%s missing: %v", k, a["balances"])
			}
		}
	}
	if len(names) != 3 || names[0] != "Plaid Checking" || names[1] != "Plaid 401k" || names[2] != "Plaid Student Loan" {
		t.Fatalf("accounts = %v", names)
	}
	checking := accounts[0]["balances"].(map[string]any)
	if checking["available"] != 100.0 || checking["current"] != 110.0 || checking["limit"] != nil ||
		checking["iso_currency_code"] != "USD" || accounts[1]["official_name"] != nil {
		t.Fatalf("balances = %v, 401k = %v", checking, accounts[1])
	}
	if res["item"].(map[string]any)["item_id"] == nil || res["request_id"] == nil {
		t.Fatalf("response = %v", res)
	}
}

func TestAccountsGetIsPerItemWithOwnIDs(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	a, b := accountsOf(tc, linkedItem(tc)), accountsOf(tc, linkedItem(tc))
	if len(a) != 3 || len(b) != 3 || a[0]["account_id"] == b[0]["account_id"] {
		t.Fatalf("two Items share accounts: %v / %v", a, b)
	}
}

func TestAccountsGetKeepsOrderAfterAStateReload(t *testing.T) {
	tc, ac, _ := setupPlaid(t)
	token := linkedItem(tc)
	before := accountsOf(tc, token)

	state := ac.GetState().AssertStatus(200).JSONMap()
	ac.LoadState(state).AssertStatus(200)

	after := accountsOf(tc, token)
	for i := range before {
		if before[i]["account_id"] != after[i]["account_id"] {
			t.Fatalf("order changed after a state reload: %v then %v", before, after)
		}
	}
}

func TestAccountsGetRefusesAnUnknownAccessToken(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	m := tc.Post("/accounts/get", creds(map[string]any{"access_token": "access-sandbox-nope"})).AssertStatus(400).JSONMap()
	if m["error_code"] != "INVALID_ACCESS_TOKEN" {
		t.Fatalf("body = %v", m)
	}
}

func TestANewItemStartsWithTheTwoPublishedTransactions(t *testing.T) {
	tc, _, mem := setupPlaid(t)
	token := linkedItem(tc)
	item, _ := mem.ItemForAccessToken(token)
	rows := mem.TransactionsForItem(item.ID)
	if len(rows) != 2 || rows[0].AccountID != accountsOf(tc, token)[0]["account_id"] {
		t.Fatalf("rows = %+v, want two on the checking account", rows)
	}
	page, err := mem.Sync(item.ID, "", 100)
	if err != nil || len(page.Added) != 2 {
		t.Fatalf("a first sync = %+v, %v", page, err)
	}
}
