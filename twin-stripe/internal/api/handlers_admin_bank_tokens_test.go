package api_test

import (
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

func attachToken(tc *testutil.TwinClient, acct, token string) *testutil.Response {
	return stripePost(tc, "/v1/accounts/"+acct+"/external_accounts?external_account="+token, nil)
}

func TestAdminBankAccountTokenResolvesToRegisteredDetails(t *testing.T) {
	_, tc := setupStripe(t)
	acct := newConnectAccount(t, tc)

	resp := tc.Post("/admin/bank_account_tokens", map[string]any{
		"token": "btok_plaidA1", "bank_name": "First Platypus Bank", "last4": "1111",
		"routing_number": "011401533", "country": "us", "currency": "USD", "account_holder_name": "Acme Merchant",
	})
	resp.AssertStatus(200)
	tok := resp.JSONMap()
	if tok["id"] != "btok_plaidA1" || tok["type"] != "bank_account" || tok["used"] != false {
		t.Errorf("token = %v", tok)
	}

	ea := attachToken(tc, acct, "btok_plaidA1")
	ea.AssertStatus(200)
	got := ea.JSONMap()
	if got["bank_name"] != "First Platypus Bank" || got["last4"] != "1111" || got["routing_number"] != "011401533" ||
		got["country"] != "US" || got["currency"] != "usd" {
		t.Errorf("external account = %v", got)
	}
	attachToken(tc, acct, "btok_plaidA1").AssertStatus(400)
}

func TestAdminBankAccountTokenDefaultsToPlaidSandbox(t *testing.T) {
	_, tc := setupStripe(t)
	acct := newConnectAccount(t, tc)

	tc.Post("/admin/bank_account_tokens", map[string]any{"token": "btok_bare"}).AssertStatus(200)
	got := attachToken(tc, acct, "btok_bare").JSONMap()
	if got["bank_name"] != "STRIPE TEST BANK" || got["last4"] != "6789" || got["routing_number"] != "110000000" ||
		got["country"] != "US" || got["currency"] != "usd" {
		t.Errorf("external account = %v", got)
	}
}

func TestAdminBankAccountTokenRefusesBadRequests(t *testing.T) {
	_, tc := setupStripe(t)
	tc.Post("/admin/bank_account_tokens", map[string]any{"token": "btok_taken"}).AssertStatus(200)

	for name, body := range map[string]map[string]any{
		"missing token":    {},
		"wrong prefix":     {"token": "tok_visa"},
		"already stored":   {"token": "btok_taken"},
		"test token id":    {"token": "btok_us_verified"},
		"last4 not digits": {"token": "btok_x1", "last4": "12a4"},
		"bad country":      {"token": "btok_x2", "country": "USA"},
		"bad currency":     {"token": "btok_x3", "currency": "dollars"},
	} {
		if resp := tc.Post("/admin/bank_account_tokens", body); resp.StatusCode != 400 {
			t.Errorf("%s: status %d, want 400", name, resp.StatusCode)
		}
	}
}

func TestAdminBankAccountTokenFollowsResetAndState(t *testing.T) {
	_, tc := setupStripe(t)
	tc.Post("/admin/bank_account_tokens", map[string]any{"token": "btok_kept", "last4": "4242"}).AssertStatus(200)
	state := tc.Get("/admin/state").JSONMap()

	tc.Post("/admin/reset", nil).AssertStatus(200)
	acct := newConnectAccount(t, tc)
	attachToken(tc, acct, "btok_kept").AssertStatus(400)

	tc.Post("/admin/state", state).AssertStatus(200)
	acct = newConnectAccount(t, tc)
	resp := attachToken(tc, acct, "btok_kept")
	resp.AssertStatus(200)
	if last4 := resp.JSONMap()["last4"]; last4 != "4242" {
		t.Errorf("last4 after state load = %v, want 4242", last4)
	}
}
