package api_test

import (
	"testing"
	"time"
)

func daysAgo(n int) string { return time.Now().UTC().AddDate(0, 0, -n).Format("2006-01-02") }

func rowOn(desc string, amount float64, ago int) map[string]any {
	d := daysAgo(ago)
	return map[string]any{"description": desc, "amount": amount, "date_posted": d, "date_transacted": d}
}

func recurring(t *testing.T, tcPost func(string, map[string]any) map[string]any, token string, extra map[string]any) map[string]any {
	t.Helper()
	b := map[string]any{"access_token": token}
	for k, v := range extra {
		b[k] = v
	}
	return tcPost("/transactions/recurring/get", creds(b))
}

func TestRecurringGetFindsStreamsInTheDocumentedShape(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	post := func(path string, b map[string]any) map[string]any {
		return tc.Post(path, b).AssertStatus(200).JSONMap()
	}
	// Plaid takes ten a call.
	tc.Post("/sandbox/transactions/create", creds(map[string]any{"access_token": token, "transactions": []any{
		rowOn("Netflix", 15.99, 21), rowOn("Netflix", 15.99, 14), rowOn("Netflix", 15.99, 7), rowOn("Netflix", 16.99, 0),
		rowOn("ACME PAYROLL", -2500, 60), rowOn("ACME PAYROLL", -2500, 30), rowOn("ACME PAYROLL", -2550, 0),
		rowOn("Gym", 30, 14), rowOn("Gym", 30, 0),
	}})).AssertStatus(200)
	tc.Post("/sandbox/transactions/create", creds(map[string]any{"access_token": token, "transactions": []any{
		rowOn("Coffee", 4, 43), rowOn("Coffee", 4, 40), rowOn("Coffee", 4, 0),
	}})).AssertStatus(200)

	res := recurring(t, post, token, nil)
	out, in := list(res, "outflow_streams"), list(res, "inflow_streams")
	if len(out) != 2 || len(in) != 1 || res["updated_datetime"] == nil || res["personal_finance_category_version"] != "v2" || res["request_id"] == nil {
		t.Fatalf("response = %v", res)
	}
	gym, netflix := out[0].(map[string]any), out[1].(map[string]any)
	if netflix["description"] != "Netflix" || netflix["frequency"] != "WEEKLY" || netflix["status"] != "MATURE" || len(netflix["transaction_ids"].([]any)) != 4 ||
		netflix["first_date"] != daysAgo(21) || netflix["last_date"] != daysAgo(0) || netflix["is_active"] != true || netflix["is_user_modified"] != false {
		t.Fatalf("netflix = %v", netflix)
	}
	if netflix["last_amount"].(map[string]any)["amount"] != 16.99 || netflix["average_amount"].(map[string]any)["amount"] != 16.24 ||
		netflix["last_amount"].(map[string]any)["iso_currency_code"] != "USD" {
		t.Fatalf("netflix amounts = %v %v", netflix["last_amount"], netflix["average_amount"])
	}
	if netflix["predicted_next_date"] != time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02") || len(netflix["stream_id"].(string)) != 37 {
		t.Fatalf("netflix next date or id = %v", netflix)
	}
	if gym["status"] != "EARLY_DETECTION" || gym["frequency"] != "BIWEEKLY" {
		t.Fatalf("two rows fourteen days apart = %v", gym)
	}
	pay := in[0].(map[string]any)
	if pay["frequency"] != "MONTHLY" || pay["last_amount"].(map[string]any)["amount"] != -2550.0 {
		t.Fatalf("payroll = %v", pay)
	}
	for _, k := range []string{"category", "category_id", "merchant_name"} {
		if v, ok := netflix[k]; !ok || v != nil {
			t.Fatalf("%s must be present as null: %v", k, netflix)
		}
	}
	if _, ok := netflix["personal_finance_category"]; ok {
		t.Fatalf("personal_finance_category is optional and not nullable, so it is omitted when no row has one: %v", netflix)
	}
	if again := recurring(t, post, token, nil); list(again, "outflow_streams")[1].(map[string]any)["stream_id"] != netflix["stream_id"] {
		t.Fatal("a stream id must be the same on every call")
	}
}

func TestRecurringGetMarksAStreamThatStoppedInactiveAndFindsNothingWithoutHistory(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	post := func(path string, b map[string]any) map[string]any {
		return tc.Post(path, b).AssertStatus(200).JSONMap()
	}
	if res := recurring(t, post, token, nil); len(list(res, "outflow_streams"))+len(list(res, "inflow_streams")) != 0 {
		t.Fatalf("the two starting rows are not a stream: %v", res)
	}
	tc.Post("/sandbox/transactions/create", creds(map[string]any{"access_token": token, "transactions": []any{
		rowOn("Old gym", 20, 114), rowOn("Old gym", 20, 107), rowOn("Old gym", 20, 100)}})).AssertStatus(200)
	if s := list(recurring(t, post, token, nil), "outflow_streams")[0].(map[string]any); s["is_active"] != false || s["status"] != "MATURE" {
		t.Fatalf("a weekly stream whose last row is 100 days old = %v", s)
	}
}

func TestRecurringGetAccountIDsAndTheItemsState(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := linkedItem(tc)
	accounts := accountsOf(tc, token)
	post := func(path string, b map[string]any) map[string]any { return tc.Post(path, b).JSONMap() }
	tc.Post("/sandbox/transactions/create", creds(map[string]any{"access_token": token, "transactions": []any{
		rowOn("Rent", 900, 60), rowOn("Rent", 900, 30), rowOn("Rent", 900, 0)}})).AssertStatus(200)

	if res := recurring(t, post, token, map[string]any{"account_ids": []any{accounts[0]["account_id"]}}); len(list(res, "outflow_streams")) != 1 {
		t.Fatalf("the checking account = %v", res)
	}
	if res := recurring(t, post, token, map[string]any{"account_ids": []any{accounts[1]["account_id"]}}); len(list(res, "outflow_streams")) != 0 {
		t.Fatalf("an account with no rows = %v", res)
	}
	broken, gone := linkedItem(tc), linkedItem(tc)
	tc.Post("/sandbox/item/reset_login", creds(map[string]any{"access_token": broken})).AssertStatus(200)
	tc.Post("/item/remove", creds(map[string]any{"access_token": gone})).AssertStatus(200)
	for name, c := range map[string]struct {
		extra map[string]any
		tok   string
		code  string
	}{
		"unknown account": {map[string]any{"account_ids": []any{"nope"}}, token, "INVALID_ACCOUNT_ID"},
		"wrong type":      {map[string]any{"account_ids": "x"}, token, "INVALID_BODY"},
		"login required":  {nil, broken, "ITEM_LOGIN_REQUIRED"},
		"removed":         {nil, gone, "ITEM_NOT_FOUND"},
		"unknown token":   {nil, "access-sandbox-nope", "INVALID_ACCESS_TOKEN"},
	} {
		if m := recurring(t, post, c.tok, c.extra); m["error_code"] != c.code {
			t.Fatalf("%s: %v", name, m)
		}
	}
}
