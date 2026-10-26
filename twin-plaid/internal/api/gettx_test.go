package api_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

func getTx(tc *testutil.TwinClient, token, start, end string, opts map[string]any) map[string]any {
	b := map[string]any{"access_token": token, "start_date": start, "end_date": end}
	if opts != nil {
		b["options"] = opts
	}
	return tc.Post("/transactions/get", creds(b)).AssertStatus(200).JSONMap()
}

func TestTransactionsGetReturnsTheItemsRowsNewestFirstWithATotal(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	today := time.Now().UTC()
	res := getTx(tc, token, today.AddDate(0, 0, -30).Format("2006-01-02"), today.Format("2006-01-02"), nil)

	rows := list(res, "transactions")
	if len(rows) != 2 || res["total_transactions"] != 2.0 || len(list(res, "accounts")) != 3 || res["item"] == nil || res["request_id"] == nil {
		t.Fatalf("response = %v", res)
	}
	if rows[0].(map[string]any)["date"].(string) < rows[1].(map[string]any)["date"].(string) {
		t.Fatalf("rows are not newest first: %v", rows)
	}
}

func TestTransactionsGetFiltersByDateRangeInclusively(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	tc.Post("/sandbox/transactions/create", creds(map[string]any{"access_token": token, "transactions": []any{
		newTx("old", 1, map[string]any{"date_posted": "2026-01-10", "date_transacted": "2026-01-10"}),
		newTx("older", 1, map[string]any{"date_posted": "2026-01-05", "date_transacted": "2026-01-05"})}})).AssertStatus(200)

	if res := getTx(tc, token, "2026-01-10", "2026-01-10", nil); res["total_transactions"] != 1.0 || list(res, "transactions")[0].(map[string]any)["name"] != "old" {
		t.Fatalf("a one day range is inclusive on both ends: %v", res)
	}
	if res := getTx(tc, token, "2026-01-01", "2026-01-31", nil); res["total_transactions"] != 2.0 {
		t.Fatalf("the January rows = %v", res)
	}
	if res := getTx(tc, token, "2020-01-01", "2020-01-02", nil); res["total_transactions"] != 0.0 || len(list(res, "transactions")) != 0 {
		t.Fatalf("an empty range must be [] and total 0: %v", res)
	}
}

func TestTransactionsGetPagesByCountAndOffset(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	var rows []any
	for i := 0; i < 6; i++ {
		d := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		rows = append(rows, newTx(fmt.Sprintf("row %d", i), float64(i), map[string]any{"date_posted": d, "date_transacted": d}))
	}
	tc.Post("/sandbox/transactions/create", creds(map[string]any{"access_token": token, "transactions": rows})).AssertStatus(200)

	var got []string
	for offset := 0; offset < 8; offset += 3 {
		res := getTx(tc, token, "2020-01-01", "2099-01-01", map[string]any{"count": 3, "offset": offset})
		if res["total_transactions"] != 8.0 {
			t.Fatalf("total changed with the page: %v", res["total_transactions"])
		}
		for _, r := range list(res, "transactions") {
			got = append(got, r.(map[string]any)["transaction_id"].(string))
		}
	}
	seen := map[string]bool{}
	for _, id := range got {
		seen[id] = true
	}
	if len(got) != 8 || len(seen) != 8 {
		t.Fatalf("pages of 3 gave %d rows with %d distinct, want 8 and 8", len(got), len(seen))
	}
	if past := getTx(tc, token, "2020-01-01", "2099-01-01", map[string]any{"offset": 100}); len(list(past, "transactions")) != 0 || past["total_transactions"] != 8.0 {
		t.Fatalf("an offset past the end = %v", past)
	}
}

func TestTransactionsGetAccountIDsFilterAccountsAndRows(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	accounts := accountsOf(tc, token)
	only := func(id any) map[string]any {
		return getTx(tc, token, "2020-01-01", "2099-01-01", map[string]any{"account_ids": []any{id}})
	}
	if res := only(accounts[0]["account_id"]); res["total_transactions"] != 2.0 || len(list(res, "accounts")) != 1 {
		t.Fatalf("the checking account = %v", res)
	}
	if res := only(accounts[1]["account_id"]); res["total_transactions"] != 0.0 || len(list(res, "accounts")) != 1 {
		t.Fatalf("an account with no rows = %v", res)
	}
	m := tc.Post("/transactions/get", creds(map[string]any{"access_token": token, "start_date": "2020-01-01", "end_date": "2099-01-01",
		"options": map[string]any{"account_ids": []any{"nope"}}})).AssertStatus(400).JSONMap()
	if m["error_code"] != "INVALID_ACCOUNT_ID" {
		t.Fatalf("an account the Item lacks = %v", m)
	}
}

func TestTransactionsGetRefusesBadRequests(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	broken, gone := linkedItem(tc), linkedItem(tc)
	tc.Post("/sandbox/item/reset_login", creds(map[string]any{"access_token": broken})).AssertStatus(200)
	tc.Post("/item/remove", creds(map[string]any{"access_token": gone})).AssertStatus(200)
	base := func(extra map[string]any) map[string]any {
		b := map[string]any{"access_token": token, "start_date": "2026-01-01", "end_date": "2026-02-01"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	for name, c := range map[string]struct {
		body map[string]any
		code string
	}{
		"no dates":         {map[string]any{"access_token": token}, "MISSING_FIELDS"},
		"no end date":      {map[string]any{"access_token": token, "start_date": "2026-01-01"}, "MISSING_FIELDS"},
		"bad date":         {base(map[string]any{"start_date": "01/01/2026"}), "INVALID_FIELD"},
		"end before start": {base(map[string]any{"start_date": "2026-03-01"}), "INVALID_FIELD"},
		"count 0":          {base(map[string]any{"options": map[string]any{"count": 0}}), "INVALID_FIELD"},
		"count 501":        {base(map[string]any{"options": map[string]any{"count": 501}}), "INVALID_FIELD"},
		"offset -1":        {base(map[string]any{"options": map[string]any{"offset": -1}}), "INVALID_FIELD"},
		"count as text":    {base(map[string]any{"options": map[string]any{"count": "5"}}), "INVALID_BODY"},
		"date as number":   {base(map[string]any{"end_date": 5}), "INVALID_BODY"},
		"options type":     {base(map[string]any{"options": "x"}), "INVALID_BODY"},
		"login required":   {map[string]any{"access_token": broken, "start_date": "2026-01-01", "end_date": "2026-02-01"}, "ITEM_LOGIN_REQUIRED"},
		"removed":          {map[string]any{"access_token": gone, "start_date": "2026-01-01", "end_date": "2026-02-01"}, "ITEM_NOT_FOUND"},
		"unknown":          {map[string]any{"access_token": "access-sandbox-nope", "start_date": "2026-01-01", "end_date": "2026-02-01"}, "INVALID_ACCESS_TOKEN"},
	} {
		if m := tc.Post("/transactions/get", creds(c.body)).AssertStatus(400).JSONMap(); m["error_code"] != c.code {
			t.Fatalf("%s: %v", name, m)
		}
	}
}
