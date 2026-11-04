package api_test

import (
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

func accountBanks(tc *testutil.TwinClient, acct string) []map[string]any {
	list, _ := stripeGet(tc, "/v1/accounts/"+acct).JSONMap()["external_accounts"].(map[string]any)
	data, _ := list["data"].([]any)
	var banks []map[string]any
	for _, d := range data {
		if m, ok := d.(map[string]any); ok {
			banks = append(banks, m)
		}
	}
	return banks
}

func requiresExternalAccount(acct map[string]any) bool {
	req, _ := acct["requirements"].(map[string]any)
	for _, k := range []string{"currently_due", "eventually_due", "past_due"} {
		fields, _ := req[k].([]any)
		for _, f := range fields {
			if f == "external_account" {
				return true
			}
		}
	}
	return false
}

func TestAccountExternalAccountOnCreate(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/accounts?type=custom&country=US&external_account=btok_us_verified", nil)
	resp.AssertStatus(200)
	acct := resp.JSONMap()
	list, _ := acct["external_accounts"].(map[string]any)
	if data, _ := list["data"].([]any); len(data) != 1 {
		t.Fatalf("created account lists %d external accounts, want 1: %v", len(data), list)
	}
	banks := accountBanks(tc, acct["id"].(string))
	if len(banks) != 1 || banks[0]["last4"] != "6789" || banks[0]["bank_name"] != "STRIPE TEST BANK" {
		t.Errorf("external accounts = %v", banks)
	}
	if requiresExternalAccount(acct) || requiresExternalAccount(stripeGet(tc, "/v1/accounts/"+acct["id"].(string)).JSONMap()) {
		t.Errorf("requirements still list external_account: %v", acct["requirements"])
	}

	dict := stripePost(tc, "/v1/accounts?type=custom&country=US&external_account[object]=bank_account"+
		"&external_account[account_number]=000999999991&external_account[routing_number]=110000000"+
		"&external_account[country]=US&external_account[currency]=usd", nil)
	dict.AssertStatus(200)
	if banks := accountBanks(tc, dict.JSONMap()["id"].(string)); len(banks) != 1 || banks[0]["last4"] != "9991" {
		t.Errorf("dictionary form: external accounts = %v", banks)
	}
}

func TestAccountExternalAccountRefusedTokenCreatesNoAccount(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/accounts?type=custom&country=US&external_account=btok_unknown", nil)
	resp.AssertStatus(400)
	if e, _ := resp.JSONMap()["error"].(map[string]any); e["code"] != "resource_missing" {
		t.Errorf("error = %v", e)
	}
	if accounts, _ := tc.Get("/admin/state").JSONMap()["accounts"].(map[string]any); len(accounts) != 0 {
		t.Errorf("a refused create left %d accounts", len(accounts))
	}
}

func TestAccountExternalAccountOnUpdateReplacesDefault(t *testing.T) {
	_, tc := setupStripe(t)
	acct := stripePost(tc, "/v1/accounts?type=custom&country=US&external_account=btok_us_verified", nil).JSONMap()["id"].(string)
	tc.Post("/admin/bank_account_tokens", map[string]any{"token": "btok_next", "bank_name": "First Platypus Bank", "last4": "1111"}).
		AssertStatus(200)

	resp := stripePost(tc, "/v1/accounts/"+acct+"?external_account=btok_next", nil)
	resp.AssertStatus(200)
	banks := accountBanks(tc, acct)
	if len(banks) != 1 || banks[0]["bank_name"] != "First Platypus Bank" || banks[0]["last4"] != "1111" {
		t.Errorf("after update, external accounts = %v, want only the new default", banks)
	}
	events, _ := stripeGet(tc, "/v1/events?type=account.updated").JSONMap()["data"].([]any)
	latest, _ := events[0].(map[string]any)
	obj, _ := latest["data"].(map[string]any)["object"].(map[string]any)
	evList, _ := obj["external_accounts"].(map[string]any)
	evData, _ := evList["data"].([]any)
	if len(evData) != 1 || evData[0].(map[string]any)["bank_name"] != "First Platypus Bank" {
		t.Errorf("account.updated event external_accounts = %v, want the new default", evList)
	}

	reuse := stripePost(tc, "/v1/accounts/"+acct+"?external_account=btok_next&email=changed@example.com", nil)
	reuse.AssertStatus(400)
	if e, _ := reuse.JSONMap()["error"].(map[string]any); e["code"] != "token_already_used" {
		t.Errorf("reuse error = %v", e)
	}
	if email := stripeGet(tc, "/v1/accounts/"+acct).JSONMap()["email"]; email == "changed@example.com" {
		t.Errorf("a refused update still changed the email")
	}
}

func TestExternalAccountsRouteClearsRequirement(t *testing.T) {
	_, tc := setupStripe(t)
	acct := newConnectAccount(t, tc)
	if !requiresExternalAccount(stripeGet(tc, "/v1/accounts/"+acct).JSONMap()) {
		t.Fatalf("a new account should require external_account")
	}

	stripePost(tc, "/v1/accounts/"+acct+"/external_accounts?external_account=btok_us_verified", nil).AssertStatus(200)
	if got := stripeGet(tc, "/v1/accounts/"+acct).JSONMap(); requiresExternalAccount(got) {
		t.Errorf("requirements still list external_account: %v", got["requirements"])
	}
}
