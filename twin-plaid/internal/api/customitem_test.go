package api_test

import (
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

// acmeConfig is a user_custom configuration in the shape the lender demo
// sends: an operating checking account with its own numbers, a savings
// account left to defaults, and a credit card with a limit and no available
// balance.
const acmeConfig = `{
	"override_accounts": [
		{"type": "depository", "subtype": "checking", "starting_balance": 48210.55, "currency": "USD",
		 "meta": {"name": "ACME Operating", "official_name": "ACME Commerce Operating Account"},
		 "numbers": {"account": "9900001234", "ach_routing": "011401533", "ach_wire_routing": "026009593"}},
		{"type": "depository", "subtype": "savings", "starting_balance": 15000, "force_available_balance": 14000},
		{"type": "credit", "subtype": "credit card", "starting_balance": 1820.4, "has_null_available_balance": true,
		 "meta": {"limit": 10000, "mask": "77"}}
	]}`

// customItem links a user_custom Item from cfg and returns its access token.
func customItem(t *testing.T, tc *testutil.TwinClient, cfg string) string {
	t.Helper()
	pub := tc.Post("/sandbox/public_token/create", customCreate(cfg)).AssertStatus(200).JSONMap()["public_token"]
	return tc.Post("/item/public_token/exchange", creds(map[string]any{"public_token": pub})).
		AssertStatus(200).JSONMap()["access_token"].(string)
}

func balancesOf(a map[string]any) map[string]any { return a["balances"].(map[string]any) }

func TestCustomItemAccountsGetBuildsOneAccountPerConfiguredEntry(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	accounts := accountsOf(tc, customItem(t, tc, acmeConfig))
	if len(accounts) != 3 {
		t.Fatalf("accounts = %v", accounts)
	}
	want := []struct {
		name, official, mask, typ, subtype string
	}{
		{"ACME Operating", "ACME Commerce Operating Account", "1234", "depository", "checking"},
		{"Plaid Savings", "", "2222", "depository", "savings"},
		{"Plaid Credit Card", "", "77", "credit", "credit card"},
	}
	for i, w := range want {
		a := accounts[i]
		if a["name"] != w.name || a["mask"] != w.mask || a["type"] != w.typ || a["subtype"] != w.subtype {
			t.Errorf("account %d = %v, want %+v", i, a, w)
		}
		if w.official == "" && a["official_name"] != nil || w.official != "" && a["official_name"] != w.official {
			t.Errorf("account %d official_name = %v, want %q", i, a["official_name"], w.official)
		}
	}
}

func TestCustomItemAccountsGetFillsAnEmptyEntryFromTheFixture(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	accounts := accountsOf(tc, customItem(t, tc,
		`{"override_accounts": [{"type": "depository", "subtype": "checking"}, {"type": "loan", "subtype": "student"}]}`))
	if len(accounts) != 2 {
		t.Fatalf("accounts = %v", accounts)
	}
	checking, loan := accounts[0], accounts[1]
	if checking["name"] != "Plaid Checking" || checking["official_name"] != "Plaid Gold Standard 0% Interest Checking" ||
		checking["mask"] != "0000" || balancesOf(checking)["current"] != 110.0 || balancesOf(checking)["available"] != 100.0 {
		t.Fatalf("checking = %v", checking)
	}
	if loan["name"] != "Plaid Student Loan" || loan["mask"] != "7777" || balancesOf(loan)["current"] != 65262.0 {
		t.Fatalf("loan = %v", loan)
	}
}

func TestCustomItemBalanceGetReturnsTheStartingBalances(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := customItem(t, tc, acmeConfig)
	res := tc.Post("/accounts/balance/get", creds(map[string]any{"access_token": token})).AssertStatus(200).JSONMap()
	accounts := res["accounts"].([]any)
	want := []map[string]any{
		{"current": 48210.55, "available": 48210.55, "limit": nil, "iso_currency_code": "USD"},
		{"current": 15000.0, "available": 14000.0, "limit": nil, "iso_currency_code": "USD"},
		{"current": 1820.4, "available": nil, "limit": 10000.0, "iso_currency_code": "USD"},
	}
	for i, w := range want {
		b := balancesOf(accounts[i].(map[string]any))
		for k, v := range w {
			if got, ok := b[k]; !ok || got != v {
				t.Errorf("account %d balances.%s = %v, want %v", i, k, got, v)
			}
		}
	}
	// /accounts/get serves the same balances (quirk LIFECYCLE-006).
	if b := balancesOf(accountsOf(tc, token)[0]); b["current"] != 48210.55 {
		t.Fatalf("/accounts/get current = %v", b["current"])
	}
}

func TestCustomItemBalanceGetUsesTheConfiguredCurrency(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := customItem(t, tc, `{"override_accounts": [{"type": "depository", "subtype": "checking", "starting_balance": 10, "currency": "CAD"}]}`)
	if b := balancesOf(accountsOf(tc, token)[0]); b["iso_currency_code"] != "CAD" {
		t.Fatalf("balances = %v", b)
	}
}

func TestCustomItemAuthGetServesConfiguredAndDefaultNumbers(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	res := tc.Post("/auth/get", creds(map[string]any{"access_token": customItem(t, tc, acmeConfig)})).AssertStatus(200).JSONMap()
	accounts, ach := res["accounts"].([]any), res["numbers"].(map[string]any)["ach"].([]any)
	if len(accounts) != 2 || len(ach) != 2 {
		t.Fatalf("auth/get must serve the checking and savings accounts, not the card: %v", res)
	}
	operating, savings := ach[0].(map[string]any), ach[1].(map[string]any)
	if operating["account"] != "9900001234" || operating["routing"] != "011401533" || operating["wire_routing"] != "026009593" {
		t.Fatalf("configured numbers = %v", operating)
	}
	// A savings account without numbers gets the fixture's routing and an account number ending in its mask.
	if savings["account"] != "1111222233332222" || savings["routing"] != "021000021" || savings["wire_routing"] != nil {
		t.Fatalf("default numbers = %v", savings)
	}
}

func TestCustomItemAuthGetWithoutAuthAccountsIsNoAuthAccounts(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := customItem(t, tc, `{"override_accounts": [{"type": "credit", "subtype": "credit card", "starting_balance": 5}]}`)
	if m := tc.Post("/auth/get", creds(map[string]any{"access_token": token})).AssertStatus(400).JSONMap(); m["error_code"] != "NO_AUTH_ACCOUNTS" {
		t.Fatalf("body = %v", m)
	}
}

func TestCustomItemWithoutOverrideAccountsHasNoAccountsAndNoFixtureRows(t *testing.T) {
	tc, _, mem := setupPlaid(t)
	token := customItem(t, tc, `{}`)
	if accounts := tc.Post("/accounts/get", creds(map[string]any{"access_token": token})).AssertStatus(200).JSONMap()["accounts"].([]any); len(accounts) != 0 {
		t.Fatalf("accounts = %v", accounts)
	}
	if mem.Transactions.Count() != 0 {
		t.Fatal("a custom Item got the fixture transactions")
	}
}

func TestCustomItemStateSurvivesAReload(t *testing.T) {
	tc, ac, _ := setupPlaid(t)
	token := customItem(t, tc, acmeConfig)
	before := accountsOf(tc, token)

	ac.LoadState(ac.GetState().AssertStatus(200).JSONMap()).AssertStatus(200)

	after := accountsOf(tc, token)
	if len(after) != len(before) {
		t.Fatalf("after reload: %v", after)
	}
	for i := range before {
		if before[i]["account_id"] != after[i]["account_id"] || before[i]["name"] != after[i]["name"] ||
			balancesOf(before[i])["current"] != balancesOf(after[i])["current"] {
			t.Fatalf("account %d changed after reload: %v then %v", i, before[i], after[i])
		}
	}
}

func TestCustomItemResetRemovesItsAccounts(t *testing.T) {
	tc, ac, mem := setupPlaid(t)
	customItem(t, tc, acmeConfig)
	ac.Reset().AssertStatus(200)
	if mem.Accounts.Count() != 0 || mem.Items.Count() != 0 {
		t.Fatalf("reset left %d accounts and %d Items", mem.Accounts.Count(), mem.Items.Count())
	}
}
