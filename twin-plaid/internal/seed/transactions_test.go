package seed

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

func TestDefaultTransactionsAreThePublishedRowsReDatedWithNewIDs(t *testing.T) {
	now := time.Date(2026, 10, 26, 15, 30, 0, 0, time.UTC)
	n := 0
	newID := func() string { n++; return fmt.Sprintf("id-%d", n) }
	rows := DefaultTransactions("item1", "acct1", newID, now)
	if len(rows) != 2 {
		t.Fatalf("%d rows, want the two published ones", len(rows))
	}
	dateRE := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	for _, r := range rows {
		if r.ItemID != "item1" || r.AccountID != "acct1" || r.Body.AccountID != "acct1" || r.Body.TransactionID != r.ID || !dateRE.MatchString(r.Date) {
			t.Fatalf("row = %+v", r)
		}
	}
	if rows[0].Date != "2026-10-25" {
		t.Fatalf("the newest row is dated %s, want the day before now (2026-10-25)", rows[0].Date)
	}
	if rows[0].Body.Name != "Dd Doordash Burgerkin" || rows[0].Body.MerchantName.V == nil || *rows[0].Body.MerchantName.V != "Burger King" || rows[0].Amount != 28.34 || !rows[0].Pending {
		t.Fatalf("the published content changed: %+v", rows[0])
	}
	if dt, _ := time.Parse(time.RFC3339, *rows[0].Body.Datetime); dt.Format("2006-01-02") != rows[0].Date {
		t.Fatalf("datetime %v does not move with date %s", *rows[0].Body.Datetime, rows[0].Date)
	}
	again := DefaultTransactions("item2", "acct2", newID, now)
	if again[0].ID == rows[0].ID || rows[0].Body.Name != again[0].Body.Name {
		t.Fatal("two Items must get distinct ids and independent copies of the same content")
	}
}

func TestCustomTransactionPendingFromDatePosted(t *testing.T) {
	now := time.Date(2026, 11, 4, 23, 30, 0, 0, time.UTC)
	amount := func(v float64) *float64 { return &v }
	cfg := &store.CustomUser{OverrideAccounts: []store.CustomAccount{{Type: "depository", Subtype: "checking",
		Transactions: []store.CustomUserTransaction{
			{DateTransacted: "2026-11-03", DatePosted: "2026-11-03", Amount: amount(10), Description: "past"},
			{DateTransacted: "2026-11-04", DatePosted: "2026-11-04", Amount: amount(11), Description: "today"},
			{DateTransacted: "2026-11-04", DatePosted: "2026-11-05", Amount: amount(12), Description: "posts tomorrow"},
		}}}}
	accounts := CustomAccounts(cfg)
	accounts[0].ID = "acc"
	n := 0
	rows := CustomTransactions("item", accounts, cfg, func() string { n++; return fmt.Sprint("tx", n) }, now)
	want := map[string]bool{"past": false, "today": false, "posts tomorrow": true}
	if len(rows) != 3 {
		t.Fatalf("rows = %v", rows)
	}
	for _, r := range rows {
		if r.Pending != want[r.Body.Name] || r.Body.Pending != r.Pending || r.AccountID != "acc" || *r.Body.IsoCurrencyCode != "USD" {
			t.Errorf("%s: pending %v (body %v), account %s", r.Body.Name, r.Pending, r.Body.Pending, r.AccountID)
		}
	}
}

func TestRollDatesForwardAnchorsTheLatestTransactionDateOnToday(t *testing.T) {
	amount := func(v float64) *float64 { return &v }
	in := &store.CustomUser{RollDatesForward: true, OverrideAccounts: []store.CustomAccount{
		{Type: "depository", Subtype: "checking", Transactions: []store.CustomUserTransaction{
			{DateTransacted: "2025-01-01", DatePosted: "2025-01-01", Amount: amount(1), Description: "first"},
			{DateTransacted: "2025-03-31", DatePosted: "2025-04-02", Amount: amount(2), Description: "latest, posts two days on"}}},
		{Type: "depository", Subtype: "savings", Transactions: []store.CustomUserTransaction{
			{DateTransacted: "2025-03-01", DatePosted: "2025-03-01", Amount: amount(3), Description: "other account"}}},
	}}
	out := RollDatesForward(in, time.Date(2026, 11, 4, 23, 59, 0, 0, time.UTC))
	got := [][2]string{}
	for _, a := range out.OverrideAccounts {
		for _, tx := range a.Transactions {
			got = append(got, [2]string{tx.DateTransacted, tx.DatePosted})
		}
	}
	// 2025-03-31 to 2026-11-04 is 583 days; every date moves by that, spacing kept.
	want := [][2]string{{"2026-08-07", "2026-08-07"}, {"2026-11-04", "2026-11-06"}, {"2026-10-05", "2026-10-05"}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("rolled = %v, want %v", got, want)
	}
	if in.OverrideAccounts[0].Transactions[1].DateTransacted != "2025-03-31" {
		t.Fatal("the stored configuration was changed in place")
	}

	// A rolled row posted after today is pending, decided from the shifted dates.
	accounts := CustomAccounts(out)
	rows := CustomTransactions("item", accounts, out, func() string { return "id" }, time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC))
	for _, r := range rows {
		if r.Pending != (r.Body.Name == "latest, posts two days on") {
			t.Errorf("%s: pending = %v", r.Body.Name, r.Pending)
		}
	}
}

func TestRollDatesForwardLeavesAConfigurationWithoutTheFlagOrRowsAlone(t *testing.T) {
	amount := func(v float64) *float64 { return &v }
	off := &store.CustomUser{OverrideAccounts: []store.CustomAccount{{Type: "depository", Subtype: "checking",
		Transactions: []store.CustomUserTransaction{{DateTransacted: "2025-01-01", DatePosted: "2025-01-01", Amount: amount(1), Description: "x"}}}}}
	if RollDatesForward(off, time.Now()) != off {
		t.Fatal("without roll_dates_forward the dates must be kept as given")
	}
	empty := &store.CustomUser{RollDatesForward: true, OverrideAccounts: []store.CustomAccount{{Type: "depository", Subtype: "checking"}}}
	if RollDatesForward(empty, time.Now()) != empty {
		t.Fatal("a configuration without rows has nothing to roll")
	}
}
