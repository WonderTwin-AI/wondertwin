package api_test

import (
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

func doSync(tc *testutil.TwinClient, token, cursor string, count int) map[string]any {
	b := map[string]any{"access_token": token}
	if cursor != "" {
		b["cursor"] = cursor
	}
	if count > 0 {
		b["count"] = count
	}
	return tc.Post("/transactions/sync", creds(b)).AssertStatus(200).JSONMap()
}

func list(m map[string]any, k string) []any { return m[k].([]any) }

func TestTransactionsSyncReturnsTheItemsRowsThenOnlyWhatChanged(t *testing.T) {
	tc, _, mem := setupPlaid(t)
	token := linkedItem(tc)
	item, _ := mem.ItemForAccessToken(token)

	first := doSync(tc, token, "", 0)
	if len(list(first, "added")) != 2 || len(list(first, "modified")) != 0 || len(list(first, "removed")) != 0 ||
		first["has_more"] != false || first["transactions_update_status"] != "HISTORICAL_UPDATE_COMPLETE" ||
		len(list(first, "accounts")) != 3 || first["request_id"] == nil || first["next_cursor"] == "" {
		t.Fatalf("first sync = %v", first)
	}
	row := list(first, "added")[0].(map[string]any)
	for _, k := range []string{"transaction_id", "account_id", "amount", "date", "name", "pending", "payment_channel", "personal_finance_category"} {
		if _, ok := row[k]; !ok {
			t.Fatalf("documented field %s missing from a row: %v", k, row)
		}
	}
	if v, ok := row["account_owner"]; !ok || v != nil {
		t.Fatalf("account_owner must be present as null, as in Plaid's sample: %v", row)
	}

	quiet := doSync(tc, token, first["next_cursor"].(string), 0)
	if len(list(quiet, "added"))+len(list(quiet, "modified"))+len(list(quiet, "removed")) != 0 {
		t.Fatalf("nothing changed, got %v", quiet)
	}

	rows := mem.TransactionsForItem(item.ID)
	edited := rows[0]
	edited.Body.Name = "edited"
	mem.ModifyTransaction(edited)
	mem.RemoveTransaction(rows[1].ID)
	next := doSync(tc, token, quiet["next_cursor"].(string), 0)
	removed := list(next, "removed")
	if len(list(next, "modified")) != 1 || list(next, "modified")[0].(map[string]any)["name"] != "edited" || len(removed) != 1 ||
		removed[0].(map[string]any)["transaction_id"] != rows[1].ID || removed[0].(map[string]any)["account_id"] != rows[1].AccountID {
		t.Fatalf("what changed = %v", next)
	}
}

func TestTransactionsSyncPagesWithCountAndHasMore(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	one := doSync(tc, token, "", 1)
	if len(list(one, "added")) != 1 || one["has_more"] != true {
		t.Fatalf("page 1 = %v", one)
	}
	two := doSync(tc, token, one["next_cursor"].(string), 1)
	if len(list(two, "added")) != 1 || two["has_more"] != false || list(one, "added")[0].(map[string]any)["transaction_id"] == list(two, "added")[0].(map[string]any)["transaction_id"] {
		t.Fatalf("page 2 = %v", two)
	}
}

func TestTransactionsSyncNowCursorSkipsHistory(t *testing.T) {
	tc, _, mem := setupPlaid(t)
	token := linkedItem(tc)
	now := doSync(tc, token, "now", 0)
	if len(list(now, "added")) != 0 || now["has_more"] != false || now["next_cursor"] == "" {
		t.Fatalf("now = %v", now)
	}
	item, _ := mem.ItemForAccessToken(token)
	rows := mem.TransactionsForItem(item.ID)
	mem.RemoveTransaction(rows[0].ID)
	if after := doSync(tc, token, now["next_cursor"].(string), 0); len(list(after, "removed")) != 1 {
		t.Fatalf("after now = %v", after)
	}
}

func TestTransactionsSyncRecordsThatSyncWasCalled(t *testing.T) {
	tc, _, mem := setupPlaid(t)
	token := linkedItem(tc)
	if it, _ := mem.ItemForAccessToken(token); it.SyncCalled {
		t.Fatal("a new Item must not have had sync called")
	}
	doSync(tc, token, "", 0)
	if it, _ := mem.ItemForAccessToken(token); !it.SyncCalled {
		t.Fatal("sync was called and the Item does not say so")
	}
}

func TestTransactionsSyncRefusesBadRequests(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	broken, gone := linkedItem(tc), linkedItem(tc)
	tc.Post("/sandbox/item/reset_login", creds(map[string]any{"access_token": broken})).AssertStatus(200)
	tc.Post("/item/remove", creds(map[string]any{"access_token": gone})).AssertStatus(200)

	for name, c := range map[string]struct {
		body map[string]any
		code string
	}{
		"count 0":        {map[string]any{"access_token": token, "count": 0}, "INVALID_FIELD"},
		"count 501":      {map[string]any{"access_token": token, "count": 501}, "INVALID_FIELD"},
		"count text":     {map[string]any{"access_token": token, "count": "5"}, "INVALID_BODY"},
		"cursor garbage": {map[string]any{"access_token": token, "cursor": "not-a-cursor"}, "INVALID_FIELD"},
		"cursor type":    {map[string]any{"access_token": token, "cursor": 5}, "INVALID_BODY"},
		"options type":   {map[string]any{"access_token": token, "options": "x"}, "INVALID_BODY"},
		"missing token":  {map[string]any{}, "MISSING_FIELDS"},
		"login required": {map[string]any{"access_token": broken}, "ITEM_LOGIN_REQUIRED"},
		"removed":        {map[string]any{"access_token": gone}, "ITEM_NOT_FOUND"},
		"unknown":        {map[string]any{"access_token": "access-sandbox-nope"}, "INVALID_ACCESS_TOKEN"},
	} {
		if m := tc.Post("/transactions/sync", creds(c.body)).AssertStatus(400).JSONMap(); m["error_code"] != c.code {
			t.Fatalf("%s: %v", name, m)
		}
	}
	other := doSync(tc, linkedItem(tc), "", 0)
	if m := tc.Post("/transactions/sync", creds(map[string]any{"access_token": token, "cursor": other["next_cursor"]})).JSONMap(); m["error_code"] == "" && m["next_cursor"] == nil {
		t.Fatalf("another Item's cursor = %v", m)
	}
}

func TestAdminRoutesMakeSyncReportModifiedAndRemovedRows(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedWithHook(tc, []string{"transactions"}, "")
	first := doSync(tc, token, "", 0)
	added := list(first, "added")
	if len(added) != 2 {
		t.Fatalf("a new Item should start with two rows, got %v", first)
	}
	keep, drop := added[0].(map[string]any)["transaction_id"].(string), added[1].(map[string]any)["transaction_id"].(string)

	tc.Post("/admin/transactions/"+keep+"/modify", map[string]any{"name": "renamed", "amount": 9.5, "pending": false}).AssertStatus(200)
	tc.Delete("/admin/transactions/" + drop).AssertStatus(200)

	next := doSync(tc, token, first["next_cursor"].(string), 0)
	modified, removed := list(next, "modified"), list(next, "removed")
	if len(modified) != 1 || modified[0].(map[string]any)["name"] != "renamed" || modified[0].(map[string]any)["amount"] != 9.5 ||
		len(removed) != 1 || removed[0].(map[string]any)["transaction_id"] != drop {
		t.Fatalf("what changed = %v", next)
	}

	tc.Post("/admin/transactions/nope/modify", map[string]any{"name": "x"}).AssertStatus(404)
	tc.Delete("/admin/transactions/nope").AssertStatus(404)
	tc.Post("/admin/transactions/"+keep+"/modify", map[string]any{"date": "yesterday"}).AssertStatus(400)
}
