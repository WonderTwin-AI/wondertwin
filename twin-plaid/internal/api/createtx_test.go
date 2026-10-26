package api_test

import (
	"testing"
	"time"
)

func newTx(desc string, amount float64, extra map[string]any) map[string]any {
	today := time.Now().UTC().Format("2006-01-02")
	m := map[string]any{"description": desc, "amount": amount, "date_posted": today, "date_transacted": today}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestSandboxTransactionsCreateAddsRowsThatSyncReportsAsAdded(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	accounts := accountsOf(tc, token)
	checking := accounts[0]["account_id"]
	start := doSync(tc, token, "", 0)

	res := tc.Post("/sandbox/transactions/create", creds(map[string]any{"access_token": token, "transactions": []any{
		newTx("Tim Hortons", 100.5, nil),
		newTx("BestBuy", -25.75, map[string]any{"iso_currency_code": "CAD", "account_id": checking, "date_transacted": "2026-01-02", "date_posted": "2026-01-03"}),
	}})).AssertStatus(200).JSONMap()
	if len(res) != 1 || res["request_id"] == nil {
		t.Fatalf("create returned %v, want request_id only", res)
	}

	added := list(doSync(tc, token, start["next_cursor"].(string), 0), "added")
	if len(added) != 2 {
		t.Fatalf("sync reported %d rows, want the 2 created", len(added))
	}
	byName := map[string]map[string]any{}
	for _, a := range added {
		row := a.(map[string]any)
		byName[row["name"].(string)] = row
	}
	hort, best := byName["Tim Hortons"], byName["BestBuy"]
	if hort["amount"] != 100.5 || hort["iso_currency_code"] != "USD" || hort["account_id"] != checking || hort["pending"] != false ||
		len(hort["transaction_id"].(string)) != 37 {
		t.Fatalf("Tim Hortons = %v", hort)
	}
	if best["amount"] != -25.75 || best["iso_currency_code"] != "CAD" || best["date"] != "2026-01-03" || best["authorized_date"] != "2026-01-02" {
		t.Fatalf("BestBuy = %v", best)
	}
	for _, k := range []string{"merchant_name", "logo_url", "website", "datetime", "authorized_datetime", "pending_transaction_id"} {
		if v, ok := hort[k]; !ok || v != nil {
			t.Fatalf("%s must be present as null for a custom transaction: %v", k, hort)
		}
	}
	for _, k := range []string{"personal_finance_category", "personal_finance_category_icon_url"} {
		if _, ok := hort[k]; ok {
			t.Fatalf("%s is optional and not nullable, so it must be omitted, never null: %v", k, hort)
		}
	}
	if c, ok := hort["counterparties"].([]any); !ok || len(c) != 0 || hort["location"].(map[string]any)["city"] != nil {
		t.Fatalf("no merchant data should be invented: %v", hort)
	}
	if hort["name"] == "PURCHASE WM SUPERCENTER #1700" || hort["payment_channel"] != "other" {
		t.Fatalf("the sample's content leaked into a created row: %v", hort)
	}
}

func TestSandboxTransactionsCreateWhilePagingWaitsForTheNextRun(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	tc.Post("/sandbox/transactions/create", creds(map[string]any{"access_token": token, "transactions": []any{
		newTx("one", 1, nil), newTx("two", 2, nil), newTx("three", 3, nil)}})).AssertStatus(200)

	page1 := doSync(tc, token, "", 2)
	tc.Post("/sandbox/transactions/create", creds(map[string]any{"access_token": token, "transactions": []any{newTx("late", 9, nil)}})).AssertStatus(200)
	seen := len(list(page1, "added"))
	cursor := page1["next_cursor"].(string)
	for more := page1["has_more"].(bool); more; {
		p := doSync(tc, token, cursor, 2)
		seen, cursor, more = seen+len(list(p, "added")), p["next_cursor"].(string), p["has_more"].(bool)
	}
	if seen != 5 {
		t.Fatalf("the run saw %d rows, want the 5 that existed when it started", seen)
	}
	next := list(doSync(tc, token, cursor, 0), "added")
	if len(next) != 1 || next[0].(map[string]any)["name"] != "late" {
		t.Fatalf("the next run = %v, want the late row", next)
	}
}

func TestSandboxTransactionsCreateRefusesBadRequests(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	accounts := accountsOf(tc, token)
	other := accountsOf(tc, linkedItem(tc))[0]["account_id"]
	today := time.Now().UTC()
	tooMany := make([]any, 11)
	for i := range tooMany {
		tooMany[i] = newTx("x", 1, nil)
	}
	broken, gone := linkedItem(tc), linkedItem(tc)
	tc.Post("/sandbox/item/reset_login", creds(map[string]any{"access_token": broken})).AssertStatus(200)
	tc.Post("/item/remove", creds(map[string]any{"access_token": gone})).AssertStatus(200)

	for name, c := range map[string]struct {
		body map[string]any
		code string
	}{
		"no transactions": {map[string]any{"access_token": token}, "MISSING_FIELDS"},
		"empty list":      {map[string]any{"access_token": token, "transactions": []any{}}, "MISSING_FIELDS"},
		"eleven":          {map[string]any{"access_token": token, "transactions": tooMany}, "INVALID_FIELD"},
		"missing fields":  {map[string]any{"access_token": token, "transactions": []any{map[string]any{"amount": 1}}}, "MISSING_FIELDS"},
		"wrong type":      {map[string]any{"access_token": token, "transactions": []any{newTx("x", 1, map[string]any{"description": 5})}}, "INVALID_BODY"},
		"amount as text":  {map[string]any{"access_token": token, "transactions": []any{newTx("x", 1, map[string]any{"amount": "1"})}}, "INVALID_BODY"},
		"future date":     {map[string]any{"access_token": token, "transactions": []any{newTx("x", 1, map[string]any{"date_posted": today.AddDate(0, 0, 1).Format("2006-01-02")})}}, "INVALID_FIELD"},
		"older than 730d": {map[string]any{"access_token": token, "transactions": []any{newTx("x", 1, map[string]any{"date_transacted": today.AddDate(0, 0, -731).Format("2006-01-02")})}}, "INVALID_FIELD"},
		"not a date":      {map[string]any{"access_token": token, "transactions": []any{newTx("x", 1, map[string]any{"date_posted": "yesterday"})}}, "INVALID_FIELD"},
		"student loan":    {map[string]any{"access_token": token, "transactions": []any{newTx("x", 1, map[string]any{"account_id": accounts[2]["account_id"]})}}, "INVALID_ACCOUNT_ID"},
		"unknown account": {map[string]any{"access_token": token, "transactions": []any{newTx("x", 1, map[string]any{"account_id": "nope"})}}, "INVALID_ACCOUNT_ID"},
		"another Items":   {map[string]any{"access_token": token, "transactions": []any{newTx("x", 1, map[string]any{"account_id": other})}}, "INVALID_ACCOUNT_ID"},
		"login required":  {map[string]any{"access_token": broken, "transactions": []any{newTx("x", 1, nil)}}, "ITEM_LOGIN_REQUIRED"},
		"removed Item":    {map[string]any{"access_token": gone, "transactions": []any{newTx("x", 1, nil)}}, "ITEM_NOT_FOUND"},
	} {
		if m := tc.Post("/sandbox/transactions/create", creds(c.body)).AssertStatus(400).JSONMap(); m["error_code"] != c.code {
			t.Fatalf("%s: %v", name, m)
		}
	}
	if got := len(list(doSync(tc, token, "", 0), "added")); got != 2 {
		t.Fatalf("a refused create changed the Item: %d rows, want the 2 it started with", got)
	}
}

func TestTransactionsRefreshAcknowledgesAndFollowsTheItemsState(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	if m := tc.Post("/transactions/refresh", creds(map[string]any{"access_token": token})).AssertStatus(200).JSONMap(); len(m) != 1 || m["request_id"] == nil {
		t.Fatalf("refresh = %v, want request_id only", m)
	}
	broken := linkedItem(tc)
	tc.Post("/sandbox/item/reset_login", creds(map[string]any{"access_token": broken})).AssertStatus(200)
	for tok, code := range map[string]string{broken: "ITEM_LOGIN_REQUIRED", "access-sandbox-nope": "INVALID_ACCESS_TOKEN", "": "MISSING_FIELDS"} {
		b := map[string]any{}
		if tok != "" {
			b["access_token"] = tok
		}
		if m := tc.Post("/transactions/refresh", creds(b)).AssertStatus(400).JSONMap(); m["error_code"] != code {
			t.Fatalf("%q: %v", tok, m)
		}
	}
}
