package api_test

import (
	"strings"
	"testing"
)

// customCreate is a /sandbox/public_token/create body for user_custom.
func customCreate(password string) map[string]any {
	return creds(map[string]any{
		"institution_id": "ins_109508", "initial_products": []string{"transactions", "auth"},
		"options": map[string]any{"override_username": "user_custom", "override_password": password},
	})
}

func TestSandboxPublicTokenCreateCustomUserIssuesATokenCarryingTheConfiguration(t *testing.T) {
	tc, _, mem := setupPlaid(t)

	m := tc.Post("/sandbox/public_token/create", customCreate(
		`{"override_accounts": [{"type": "depository", "subtype": "checking", "starting_balance": 5000}]}`)).AssertStatus(200).JSONMap()
	tok, _ := m["public_token"].(string)
	if !publicRE.MatchString(tok) {
		t.Fatalf("public_token = %v", m["public_token"])
	}
	pt, ok := mem.PublicTokens.Get(tok)
	if !ok || pt.CustomUser == nil || *pt.CustomUser.OverrideAccounts[0].StartingBalance != 5000 {
		t.Fatalf("token = %+v", pt)
	}
	if mem.Items.Count() != 0 {
		t.Fatal("an Item exists before the exchange")
	}
}

func TestSandboxPublicTokenCreateCustomUserRefusesABadConfiguration(t *testing.T) {
	tc, _, mem := setupPlaid(t)

	for password, want := range map[string]string{
		"pass_good": "override_password for user_custom must be a JSON object",
		`{"override_accounts": [{"type": "depository", "subtype": "checking", "meta": {"mask": "123456"}}]}`: "override_password for user_custom override_accounts[0].meta.mask must be empty or 2 to 4 letters and digits",
		`{"override_accounts": [{"type": "depository", "subtype": "checking", "transactions": [{"amount": 1}]}]}`: "override_password for user_custom override_accounts[0].transactions[0] is missing date_transacted, date_posted, description",
	} {
		m := tc.Post("/sandbox/public_token/create", customCreate(password)).AssertStatus(400).JSONMap()
		if m["error_type"] != "INVALID_REQUEST" || m["error_code"] != "INVALID_FIELD" || m["error_message"] != want {
			t.Errorf("%s: body = %v", password, m)
		}
	}
	if mem.PublicTokens.Count() != 0 {
		t.Fatal("a refused configuration issued a token")
	}
}

func TestSandboxPublicTokenCreateCustomUserKeepsErrorPasswordsForUserGoodOnly(t *testing.T) {
	tc, _, _ := setupPlaid(t)

	// error_{CODE} is a configuration problem for user_custom, not a forced error (quirk LIFECYCLE-005).
	m := tc.Post("/sandbox/public_token/create", customCreate("error_ITEM_LOCKED")).AssertStatus(400).JSONMap()
	if m["error_code"] != "INVALID_FIELD" || !strings.HasPrefix(m["error_message"].(string), "override_password for user_custom") {
		t.Fatalf("body = %v", m)
	}
	// Other usernames are as before: the password is not read.
	other := creds(map[string]any{"institution_id": "ins_109508", "initial_products": []string{"auth"},
		"options": map[string]any{"override_username": "user_other", "override_password": "{not json"}})
	tc.Post("/sandbox/public_token/create", other).AssertStatus(200)
}
