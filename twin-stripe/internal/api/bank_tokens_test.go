package api_test

import (
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

func newConnectAccount(t *testing.T, tc *testutil.TwinClient) string {
	t.Helper()
	resp := stripePost(tc, "/v1/accounts?type=custom&country=US", nil)
	resp.AssertStatus(200)
	return resp.JSONMap()["id"].(string)
}

// storeToken loads a token into the store through /admin/state, as a token
// another service issued would be.
func storeToken(t *testing.T, tc *testutil.TwinClient, tok map[string]any) {
	t.Helper()
	state := tc.Get("/admin/state").JSONMap()
	tokens, _ := state["tokens"].(map[string]any)
	if tokens == nil {
		tokens = map[string]any{}
	}
	tokens[tok["id"].(string)] = tok
	state["tokens"] = tokens
	tc.Post("/admin/state", state).AssertStatus(200)
}

func externalAccountCount(tc *testutil.TwinClient, acct string) int {
	ea := stripeGet(tc, "/v1/accounts/"+acct).JSONMap()["external_accounts"].(map[string]any)
	data, _ := ea["data"].([]any)
	return len(data)
}

func TestExternalAccountTokenUnknownIsRefused(t *testing.T) {
	_, tc := setupStripe(t)
	acct := newConnectAccount(t, tc)

	resp := stripePost(tc, "/v1/accounts/"+acct+"/external_accounts?external_account=btok_this_does_not_exist", nil)
	resp.AssertStatus(400)
	e, _ := resp.JSONMap()["error"].(map[string]any)
	if e["type"] != "invalid_request_error" || e["code"] != "resource_missing" || e["param"] != "external_account" ||
		e["message"] != "No such token: 'btok_this_does_not_exist'" {
		t.Errorf("error = %v", e)
	}
	if n := externalAccountCount(tc, acct); n != 0 {
		t.Errorf("a refused token created %d external accounts", n)
	}
}

func TestExternalAccountTokenTestTokenResolvesAndIsReusable(t *testing.T) {
	_, tc := setupStripe(t)
	acct := newConnectAccount(t, tc)

	var fingerprints []any
	for i := 0; i < 2; i++ {
		resp := stripePost(tc, "/v1/accounts/"+acct+"/external_accounts?external_account=btok_us_verified", nil)
		resp.AssertStatus(200)
		ea := resp.JSONMap()
		if ea["bank_name"] != "STRIPE TEST BANK" || ea["last4"] != "6789" || ea["routing_number"] != "110000000" ||
			ea["country"] != "US" || ea["currency"] != "usd" || ea["object"] != "bank_account" {
			t.Errorf("use %d: external account = %v", i+1, ea)
		}
		fingerprints = append(fingerprints, ea["fingerprint"])
	}

	raw := stripePost(tc, "/v1/accounts/"+acct+"/external_accounts?external_account[object]=bank_account"+
		"&external_account[routing_number]=110000000&external_account[account_number]=000123456789"+
		"&external_account[country]=US&external_account[currency]=usd", nil)
	raw.AssertStatus(200)
	if fp := raw.JSONMap()["fingerprint"]; fp != fingerprints[0] || fp != fingerprints[1] {
		t.Errorf("fingerprints differ for one bank account: token %v, raw numbers %v", fingerprints, fp)
	}
}

func TestExternalAccountTokenStoredTokenIsSingleUse(t *testing.T) {
	_, tc := setupStripe(t)
	acct := newConnectAccount(t, tc)
	storeToken(t, tc, map[string]any{
		"id": "btok_plaid000000000000000001", "object": "token", "type": "bank_account", "used": false,
		"bank_account": map[string]any{
			"id": "ba_plaid1", "object": "bank_account", "bank_name": "First Platypus Bank",
			"country": "US", "currency": "usd", "last4": "1111", "routing_number": "011401533",
			"fingerprint": "fp_plaid1", "status": "new",
		},
	})

	path := "/v1/accounts/" + acct + "/external_accounts?external_account=btok_plaid000000000000000001"
	resp := stripePost(tc, path, nil)
	resp.AssertStatus(200)
	ea := resp.JSONMap()
	if ea["bank_name"] != "First Platypus Bank" || ea["last4"] != "1111" || ea["routing_number"] != "011401533" ||
		ea["fingerprint"] != "fp_plaid1" || ea["account"] != acct {
		t.Errorf("external account = %v", ea)
	}
	tokens, _ := tc.Get("/admin/state").JSONMap()["tokens"].(map[string]any)
	tok, _ := tokens["btok_plaid000000000000000001"].(map[string]any)
	if used := tok["used"]; used != true {
		t.Errorf("token used = %v after attaching, want true", used)
	}

	resp = stripePost(tc, path, nil)
	resp.AssertStatus(400)
	e, _ := resp.JSONMap()["error"].(map[string]any)
	if e["code"] != "token_already_used" || e["param"] != "external_account" || e["doc_url"] == nil {
		t.Errorf("reuse error = %v", e)
	}
	if n := externalAccountCount(tc, acct); n != 1 {
		t.Errorf("external accounts = %d after a refused reuse, want 1", n)
	}
}

func TestExternalAccountTokenCardTokenIsNotABankAccount(t *testing.T) {
	_, tc := setupStripe(t)
	acct := newConnectAccount(t, tc)
	card := stripePost(tc, "/v1/tokens?card[number]=4000056655665556&card[exp_month]=12&card[exp_year]=2030", nil)
	card.AssertStatus(200)
	id := card.JSONMap()["id"].(string)

	resp := stripePost(tc, "/v1/accounts/"+acct+"/external_accounts?external_account="+id, nil)
	resp.AssertStatus(400)
	e, _ := resp.JSONMap()["error"].(map[string]any)
	if e["code"] != "resource_missing" {
		t.Errorf("code = %v, want resource_missing", e["code"])
	}
}
