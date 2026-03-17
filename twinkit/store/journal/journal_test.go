package journal

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/store"
)

func newTestJournal() *Journal {
	return New(store.NewClock())
}

func balancedTx(debitAcct, creditAcct, currency string, amount int64) Transaction {
	return Transaction{
		Entries: []Entry{
			{AccountID: debitAcct, Type: Debit, Amount: amount, Currency: currency, SourceType: "test", SourceID: "t1"},
			{AccountID: creditAcct, Type: Credit, Amount: amount, Currency: currency, SourceType: "test", SourceID: "t1"},
		},
	}
}

func TestAppend_Balanced(t *testing.T) {
	j := newTestJournal()
	err := j.Append(balancedTx("acct_ar", "acct_rev", "USD", 10000))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	txs := j.Transactions()
	if len(txs) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txs))
	}
	if txs[0].ID != "txn_000001" {
		t.Errorf("expected txn_000001, got %s", txs[0].ID)
	}
	if len(txs[0].Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(txs[0].Entries))
	}
}

func TestAppend_Unbalanced(t *testing.T) {
	j := newTestJournal()
	err := j.Append(Transaction{
		Entries: []Entry{
			{AccountID: "a", Type: Debit, Amount: 100, Currency: "USD"},
			{AccountID: "b", Type: Credit, Amount: 50, Currency: "USD"},
		},
	})
	if err == nil {
		t.Fatal("expected error for unbalanced transaction")
	}
}

func TestAppend_ZeroAmount(t *testing.T) {
	j := newTestJournal()
	err := j.Append(Transaction{
		Entries: []Entry{
			{AccountID: "a", Type: Debit, Amount: 0, Currency: "USD"},
			{AccountID: "b", Type: Credit, Amount: 0, Currency: "USD"},
		},
	})
	if err == nil {
		t.Fatal("expected error for zero amount")
	}
}

func TestAppend_NegativeAmount(t *testing.T) {
	j := newTestJournal()
	err := j.Append(Transaction{
		Entries: []Entry{
			{AccountID: "a", Type: Debit, Amount: -100, Currency: "USD"},
			{AccountID: "b", Type: Credit, Amount: -100, Currency: "USD"},
		},
	})
	if err == nil {
		t.Fatal("expected error for negative amount")
	}
}

func TestAppend_MixedCurrency(t *testing.T) {
	j := newTestJournal()
	err := j.Append(Transaction{
		Entries: []Entry{
			{AccountID: "a", Type: Debit, Amount: 100, Currency: "USD"},
			{AccountID: "b", Type: Credit, Amount: 100, Currency: "EUR"},
		},
	})
	if err == nil {
		t.Fatal("expected error for mixed currency")
	}
}

func TestAppend_TooFewEntries(t *testing.T) {
	j := newTestJournal()
	err := j.Append(Transaction{
		Entries: []Entry{
			{AccountID: "a", Type: Debit, Amount: 100, Currency: "USD"},
		},
	})
	if err == nil {
		t.Fatal("expected error for single entry")
	}
}

