package api_test

import (
	"strings"
	"testing"
)

func TestCreateBankAccountToken(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/tokens?bank_account[account_number]=000123456789&bank_account[country]=us"+
		"&bank_account[routing_number]=110000000&bank_account[account_holder_name]=Jenny+Rosen"+
		"&bank_account[account_holder_type]=individual", nil)
	resp.AssertStatus(200)
	tok := resp.JSONMap()
	if id, _ := tok["id"].(string); !strings.HasPrefix(id, "btok_") || tok["type"] != "bank_account" || tok["used"] != false {
		t.Fatalf("token = %v", tok)
	}
	ba, _ := tok["bank_account"].(map[string]any)
	if id, _ := ba["id"].(string); !strings.HasPrefix(id, "ba_") || ba["bank_name"] != "STRIPE TEST BANK" ||
		ba["last4"] != "6789" || ba["routing_number"] != "110000000" || ba["country"] != "US" ||
		ba["currency"] != "usd" || ba["account_holder_name"] != "Jenny Rosen" ||
		ba["account_holder_type"] != "individual" || ba["status"] != "new" {
		t.Errorf("bank_account = %v", ba)
	}

	// The token stands for the same account as its raw numbers.
	acct := newConnectAccount(t, tc)
	viaToken := attachToken(tc, acct, tok["id"].(string))
	viaToken.AssertStatus(200)
	if fp := viaToken.JSONMap()["fingerprint"]; fp != ba["fingerprint"] {
		t.Errorf("attached fingerprint %v, token fingerprint %v", fp, ba["fingerprint"])
	}
}

func TestCreateBankAccountTokenRequiresAccountNumberAndCountry(t *testing.T) {
	_, tc := setupStripe(t)
	for query, param := range map[string]string{
		"bank_account[country]=US":                  "bank_account[account_number]",
		"bank_account[account_number]=000123456789": "bank_account[country]",
	} {
		resp := stripePost(tc, "/v1/tokens?"+query, nil)
		resp.AssertStatus(400)
		e, _ := resp.JSONMap()["error"].(map[string]any)
		if e["code"] != "parameter_missing" || e["param"] != param {
			t.Errorf("%s: error = %v, want parameter_missing on %s", query, e, param)
		}
	}
}

func TestGetTokenShowsBankAccountAndUse(t *testing.T) {
	_, tc := setupStripe(t)
	created := stripePost(tc, "/v1/tokens?bank_account[account_number]=000111111116&bank_account[country]=US"+
		"&bank_account[routing_number]=110000000", nil).JSONMap()["id"].(string)
	tc.Post("/admin/bank_account_tokens", map[string]any{"token": "btok_registered", "bank_name": "First Platypus Bank"}).
		AssertStatus(200)
	acct := newConnectAccount(t, tc)

	for id, bank := range map[string]string{created: "STRIPE TEST BANK", "btok_registered": "First Platypus Bank"} {
		before := stripeGet(tc, "/v1/tokens/"+id)
		before.AssertStatus(200)
		ba, _ := before.JSONMap()["bank_account"].(map[string]any)
		if ba["bank_name"] != bank || before.JSONMap()["used"] != false {
			t.Errorf("%s before use: %v", id, before.JSONMap())
		}
		attachToken(tc, acct, id).AssertStatus(200)
		if used := stripeGet(tc, "/v1/tokens/"+id).JSONMap()["used"]; used != true {
			t.Errorf("%s used = %v after attaching, want true", id, used)
		}
	}
}
