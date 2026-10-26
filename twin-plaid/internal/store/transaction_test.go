package store

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
)

func row(id, date string) Transaction {
	return Transaction{ID: id, ItemID: "item1", AccountID: "acct1", Date: date, Amount: 10,
		Body: TransactionBody{AccountID: "acct1", TransactionID: id, Date: date, PaymentChannel: "other", Name: "row " + id}}
}

func ids(rows []TransactionBody) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.TransactionID)
	}
	sort.Strings(out)
	return out
}

func TestTransactionsComeBackNewestFirstAndGoWithTheItem(t *testing.T) {
	s := seeded(1)
	s.PutItem(Item{ID: "item1"})
	s.AddTransaction(row("b", "2026-10-01"))
	s.AddTransaction(row("a", "2026-10-05"))
	s.AddTransaction(row("c", "2026-10-01"))
	other := row("x", "2026-10-09")
	other.ItemID = "item2"
	s.AddTransaction(other)

	var got []string
	for _, tx := range s.TransactionsForItem("item1") {
		got = append(got, tx.ID)
	}
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	s.RemoveItem("item1")
	if len(s.TransactionsForItem("item1")) != 0 || len(s.TransactionsForItem("item2")) != 1 {
		t.Fatal("removing an Item must remove its transactions and no others")
	}
	if p, _ := s.Sync("item1", "", 10); len(p.Added) != 0 {
		t.Fatal("a removed Item kept its change log")
	}
}

func TestChangeLogSyncFromAnEmptyCursorReportsEveryRowAsAddedThenOnlyWhatChanged(t *testing.T) {
	s := seeded(1)
	s.AddTransaction(row("a", "2026-10-01"))
	s.AddTransaction(row("b", "2026-10-02"))

	first, err := s.Sync("item1", "", 100)
	if err != nil || !reflect.DeepEqual(ids(first.Added), []string{"a", "b"}) || len(first.Modified)+len(first.Removed) != 0 || first.HasMore {
		t.Fatalf("first run = %+v, %v", first, err)
	}
	quiet, _ := s.Sync("item1", first.NextCursor, 100)
	if len(quiet.Added)+len(quiet.Modified)+len(quiet.Removed) != 0 || quiet.HasMore {
		t.Fatalf("nothing changed, got %+v", quiet)
	}

	changed := row("a", "2026-10-01")
	changed.Body.Name = "row a, changed"
	s.ModifyTransaction(changed)
	s.RemoveTransaction("b")
	s.AddTransaction(row("c", "2026-10-03"))

	next, err := s.Sync("item1", quiet.NextCursor, 100)
	if err != nil || !reflect.DeepEqual(ids(next.Added), []string{"c"}) || !reflect.DeepEqual(ids(next.Modified), []string{"a"}) ||
		len(next.Removed) != 1 || next.Removed[0] != (RemovedTransaction{"acct1", "b"}) || next.Modified[0].Name != "row a, changed" {
		t.Fatalf("what changed = %+v, %v", next, err)
	}
}

func TestChangeLogCollapsesARowChangedInsideOneWindow(t *testing.T) {
	s := seeded(1)
	s.AddTransaction(row("a", "2026-10-01"))
	s.AddTransaction(row("b", "2026-10-02"))
	edited := row("a", "2026-10-01")
	edited.Body.Name = "latest"
	s.ModifyTransaction(edited)
	s.RemoveTransaction("b")

	p, _ := s.Sync("item1", "", 100)
	if !reflect.DeepEqual(ids(p.Added), []string{"a"}) || p.Added[0].Name != "latest" || len(p.Modified) != 0 || len(p.Removed) != 0 {
		t.Fatalf("added then changed is one added row with its latest body, added then removed is nothing: %+v", p)
	}
}

func TestChangeLogPagesAreCutFromAFixedWindow(t *testing.T) {
	s := seeded(1)
	for i := 0; i < 7; i++ {
		s.AddTransaction(row(fmt.Sprintf("t%d", i), "2026-10-01"))
	}
	var got []string
	cur, pages := "", 0
	for {
		p, err := s.Sync("item1", cur, 3)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, ids(p.Added)...)
		pages++
		if pages == 1 {
			s.AddTransaction(row("late", "2026-10-02")) // lands while a run is paging
		}
		cur = p.NextCursor
		if !p.HasMore {
			break
		}
	}
	sort.Strings(got)
	if pages != 3 || len(got) != 7 || got[0] != "t0" || got[6] != "t6" {
		t.Fatalf("%d pages gave %v; a change landing mid-run must wait for the next run", pages, got)
	}
	next, _ := s.Sync("item1", cur, 3)
	if !reflect.DeepEqual(ids(next.Added), []string{"late"}) {
		t.Fatalf("the next run = %+v, want the late row", next)
	}
}

func TestChangeLogCursorsFromElsewhereAreRefused(t *testing.T) {
	s := seeded(1)
	s.AddTransaction(row("a", "2026-10-01"))
	for name, cur := range map[string]string{
		"not base64":      "!!!",
		"wrong version":   "djI6MDoxOjA",            // v2:0:1:0
		"too few parts":   "djE6MDox",               // v1:0:1
		"past the log":    cursor{0, 9, 0}.String(), // log has one entry
		"start after end": cursor{2, 1, 0}.String(),
	} {
		if _, err := s.Sync("item1", cur, 10); !errors.Is(err, ErrBadCursor) {
			t.Errorf("%s: error = %v, want ErrBadCursor", name, err)
		}
	}
}

func TestChangeLogSurvivesAStateRoundTripAndResetClearsIt(t *testing.T) {
	s := seeded(1)
	s.AddTransaction(row("a", "2026-10-01"))
	first, _ := s.Sync("item1", "", 10)
	s.AddTransaction(row("b", "2026-10-02"))

	raw, err := jsonMarshal(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	loaded := seeded(2)
	if err := loaded.LoadState(raw); err != nil {
		t.Fatal(err)
	}
	next, err := loaded.Sync("item1", first.NextCursor, 10)
	if err != nil || !reflect.DeepEqual(ids(next.Added), []string{"b"}) {
		t.Fatalf("a cursor from before the round trip = %+v, %v", next, err)
	}
	loaded.Reset()
	if p, _ := loaded.Sync("item1", "", 10); len(p.Added) != 0 || loaded.Transactions.Count() != 0 {
		t.Fatal("Reset left transactions or a change log behind")
	}
}