func TestAppend_InvalidType(t *testing.T) {
	j := newTestJournal()
	err := j.Append(Transaction{
		Entries: []Entry{
			{AccountID: "a", Type: "invalid", Amount: 100, Currency: "USD"},
			{AccountID: "b", Type: Credit, Amount: 100, Currency: "USD"},
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid entry type")
	}
}

func TestAppend_MissingAccountID(t *testing.T) {
	j := newTestJournal()
	err := j.Append(Transaction{
		Entries: []Entry{
			{AccountID: "", Type: Debit, Amount: 100, Currency: "USD"},
			{AccountID: "b", Type: Credit, Amount: 100, Currency: "USD"},
		},
	})
	if err == nil {
		t.Fatal("expected error for missing account_id")
	}
}

func TestAppend_MissingCurrency(t *testing.T) {
	j := newTestJournal()
	err := j.Append(Transaction{
		Entries: []Entry{
			{AccountID: "a", Type: Debit, Amount: 100, Currency: ""},
			{AccountID: "b", Type: Credit, Amount: 100, Currency: ""},
		},
	})
	if err == nil {
		t.Fatal("expected error for missing currency")
	}
}

func TestAppend_Immutability(t *testing.T) {
	j := newTestJournal()
	tx := balancedTx("a", "b", "USD", 500)
	if err := j.Append(tx); err != nil {
		t.Fatal(err)
	}

	// Mutating the original should not affect the journal.
	tx.Entries[0].Amount = 9999
	txs := j.Transactions()
	if txs[0].Entries[0].Amount != 500 {
		t.Error("journal entry was mutated after append")
	}
}

func TestBalance(t *testing.T) {
	j := newTestJournal()

	// Invoice: debit AR, credit Revenue
	if err := j.Append(balancedTx("acct_ar", "acct_rev", "USD", 10000)); err != nil {
		t.Fatal(err)
	}
	// Payment: debit Bank, credit AR
	if err := j.Append(balancedTx("acct_bank", "acct_ar", "USD", 10000)); err != nil {
		t.Fatal(err)
	}

	// AR: debited 10000, credited 10000 → net 0
	d, c, net := j.Balance("acct_ar")
	if d != 10000 || c != 10000 || net != 0 {
		t.Errorf("AR balance: debit=%d credit=%d net=%d, want 10000/10000/0", d, c, net)
	}

	// Revenue: only credited 10000 → net -10000
	d, c, net = j.Balance("acct_rev")
	if d != 0 || c != 10000 || net != -10000 {
		t.Errorf("Revenue balance: debit=%d credit=%d net=%d, want 0/10000/-10000", d, c, net)
	}

	// Bank: only debited 10000 → net 10000
	d, c, net = j.Balance("acct_bank")
	if d != 10000 || c != 0 || net != 10000 {
		t.Errorf("Bank balance: debit=%d credit=%d net=%d, want 10000/0/10000", d, c, net)
	}

	// Unknown account → all zeros
	d, c, net = j.Balance("nonexistent")
	if d != 0 || c != 0 || net != 0 {
		t.Errorf("unknown account balance: debit=%d credit=%d net=%d, want 0/0/0", d, c, net)
	}
}

func TestBalanceAt(t *testing.T) {
	clock := store.NewClock()
	j := New(clock)

	if err := j.Append(balancedTx("a", "b", "USD", 100)); err != nil {
		t.Fatal(err)
	}
	t1 := clock.Now()

	clock.Advance(time.Hour)
	if err := j.Append(balancedTx("a", "b", "USD", 200)); err != nil {
		t.Fatal(err)
	}

	// Balance at t1 should only include the first transaction.
	d, _, _ := j.BalanceAt("a", t1)
	if d != 100 {
		t.Errorf("BalanceAt(t1) debit=%d, want 100", d)
	}

	// Full balance should include both.
	d, _, _ = j.Balance("a")
	if d != 300 {
		t.Errorf("Balance debit=%d, want 300", d)
	}
}

func TestEntries(t *testing.T) {
	j := newTestJournal()
	if err := j.Append(balancedTx("a", "b", "USD", 100)); err != nil {
		t.Fatal(err)
	}
	if err := j.Append(balancedTx("a", "c", "USD", 200)); err != nil {
		t.Fatal(err)
	}

	entries := j.Entries("a")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for 'a', got %d", len(entries))
	}
	if entries[0].Amount != 100 || entries[1].Amount != 200 {
		t.Error("entries not in expected order")
	}

	entries = j.Entries("b")
	if len(entries) != 1 || entries[0].Amount != 100 {
		t.Error("unexpected entries for 'b'")
	}
}

func TestEntriesBetween(t *testing.T) {
	clock := store.NewClock()
	j := New(clock)

	if err := j.Append(balancedTx("a", "b", "USD", 100)); err != nil {
		t.Fatal(err)
	}
	t1 := clock.Now()

	clock.Advance(2 * time.Hour)
	if err := j.Append(balancedTx("a", "b", "USD", 200)); err != nil {
		t.Fatal(err)
	}
	t2 := clock.Now()

	clock.Advance(2 * time.Hour)
	if err := j.Append(balancedTx("a", "b", "USD", 300)); err != nil {
		t.Fatal(err)
	}

	// Only the middle transaction.
	entries := j.EntriesBetween("a", t1.Add(time.Hour), t2)
	if len(entries) != 1 || entries[0].Amount != 200 {
		t.Errorf("expected 1 entry (200), got %d entries", len(entries))
	}
}

func TestMultiLegTransaction(t *testing.T) {
	j := newTestJournal()
	// 3-leg transaction: split revenue across two accounts.
	err := j.Append(Transaction{
		Entries: []Entry{
			{AccountID: "ar", Type: Debit, Amount: 1000, Currency: "USD"},
			{AccountID: "rev_product", Type: Credit, Amount: 700, Currency: "USD"},
			{AccountID: "rev_service", Type: Credit, Amount: 300, Currency: "USD"},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	d, c, net := j.Balance("ar")
	if net != 1000 {
		t.Errorf("AR net=%d, want 1000 (d=%d c=%d)", net, d, c)
	}
	_, _, net = j.Balance("rev_product")
	if net != -700 {
		t.Errorf("rev_product net=%d, want -700", net)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	j := newTestJournal()
	if err := j.Append(balancedTx("a", "b", "USD", 500)); err != nil {
		t.Fatal(err)
	}
	if err := j.Append(balancedTx("c", "d", "USD", 300)); err != nil {
		t.Fatal(err)
	}

	snap := j.Snapshot()
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}

	j2 := newTestJournal()
	if err := j2.LoadSnapshot(data); err != nil {
		t.Fatalf("load snapshot: %v", err)
	}

	// Verify same state.
	txs := j2.Transactions()
	if len(txs) != 2 {
		t.Fatalf("expected 2 transactions after load, got %d", len(txs))
	}

	d1, c1, _ := j.Balance("a")
	d2, c2, _ := j2.Balance("a")
	if d1 != d2 || c1 != c2 {
		t.Error("balance mismatch after snapshot round-trip")
	}

	// New append should use correct counters.
	if err := j2.Append(balancedTx("e", "f", "USD", 100)); err != nil {
		t.Fatal(err)
	}
	txs = j2.Transactions()
	if txs[2].ID != "txn_000003" {
		t.Errorf("expected txn_000003, got %s", txs[2].ID)
	}
}

func TestReset(t *testing.T) {
	j := newTestJournal()
	if err := j.Append(balancedTx("a", "b", "USD", 100)); err != nil {
		t.Fatal(err)
	}
	j.Reset()

	if len(j.Transactions()) != 0 {
		t.Error("expected 0 transactions after reset")
	}
	d, c, net := j.Balance("a")
	if d != 0 || c != 0 || net != 0 {
		t.Error("expected zero balance after reset")
	}

	// Counters should reset too.
	if err := j.Append(balancedTx("a", "b", "USD", 50)); err != nil {
		t.Fatal(err)
	}
	txs := j.Transactions()
	if txs[0].ID != "txn_000001" {
		t.Errorf("expected txn_000001 after reset, got %s", txs[0].ID)
	}
}

func TestConcurrentAppend(t *testing.T) {
	j := newTestJournal()
	var wg sync.WaitGroup
	n := 100
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = j.Append(balancedTx("a", "b", "USD", 10))
		}()
	}
	wg.Wait()

	txs := j.Transactions()
	if len(txs) != n {
		t.Errorf("expected %d transactions, got %d", n, len(txs))
	}

	d, _, _ := j.Balance("a")
	if d != int64(n*10) {
		t.Errorf("expected debit %d, got %d", n*10, d)
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	j := newTestJournal()
	var wg sync.WaitGroup

	// Writers.
	wg.Add(50)
	for i := 0; i < 50; i++ {
		go func() {
			defer wg.Done()
			_ = j.Append(balancedTx("a", "b", "USD", 10))
		}()
	}

	// Concurrent readers.
	wg.Add(50)
	for i := 0; i < 50; i++ {
		go func() {
			defer wg.Done()
			j.Balance("a")
			j.Entries("a")
			j.Transactions()
		}()
	}

	wg.Wait()
}

func TestIDAssignment(t *testing.T) {
	j := newTestJournal()
	if err := j.Append(balancedTx("a", "b", "USD", 100)); err != nil {
		t.Fatal(err)
	}

	txs := j.Transactions()
	if txs[0].Entries[0].ID != "je_000001" {
		t.Errorf("first entry ID = %s, want je_000001", txs[0].Entries[0].ID)
	}
	if txs[0].Entries[1].ID != "je_000002" {
		t.Errorf("second entry ID = %s, want je_000002", txs[0].Entries[1].ID)
	}
	if txs[0].Entries[0].TransactionID != "txn_000001" {
		t.Errorf("entry transaction_id = %s, want txn_000001", txs[0].Entries[0].TransactionID)
	}
}

func TestSourceFields(t *testing.T) {
	j := newTestJournal()
	err := j.Append(Transaction{
		Entries: []Entry{
			{AccountID: "ar", Type: Debit, Amount: 1000, Currency: "USD", SourceType: "invoice", SourceID: "inv_001"},
			{AccountID: "rev", Type: Credit, Amount: 1000, Currency: "USD", SourceType: "invoice", SourceID: "inv_001"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	entries := j.Entries("ar")
	if entries[0].SourceType != "invoice" || entries[0].SourceID != "inv_001" {
		t.Errorf("source fields not preserved: type=%s id=%s", entries[0].SourceType, entries[0].SourceID)
	}
}
