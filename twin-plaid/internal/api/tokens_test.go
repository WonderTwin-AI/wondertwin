package api_test

import (
	"regexp"
	"testing"
	"time"
)

var (
	publicRE   = regexp.MustCompile(`^public-sandbox-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	accessRE   = regexp.MustCompile(`^access-sandbox-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	idRE       = regexp.MustCompile(`^[0-9A-Za-z]{37}$`)
	regexpLink = regexp.MustCompile(`^link-sandbox-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

func creds(m map[string]any) map[string]any {
	m["client_id"], m["secret"] = "id", "secret"
	return m
}

func TestSandboxPublicTokenCreateIssuesAPublicToken(t *testing.T) {
	tc, _, mem := setupPlaid(t)

	m := tc.Post("/sandbox/public_token/create", creds(map[string]any{
		"institution_id": "ins_109508", "initial_products": []string{"transactions", "auth"},
	})).AssertStatus(200).JSONMap()
	if tok, _ := m["public_token"].(string); !publicRE.MatchString(tok) {
		t.Fatalf("public_token = %v", m["public_token"])
	}
	if m["request_id"] == nil {
		t.Fatal("no request_id")
	}
	if mem.Items.Count() != 0 {
		t.Fatal("an Item exists before the exchange")
	}
}

func TestSandboxPublicTokenCreateChecksItsFields(t *testing.T) {
	tc, _, _ := setupPlaid(t)

	m := tc.Post("/sandbox/public_token/create", creds(map[string]any{})).AssertStatus(400).JSONMap()
	if m["error_code"] != "MISSING_FIELDS" ||
		m["error_message"] != "the following required fields are missing: institution_id, initial_products" {
		t.Fatalf("body = %v", m)
	}
	for _, bad := range []map[string]any{
		{"institution_id": 5, "initial_products": []string{"auth"}},
		{"institution_id": "ins_1", "initial_products": "auth"},
		{"institution_id": "ins_1", "initial_products": []int{1}},
		{"institution_id": "ins_1", "initial_products": []string{"auth"}, "options": "x"},
		{"institution_id": "ins_1", "initial_products": []string{"auth"}, "options": map[string]any{"webhook": 1}},
	} {
		if m := tc.Post("/sandbox/public_token/create", creds(bad)).AssertStatus(400).JSONMap(); m["error_code"] != "INVALID_BODY" {
			t.Fatalf("%v: %v", bad, m)
		}
	}
}

func TestExchangeMakesTheItemAndAnAccessToken(t *testing.T) {
	tc, _, mem := setupPlaid(t)
	pub := tc.Post("/sandbox/public_token/create", creds(map[string]any{
		"institution_id": "ins_109508", "initial_products": []string{"transactions"},
		"options": map[string]any{"webhook": "http://hooks.test/plaid"},
	})).JSONMap()["public_token"]

	m := tc.Post("/item/public_token/exchange", creds(map[string]any{"public_token": pub})).AssertStatus(200).JSONMap()
	if !accessRE.MatchString(m["access_token"].(string)) || !idRE.MatchString(m["item_id"].(string)) {
		t.Fatalf("body = %v", m)
	}
	it, ok := mem.GetItem(m["item_id"].(string))
	if !ok || it.InstitutionID != "ins_109508" || it.Webhook != "http://hooks.test/plaid" || it.Products[0] != "transactions" {
		t.Fatalf("item = %+v, found %v", it, ok)
	}
	if at, _ := mem.AccessTokens.Get(m["access_token"].(string)); at.ItemID != it.ID {
		t.Fatalf("access token bound to %q, want %q", at.ItemID, it.ID)
	}

	again := tc.Post("/item/public_token/exchange", creds(map[string]any{"public_token": pub})).AssertStatus(400).JSONMap()
	if again["error_code"] != "INVALID_PUBLIC_TOKEN" || again["error_type"] != "INVALID_INPUT" {
		t.Fatalf("second exchange = %v", again)
	}
}

func TestExchangeRefusesUnknownMissingAndExpiredTokens(t *testing.T) {
	tc, _, mem := setupPlaid(t)

	if m := tc.Post("/item/public_token/exchange", creds(map[string]any{"public_token": "public-sandbox-nope"})).
		AssertStatus(400).JSONMap(); m["error_code"] != "INVALID_PUBLIC_TOKEN" {
		t.Fatalf("unknown token = %v", m)
	}
	if m := tc.Post("/item/public_token/exchange", creds(map[string]any{})).AssertStatus(400).JSONMap(); m["error_code"] != "MISSING_FIELDS" {
		t.Fatalf("missing token = %v", m)
	}

	pub := tc.Post("/sandbox/public_token/create", creds(map[string]any{
		"institution_id": "ins_1", "initial_products": []string{"auth"},
	})).JSONMap()["public_token"]
	mem.Clock.Advance(31 * time.Minute)
	if m := tc.Post("/item/public_token/exchange", creds(map[string]any{"public_token": pub})).
		AssertStatus(400).JSONMap(); m["error_code"] != "INVALID_PUBLIC_TOKEN" || mem.Items.Count() != 0 {
		t.Fatalf("expired token = %v, items %d", m, mem.Items.Count())
	}
}
