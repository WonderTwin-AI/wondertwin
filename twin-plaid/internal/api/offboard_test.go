package api_test

import "testing"

func TestItemRemoveMakesItsTokenAnswerItemNotFound(t *testing.T) {
	tc, _, mem := setupPlaid(t)
	gone, kept := linkedItem(tc), linkedItem(tc)

	body := tc.Post("/item/remove", creds(map[string]any{"access_token": gone})).AssertStatus(200).JSONMap()
	if len(body) != 1 || body["request_id"] == nil {
		t.Fatalf("remove body = %v, want request_id only", body)
	}
	for _, path := range []string{"/item/get", "/accounts/get", "/item/remove"} {
		m := tc.Post(path, creds(map[string]any{"access_token": gone})).AssertStatus(400).JSONMap()
		if m["error_code"] != "ITEM_NOT_FOUND" || m["error_type"] != "ITEM_ERROR" {
			t.Fatalf("%s after remove = %v", path, m)
		}
	}
	if m := tc.Post("/item/get", creds(map[string]any{"access_token": "access-sandbox-nope"})).JSONMap(); m["error_code"] != "INVALID_ACCESS_TOKEN" {
		t.Fatalf("a never-issued token must stay INVALID_ACCESS_TOKEN, got %v", m)
	}
	if mem.Items.Count() != 1 || mem.Accounts.Count() != 3 || len(accountsOf(tc, kept)) != 3 {
		t.Fatalf("removal reached another Item: items %d, accounts %d", mem.Items.Count(), mem.Accounts.Count())
	}
}

func TestItemAccessTokenInvalidateSwapsTheToken(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	old := linkedItem(tc)
	itemID := tc.Post("/item/get", creds(map[string]any{"access_token": old})).JSONMap()["item"].(map[string]any)["item_id"]

	fresh := tc.Post("/item/access_token/invalidate", creds(map[string]any{"access_token": old})).
		AssertStatus(200).JSONMap()["new_access_token"].(string)
	if !accessRE.MatchString(fresh) || fresh == old {
		t.Fatalf("new_access_token = %q (old %q)", fresh, old)
	}
	if m := tc.Post("/item/get", creds(map[string]any{"access_token": old})).AssertStatus(400).JSONMap(); m["error_code"] != "INVALID_ACCESS_TOKEN" {
		t.Fatalf("old token after invalidate = %v", m)
	}
	got := tc.Post("/item/get", creds(map[string]any{"access_token": fresh})).AssertStatus(200).JSONMap()["item"].(map[string]any)
	if got["item_id"] != itemID || len(accountsOf(tc, fresh)) != 3 {
		t.Fatalf("new token reaches a different Item or lost its accounts: %v", got)
	}
}

func TestItemAccessTokenInvalidateChecksItsToken(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	if m := tc.Post("/item/access_token/invalidate", creds(map[string]any{})).AssertStatus(400).JSONMap(); m["error_code"] != "MISSING_FIELDS" {
		t.Fatalf("missing token = %v", m)
	}
	token := linkedItem(tc)
	tc.Post("/item/remove", creds(map[string]any{"access_token": token})).AssertStatus(200)
	if m := tc.Post("/item/access_token/invalidate", creds(map[string]any{"access_token": token})).AssertStatus(400).JSONMap(); m["error_code"] != "ITEM_NOT_FOUND" {
		t.Fatalf("invalidate after remove = %v", m)
	}
}
