package api_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// payoutConfig is a checking account with weekly payout deposits (negative:
// money in) and two expenses, dated back from today on the app emulator's clock.
func payoutConfig(today time.Time) string {
	day := func(ago int) string { return today.AddDate(0, 0, -ago).Format("2006-01-02") }
	var rows []string
	for _, ago := range []int{21, 14, 7, 0} {
		rows = append(rows, fmt.Sprintf(`{"date_transacted": %q, "date_posted": %q, "amount": -1250.5, "description": "STRIPE PAYOUT"}`, day(ago), day(ago)))
	}
	rows = append(rows,
		fmt.Sprintf(`{"date_transacted": %q, "date_posted": %q, "amount": 89.99, "description": "Shopify Plan"}`, day(10), day(9)),
		fmt.Sprintf(`{"date_transacted": %q, "date_posted": %q, "amount": 420, "description": "Office Rent", "currency": "CAD"}`, day(3), day(3)))
	return `{"override_accounts": [
		{"type": "depository", "subtype": "checking", "starting_balance": 10000, "transactions": [` + strings.Join(rows, ",") + `]},
		{"type": "depository", "subtype": "savings", "starting_balance": 500}]}`
}

func TestCustomItemTransactionsSyncReturnsTheConfiguredRowsOnly(t *testing.T) {
	tc, _, mem := setupPlaid(t)
	token := customItem(t, tc, payoutConfig(mem.Clock.Now().UTC()))
	checking := accountsOf(tc, token)[0]["account_id"]

	res := tc.Post("/transactions/sync", creds(map[string]any{"access_token": token})).AssertStatus(200).JSONMap()
	added := res["added"].([]any)
	if len(added) != 6 || res["has_more"] != false {
		t.Fatalf("added %d rows, has_more %v; want the 6 configured rows", len(added), res["has_more"])
	}
	payouts := 0
	for _, a := range added {
		row := a.(map[string]any)
		if row["account_id"] != checking || row["pending"] != false {
			t.Fatalf("row = %v", row)
		}
		switch row["name"] {
		case "STRIPE PAYOUT":
			payouts++
			if row["amount"] != -1250.5 || row["iso_currency_code"] != "USD" || row["date"] != row["authorized_date"] {
				t.Fatalf("payout = %v", row)
			}
		case "Shopify Plan":
			if d, ad := row["date"].(string), row["authorized_date"].(string); d <= ad {
				t.Fatalf("date_posted is date, date_transacted is authorized_date: %v", row)
			}
		case "Office Rent":
			if row["iso_currency_code"] != "CAD" {
				t.Fatalf("a row's own currency wins: %v", row)
			}
		default:
			t.Fatalf("unexpected row %v", row)
		}
	}
	if payouts != 4 {
		t.Fatalf("payouts = %d", payouts)
	}
}

func TestCustomItemTransactionsSyncStartingRowsAreNotReportedByRefresh(t *testing.T) {
	tc, _, mem := setupPlaid(t)
	customItem(t, tc, payoutConfig(mem.Clock.Now().UTC()))
	// What a /transactions/refresh would report: nothing, the rows were there at creation.
	for _, it := range mem.Items.List() {
		if added, total := mem.ChangesSince(it.ID, it.NotifiedChanges); added != 0 || total != 6 {
			t.Fatalf("starting rows are news to a receiver: %d of %d added", added, total)
		}
	}
}

func TestCustomItemTransactionsGetFiltersByDateAndAccount(t *testing.T) {
	tc, _, mem := setupPlaid(t)
	today := mem.Clock.Now().UTC()
	token := customItem(t, tc, payoutConfig(today))
	get := func(start, end time.Time) map[string]any {
		return tc.Post("/transactions/get", creds(map[string]any{"access_token": token,
			"start_date": start.Format("2006-01-02"), "end_date": end.Format("2006-01-02")})).AssertStatus(200).JSONMap()
	}
	all := get(today.AddDate(0, 0, -90), today)
	if all["total_transactions"] != 6.0 || len(all["accounts"].([]any)) != 2 {
		t.Fatalf("total = %v, accounts = %v", all["total_transactions"], all["accounts"])
	}
	if week := get(today.AddDate(0, 0, -6), today); week["total_transactions"] != 2.0 {
		t.Fatalf("last 7 days = %v rows, want this week's payout and the rent", week["total_transactions"])
	}
}

func TestCustomItemRecurringWeeklyDepositsFormAnInflowStream(t *testing.T) {
	tc, _, mem := setupPlaid(t)
	today := mem.Clock.Now().UTC()
	token := customItem(t, tc, payoutConfig(today))
	res := tc.Post("/transactions/recurring/get", creds(map[string]any{"access_token": token})).AssertStatus(200).JSONMap()
	inflow := res["inflow_streams"].([]any)
	if len(inflow) != 1 {
		t.Fatalf("inflow_streams = %v", inflow)
	}
	s := inflow[0].(map[string]any)
	if s["description"] != "STRIPE PAYOUT" || s["frequency"] != "WEEKLY" || s["status"] != "MATURE" || s["is_active"] != true ||
		len(s["transaction_ids"].([]any)) != 4 || s["predicted_next_date"] != today.AddDate(0, 0, 7).Format("2006-01-02") {
		t.Fatalf("stream = %v", s)
	}
}

func TestCustomItemTransactionsWithoutRowsAreEmpty(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := customItem(t, tc, `{"override_accounts": [{"type": "depository", "subtype": "checking"}]}`)
	if added := tc.Post("/transactions/sync", creds(map[string]any{"access_token": token})).AssertStatus(200).JSONMap()["added"].([]any); len(added) != 0 {
		t.Fatalf("a custom Item got fixture rows: %v", added)
	}
}
