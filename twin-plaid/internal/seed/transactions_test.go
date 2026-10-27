package seed

import (
	"fmt"
	"regexp"
	"testing"
	"time"
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
