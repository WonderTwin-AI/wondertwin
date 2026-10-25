package api_test

import (
	"encoding/json"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

// linkedWithHook links an Item with the given products and webhook URL.
func linkedWithHook(tc *testutil.TwinClient, products []string, url string) string {
	opts := map[string]any{}
	if url != "" {
		opts["webhook"] = url
	}
	pub := tc.Post("/sandbox/public_token/create", creds(map[string]any{
		"institution_id": "ins_109508", "initial_products": products, "options": opts,
	})).JSONMap()["public_token"]
	return tc.Post("/item/public_token/exchange", creds(map[string]any{"public_token": pub})).JSONMap()["access_token"].(string)
}

func fire(tc *testutil.TwinClient, token string, extra map[string]any) *testutil.Response {
	b := map[string]any{"access_token": token}
	for k, v := range extra {
		b[k] = v
	}
	return tc.Post("/sandbox/item/fire_webhook", creds(b))
}

func delivered(t *testing.T, got chan received) map[string]any {
	t.Helper()
	r := next(t, got)
	if r.headers.Get("Plaid-Verification") == "" {
		t.Fatal("the webhook was not signed")
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(r.body), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestFireWebhookDeliversTheDocumentedBodyForEachSupportedCode(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	url, got := receiver(t)
	token := linkedWithHook(tc, []string{"auth", "transactions"}, url)
	accounts := map[string]bool{}
	for _, a := range accountsOf(tc, token) {
		accounts[a["account_id"].(string)] = true
	}

	for _, c := range []struct{ kind, code string }{
		{"", "NEW_ACCOUNTS_AVAILABLE"}, {"", "LOGIN_REPAIRED"}, {"", "PENDING_DISCONNECT"}, {"", "USER_PERMISSION_REVOKED"},
		{"", "USER_ACCOUNT_REVOKED"}, {"", "DEFAULT_UPDATE"}, {"TRANSACTIONS", "DEFAULT_UPDATE"}, {"", "SYNC_UPDATES_AVAILABLE"},
		{"", "RECURRING_TRANSACTIONS_UPDATE"}, {"AUTH", "DEFAULT_UPDATE"}, {"ITEM", "LOGIN_REPAIRED"},
	} {
		extra := map[string]any{"webhook_code": c.code}
		if c.kind != "" {
			extra["webhook_type"] = c.kind
		}
		if m := fire(tc, token, extra).AssertStatus(200).JSONMap(); m["webhook_fired"] != true || m["request_id"] == nil {
			t.Fatalf("%v: %v", c, m)
		}
		body := delivered(t, got)
		wantType := map[string]string{"": "", "TRANSACTIONS": "TRANSACTIONS", "AUTH": "AUTH", "ITEM": "ITEM"}[c.kind]
		if wantType != "" && body["webhook_type"] != wantType || body["webhook_code"] != c.code ||
			body["environment"] != "sandbox" || body["item_id"] == nil {
			t.Fatalf("%v: body = %v", c, body)
		}
		if _, ok := body["user_id"]; ok {
			t.Fatalf("%v: user_id is only for user-based Items: %v", c, body)
		}
		switch c.code {
		case "USER_ACCOUNT_REVOKED":
			if !accounts[body["account_id"].(string)] {
				t.Fatalf("account_id %v is not one of the Item's accounts", body["account_id"])
			}
		case "RECURRING_TRANSACTIONS_UPDATE":
			if ids := body["account_ids"].([]any); len(ids) != 3 || !accounts[ids[0].(string)] {
				t.Fatalf("account_ids = %v", ids)
			}
		}
	}
}

func TestFireWebhookDefaultUpdateTakesNewTransactionsAndTheDefaultTypeIsTransactions(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	url, got := receiver(t)
	token := linkedWithHook(tc, []string{"transactions"}, url)

	fire(tc, token, map[string]any{"webhook_code": "DEFAULT_UPDATE"}).AssertStatus(200)
	if b := delivered(t, got); b["webhook_type"] != "TRANSACTIONS" || b["new_transactions"] != 0.0 {
		t.Fatalf("default = %v", b)
	}
	fire(tc, token, map[string]any{"webhook_code": "DEFAULT_UPDATE", "new_transactions": 7}).AssertStatus(200)
	if b := delivered(t, got); b["new_transactions"] != 7.0 {
		t.Fatalf("new_transactions = %v", b["new_transactions"])
	}
}

func TestFireWebhookRefusesWhatItCannotFire(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	url, _ := receiver(t)
	withHook := linkedWithHook(tc, []string{"auth"}, url)
	noHook := linkedWithHook(tc, []string{"transactions"}, "")

	for name, c := range map[string]struct {
		token string
		extra map[string]any
		typ   string
		code  string
	}{
		"no webhook URL":             {noHook, map[string]any{"webhook_code": "SYNC_UPDATES_AVAILABLE"}, "SANDBOX_ERROR", "SANDBOX_WEBHOOK_INVALID"},
		"product not on the Item":    {withHook, map[string]any{"webhook_code": "SYNC_UPDATES_AVAILABLE"}, "SANDBOX_ERROR", "SANDBOX_PRODUCT_NOT_ENABLED"},
		"an Assets webhook":          {withHook, map[string]any{"webhook_code": "PRODUCT_READY", "webhook_type": "ASSETS"}, "INVALID_REQUEST", "INVALID_FIELD"},
		"a code with no sample":      {withHook, map[string]any{"webhook_code": "AUTHORIZATION_GRANTED"}, "INVALID_REQUEST", "INVALID_FIELD"},
		"identity default update":    {withHook, map[string]any{"webhook_code": "DEFAULT_UPDATE", "webhook_type": "IDENTITY"}, "INVALID_REQUEST", "INVALID_FIELD"},
		"new_transactions elsewhere": {withHook, map[string]any{"webhook_code": "LOGIN_REPAIRED", "new_transactions": 3}, "INVALID_REQUEST", "INVALID_FIELD"},
		"negative new_transactions":  {noHook, map[string]any{"webhook_code": "DEFAULT_UPDATE", "new_transactions": -1}, "INVALID_REQUEST", "INVALID_FIELD"},
		"missing code":               {withHook, map[string]any{}, "INVALID_REQUEST", "MISSING_FIELDS"},
		"unknown token":              {"access-sandbox-nope", map[string]any{"webhook_code": "LOGIN_REPAIRED"}, "INVALID_INPUT", "INVALID_ACCESS_TOKEN"},
	} {
		m := fire(tc, c.token, c.extra).AssertStatus(400).JSONMap()
		if m["error_type"] != c.typ || m["error_code"] != c.code {
			t.Fatalf("%s: %v", name, m)
		}
	}
	if m := fire(tc, withHook, map[string]any{"webhook_code": "SYNC_UPDATES_AVAILABLE"}).JSONMap(); m["error_message"] != "The transactions product is not enabled on this item" {
		t.Fatalf("message = %v", m["error_message"])
	}
}

func TestResetLoginFiresAnItemErrorWebhookWhenTheItemHasAURL(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	url, got := receiver(t)
	token := linkedWithHook(tc, []string{"auth"}, url)

	tc.Post("/sandbox/item/reset_login", creds(map[string]any{"access_token": token})).AssertStatus(200)
	body := delivered(t, got)
	e, _ := body["error"].(map[string]any)
	if body["webhook_type"] != "ITEM" || body["webhook_code"] != "ERROR" || e["error_code"] != "ITEM_LOGIN_REQUIRED" ||
		e["error_type"] != "ITEM_ERROR" || e["status"] != 400.0 {
		t.Fatalf("body = %v", body)
	}
	if _, ok := e["request_id"]; ok {
		t.Fatalf("a webhook error carries no request_id: %v", e)
	}
}
