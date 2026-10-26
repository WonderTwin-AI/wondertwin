package api_test

import (
	"regexp"
	"testing"
)

var (
	processorRE = regexp.MustCompile(`^processor-sandbox-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	btokRE      = regexp.MustCompile(`^btok_[0-9A-Za-z]{24}$`)
)

func TestProcessorTokensAreIssuedForTheCheckingAccountAndRevokedWithTheItem(t *testing.T) {
	tc, _, mem := setupPlaid(t)
	token := linkedItem(tc)
	checking := accountsOf(tc, token)[0]["account_id"]

	seen := map[string]bool{}
	for _, processor := range []string{"dwolla", "adyen", "dwolla"} {
		m := tc.Post("/processor/token/create", creds(map[string]any{"access_token": token, "account_id": checking, "processor": processor})).
			AssertStatus(200).JSONMap()
		tok, _ := m["processor_token"].(string)
		if !processorRE.MatchString(tok) || m["request_id"] == nil || seen[tok] {
			t.Fatalf("processor %s gave %v", processor, m)
		}
		seen[tok] = true
	}
	stripe := tc.Post("/processor/stripe/bank_account_token/create", creds(map[string]any{"access_token": token, "account_id": checking})).
		AssertStatus(200).JSONMap()
	if !btokRE.MatchString(stripe["stripe_bank_account_token"].(string)) {
		t.Fatalf("stripe token = %v", stripe)
	}
	if mem.ProcessorTokens.Count() != 4 {
		t.Fatalf("%d processor tokens stored, want 4", mem.ProcessorTokens.Count())
	}

	tc.Post("/item/remove", creds(map[string]any{"access_token": token})).AssertStatus(200)
	if mem.ProcessorTokens.Count() != 0 {
		t.Fatalf("removing the Item left %d processor tokens, Plaid revokes access with the Item", mem.ProcessorTokens.Count())
	}
}

func TestProcessorTokensRefuseWhatPlaidDoes(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	accounts := accountsOf(tc, token)
	checking, retirement := accounts[0]["account_id"], accounts[1]["account_id"]
	other := accountsOf(tc, linkedItem(tc))[0]["account_id"]

	for name, c := range map[string]struct {
		path string
		body map[string]any
		code string
	}{
		"missing everything":      {"/processor/token/create", map[string]any{}, "MISSING_FIELDS"},
		"missing processor":       {"/processor/token/create", map[string]any{"access_token": token, "account_id": checking}, "MISSING_FIELDS"},
		"unknown processor":       {"/processor/token/create", map[string]any{"access_token": token, "account_id": checking, "processor": "not_a_processor"}, "INVALID_FIELD"},
		"an unknown account":      {"/processor/token/create", map[string]any{"access_token": token, "account_id": "nope", "processor": "dwolla"}, "INVALID_ACCOUNT_ID"},
		"another Item's account":  {"/processor/token/create", map[string]any{"access_token": token, "account_id": other, "processor": "dwolla"}, "INVALID_ACCOUNT_ID"},
		"an account without Auth": {"/processor/token/create", map[string]any{"access_token": token, "account_id": retirement, "processor": "dwolla"}, "NO_AUTH_ACCOUNTS"},
		"stripe: no Auth account": {"/processor/stripe/bank_account_token/create", map[string]any{"access_token": token, "account_id": retirement}, "NO_AUTH_ACCOUNTS"},
		"stripe: missing account": {"/processor/stripe/bank_account_token/create", map[string]any{"access_token": token}, "MISSING_FIELDS"},
		"wrong type":              {"/processor/token/create", map[string]any{"access_token": token, "account_id": 5, "processor": "dwolla"}, "INVALID_BODY"},
		"unknown token":           {"/processor/token/create", map[string]any{"access_token": "access-sandbox-nope", "account_id": checking, "processor": "dwolla"}, "INVALID_ACCESS_TOKEN"},
	} {
		if m := tc.Post(c.path, creds(c.body)).AssertStatus(400).JSONMap(); m["error_code"] != c.code {
			t.Fatalf("%s: %v", name, m)
		}
	}
	m := tc.Post("/processor/token/create", creds(map[string]any{})).JSONMap()
	if m["error_message"] != "the following required fields are missing: access_token, account_id, processor" {
		t.Fatalf("message = %v", m["error_message"])
	}
}

func TestProcessorTokensFollowTheItemsState(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	broken := linkedItem(tc)
	checking := accountsOf(tc, broken)[0]["account_id"]
	tc.Post("/sandbox/item/reset_login", creds(map[string]any{"access_token": broken})).AssertStatus(200)
	if m := tc.Post("/processor/stripe/bank_account_token/create", creds(map[string]any{"access_token": broken, "account_id": checking})).
		AssertStatus(400).JSONMap(); m["error_code"] != "ITEM_LOGIN_REQUIRED" {
		t.Fatalf("login required Item = %v", m)
	}
}
