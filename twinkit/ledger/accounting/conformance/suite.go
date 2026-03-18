// Package conformance provides a test suite that validates accounting engine
// invariants. Any twin using twinkit/ledger/accounting must pass this suite.
//
// Usage in a twin's test file:
//
//	func TestAccountingConformance(t *testing.T) {
//	    engine := setupMyEngine() // your twin's engine setup
//	    conformance.Run(t, engine)
//	}
package conformance

import (
	"context"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/ledger/accounting"
	"github.com/wondertwin-ai/wondertwin/twinkit/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/store/journal"
)

// Run executes the full accounting engine conformance suite against the given engine.
// The engine should be freshly created with no pre-existing state.
func Run(t *testing.T, engine *accounting.Engine) {
	t.Helper()
	t.Run("Conformance/AccountManagement", func(t *testing.T) { testAccountManagement(t, engine) })
	t.Run("Conformance/JournalBalancing", func(t *testing.T) { testJournalBalancing(t, engine) })
	t.Run("Conformance/DocumentStateTransitions", func(t *testing.T) { testDocumentStateTransitions(t, engine) })
	t.Run("Conformance/PaymentApplication", func(t *testing.T) { testPaymentApplication(t, engine) })
	t.Run("Conformance/PeriodLocking", func(t *testing.T) { testPeriodLocking(t, engine) })
	t.Run("Conformance/ReportsFromJournal", func(t *testing.T) { testReportsFromJournal(t, engine) })
	t.Run("Conformance/ManualJournal", func(t *testing.T) { testManualJournal(t, engine) })
}

// RunWithFactory creates a fresh engine for each sub-test using the provided factory.
// This is the preferred method — it isolates sub-tests from each other.
func RunWithFactory(t *testing.T, factory func() *accounting.Engine) {
	t.Helper()
	t.Run("Conformance/AccountManagement", func(t *testing.T) { testAccountManagement(t, factory()) })
	t.Run("Conformance/JournalBalancing", func(t *testing.T) { testJournalBalancing(t, factory()) })
	t.Run("Conformance/DocumentStateTransitions", func(t *testing.T) { testDocumentStateTransitions(t, factory()) })
	t.Run("Conformance/PaymentApplication", func(t *testing.T) { testPaymentApplication(t, factory()) })
	t.Run("Conformance/PeriodLocking", func(t *testing.T) { testPeriodLocking(t, factory()) })
	t.Run("Conformance/ReportsFromJournal", func(t *testing.T) { testReportsFromJournal(t, factory()) })
	t.Run("Conformance/ManualJournal", func(t *testing.T) { testManualJournal(t, factory()) })
}

// NewTestEngine creates a standard engine for conformance testing with
// a basic chart of accounts pre-configured.
func NewTestEngine() *accounting.Engine {
	clock := store.NewClock()
	j := journal.New(clock)
	engine := accounting.NewEngine(
		accounting.WithJournal(j),
		accounting.WithClock(clock),
	)

	engine.CreateAccount(accounting.Account{ID: "ar", Name: "Accounts Receivable", Type: accounting.AccountTypeAsset, Class: accounting.ClassCurrentAsset})
	engine.CreateAccount(accounting.Account{ID: "ap", Name: "Accounts Payable", Type: accounting.AccountTypeLiability, Class: accounting.ClassCurrentLiability})
	engine.CreateAccount(accounting.Account{ID: "rev", Name: "Revenue", Type: accounting.AccountTypeRevenue, Class: accounting.ClassRevenue})
	engine.CreateAccount(accounting.Account{ID: "exp", Name: "Expenses", Type: accounting.AccountTypeExpense, Class: accounting.ClassExpense})
	engine.CreateAccount(accounting.Account{ID: "bank", Name: "Bank", Type: accounting.AccountTypeAsset, Class: accounting.ClassCurrentAsset})

	engine.SetReceivableAccount("ar")
	engine.SetPayableAccount("ap")

	return engine
}

func testAccountManagement(t *testing.T, engine *accounting.Engine) {
	// Invariant: accounts can be created and retrieved.
	acct, err := engine.CreateAccount(accounting.Account{
		Name: "Test Account", Type: accounting.AccountTypeAsset, Class: accounting.ClassCurrentAsset,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if acct.ID == "" {
		t.Fatal("account ID should be assigned")
	}

	got, err := engine.GetAccount(acct.ID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got.Name != "Test Account" {
		t.Errorf("Name = %s, want 'Test Account'", got.Name)
	}

	// Invariant: duplicate IDs are rejected.
	_, err = engine.CreateAccount(accounting.Account{ID: acct.ID, Name: "Dup", Type: accounting.AccountTypeAsset})
	if err == nil {
		t.Fatal("duplicate account ID should be rejected")
	}

	// Invariant: missing name is rejected.
	_, err = engine.CreateAccount(accounting.Account{Type: accounting.AccountTypeAsset})
	if err == nil {
		t.Fatal("missing name should be rejected")
	}

	// Invariant: accounts appear in list.
	accts := engine.ListAccounts()
	if len(accts) == 0 {
		t.Fatal("ListAccounts should return at least one account")
	}
}

func testJournalBalancing(t *testing.T, engine *accounting.Engine) {
	// Invariant: journal rejects unbalanced entries.
	j := engine.Journal()

	err := j.Append(journal.Transaction{
		Entries: []journal.Entry{
			{AccountID: "a", Type: journal.Debit, Amount: 100, Currency: "USD"},
			{AccountID: "b", Type: journal.Credit, Amount: 50, Currency: "USD"},
		},
	})
	if err == nil {
		t.Fatal("journal should reject unbalanced transaction")
	}

	// Invariant: journal accepts balanced entries.
	err = j.Append(journal.Transaction{
		Entries: []journal.Entry{
			{AccountID: "a", Type: journal.Debit, Amount: 100, Currency: "USD"},
			{AccountID: "b", Type: journal.Credit, Amount: 100, Currency: "USD"},
		},
	})
	if err != nil {
		t.Fatalf("balanced transaction should be accepted: %v", err)
	}

	// Invariant: balance is computed from entries, not stored.
	d, c, net := j.Balance("a")
	if d != 100 || c != 0 || net != 100 {
		t.Errorf("Balance(a) = d:%d c:%d net:%d, want 100/0/100", d, c, net)
	}

	// Invariant: zero-amount entries are rejected.
	err = j.Append(journal.Transaction{
		Entries: []journal.Entry{
			{AccountID: "a", Type: journal.Debit, Amount: 0, Currency: "USD"},
			{AccountID: "b", Type: journal.Credit, Amount: 0, Currency: "USD"},
		},
	})
	if err == nil {
		t.Fatal("journal should reject zero-amount entries")
	}

	// Invariant: mixed currencies are rejected.
	err = j.Append(journal.Transaction{
		Entries: []journal.Entry{
			{AccountID: "a", Type: journal.Debit, Amount: 100, Currency: "USD"},
			{AccountID: "b", Type: journal.Credit, Amount: 100, Currency: "EUR"},
		},
	})
	if err == nil {
		t.Fatal("journal should reject mixed-currency transaction")
	}
}

func testDocumentStateTransitions(t *testing.T, engine *accounting.Engine) {
	ctx := context.Background()

	doc := &accounting.Document{
		ID: "inv_test", Type: accounting.DocTypeInvoice, Status: accounting.StatusDraft,
		Currency: "USD", Date: time.Now(),
		LineItems: []accounting.LineItem{
			{AccountID: "rev", Quantity: 100, UnitAmount: 10000, Amount: 10000},
		},
		SubTotal: 10000, Total: 10000, AmountDue: 10000,
	}

	// Invariant: valid transitions succeed.
	if err := engine.TransitionDocument(ctx, doc, accounting.StatusSubmitted); err != nil {
		t.Fatalf("DRAFT→SUBMITTED: %v", err)
	}
	if doc.Status != accounting.StatusSubmitted {
		t.Errorf("status = %s, want SUBMITTED", doc.Status)
	}

	if err := engine.TransitionDocument(ctx, doc, accounting.StatusAuthorised); err != nil {
		t.Fatalf("SUBMITTED→AUTHORISED: %v", err)
	}

	// Invariant: AUTHORISED generates journal entries.
	txs := engine.Journal().Transactions()
	if len(txs) == 0 {
		t.Fatal("AUTHORISED should generate journal entries")
	}

	// Invariant: invalid transitions are rejected.
	err := engine.TransitionDocument(ctx, doc, accounting.StatusSubmitted)
	if err == nil {
		t.Fatal("AUTHORISED→SUBMITTED should be rejected")
	}

	// Invariant: AUTHORISED→VOIDED is valid.
	if err := engine.TransitionDocument(ctx, doc, accounting.StatusVoided); err != nil {
		t.Fatalf("AUTHORISED→VOIDED: %v", err)
	}

	// Invariant: terminal states reject all transitions.
	err = engine.TransitionDocument(ctx, doc, accounting.StatusSubmitted)
	if err == nil {
		t.Fatal("VOIDED→SUBMITTED should be rejected (terminal)")
	}
}

func testPaymentApplication(t *testing.T, engine *accounting.Engine) {
	ctx := context.Background()

	doc := &accounting.Document{
		ID: "inv_pay", Type: accounting.DocTypeInvoice, Status: accounting.StatusDraft,
		Currency: "USD", Date: time.Now(),
		LineItems: []accounting.LineItem{
			{AccountID: "rev", Quantity: 100, UnitAmount: 50000, Amount: 50000},
		},
		SubTotal: 50000, Total: 50000, AmountDue: 50000,
	}
	engine.TransitionDocument(ctx, doc, accounting.StatusSubmitted)
	engine.TransitionDocument(ctx, doc, accounting.StatusAuthorised)

	// Invariant: partial payment reduces AmountDue.
	pmt := &accounting.Payment{
		ID: "pmt_1", DocumentID: "inv_pay", DocumentType: accounting.DocTypeInvoice,
		BankAccountID: "bank", Amount: 30000, Currency: "USD",
	}
	if err := engine.ApplyPayment(ctx, pmt, doc); err != nil {
		t.Fatalf("partial payment: %v", err)
	}
	if doc.AmountDue != 20000 {
		t.Errorf("AmountDue after partial = %d, want 20000", doc.AmountDue)
	}
	if doc.Status != accounting.StatusAuthorised {
		t.Errorf("status after partial = %s, want AUTHORISED", doc.Status)
	}

	// Invariant: full payment sets AmountDue to 0 and transitions to PAID.
	pmt2 := &accounting.Payment{
		ID: "pmt_2", DocumentID: "inv_pay", DocumentType: accounting.DocTypeInvoice,
		BankAccountID: "bank", Amount: 20000, Currency: "USD",
	}
	if err := engine.ApplyPayment(ctx, pmt2, doc); err != nil {
		t.Fatalf("full payment: %v", err)
	}
	if doc.AmountDue != 0 {
		t.Errorf("AmountDue after full = %d, want 0", doc.AmountDue)
	}
	if doc.Status != accounting.StatusPaid {
		t.Errorf("status after full = %s, want PAID", doc.Status)
	}

	// Invariant: overpayment is rejected.
	pmt3 := &accounting.Payment{
		ID: "pmt_3", DocumentID: "inv_pay", DocumentType: accounting.DocTypeInvoice,
		BankAccountID: "bank", Amount: 1, Currency: "USD",
	}
	err := engine.ApplyPayment(ctx, pmt3, doc)
	if err == nil {
		t.Fatal("payment on PAID document should be rejected")
	}

	// Invariant: payment on wrong status is rejected.
	draft := &accounting.Document{
		ID: "inv_draft", Type: accounting.DocTypeInvoice, Status: accounting.StatusDraft,
		Currency: "USD", Total: 10000, AmountDue: 10000,
	}
	err = engine.ApplyPayment(ctx, &accounting.Payment{
		ID: "pmt_bad", Amount: 10000, Currency: "USD", BankAccountID: "bank",
	}, draft)
	if err == nil {
		t.Fatal("payment on DRAFT document should be rejected")
	}
}

func testPeriodLocking(t *testing.T, engine *accounting.Engine) {
	ctx := context.Background()
	lockDate := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	engine.LockPeriod(lockDate)

	if engine.PeriodLock() != lockDate {
		t.Fatalf("PeriodLock = %v, want %v", engine.PeriodLock(), lockDate)
	}

	// Invariant: documents in locked period are rejected.
	doc := &accounting.Document{
		ID: "inv_locked", Type: accounting.DocTypeInvoice, Status: accounting.StatusDraft,
		Currency: "USD", Date: time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
		LineItems: []accounting.LineItem{{AccountID: "rev", Quantity: 100, UnitAmount: 100, Amount: 100}},
		SubTotal: 100, Total: 100, AmountDue: 100,
	}
	err := engine.TransitionDocument(ctx, doc, accounting.StatusSubmitted)
	if err == nil {
		t.Fatal("document in locked period should be rejected")
	}

	// Invariant: documents after lock period are accepted.
	doc.Date = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	if err := engine.TransitionDocument(ctx, doc, accounting.StatusSubmitted); err != nil {
		t.Fatalf("document after lock should be accepted: %v", err)
	}

	// Invariant: manual journals in locked period are rejected.
	mj := &accounting.ManualJournal{
		ID: "mj_locked", Date: time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
		Lines: []accounting.ManualJournalLine{
			{AccountID: "bank", DebitAmount: 100},
			{AccountID: "rev", CreditAmount: 100},
		},
	}
	err = engine.PostManualJournal(ctx, mj)
	if err == nil {
		t.Fatal("manual journal in locked period should be rejected")
	}
}

func testReportsFromJournal(t *testing.T, engine *accounting.Engine) {
	ctx := context.Background()

	// Create an authorised invoice to generate journal entries.
	doc := &accounting.Document{
		ID: "inv_report", Type: accounting.DocTypeInvoice, Status: accounting.StatusDraft,
		Currency: "USD", Date: time.Now(),
		LineItems: []accounting.LineItem{
			{AccountID: "rev", Quantity: 100, UnitAmount: 25000, Amount: 25000},
		},
		SubTotal: 25000, Total: 25000, AmountDue: 25000,
	}
	engine.TransitionDocument(ctx, doc, accounting.StatusSubmitted)
	engine.TransitionDocument(ctx, doc, accounting.StatusAuthorised)

	// Invariant: trial balance debits equal credits.
	tb := engine.TrialBalanceReport(time.Time{})
	if tb.TotalDebit != tb.TotalCredit {
		t.Errorf("trial balance unequal: debit=%d credit=%d", tb.TotalDebit, tb.TotalCredit)
	}
	if tb.TotalDebit == 0 {
		t.Error("trial balance should have non-zero totals after authorised invoice")
	}

	// Invariant: P&L reports revenue from invoice.
	from := time.Now().Add(-time.Hour)
	to := time.Now().Add(time.Hour)
	pnl := engine.ProfitAndLossReport(from, to)
	if pnl.Revenue.Total == 0 {
		t.Error("P&L should show revenue after authorised invoice")
	}

	// Invariant: balance sheet shows AR.
	bs := engine.BalanceSheetReport(time.Time{})
	if bs.Assets.Total == 0 {
		t.Error("balance sheet should show AR as asset after authorised invoice")
	}
}

func testManualJournal(t *testing.T, engine *accounting.Engine) {
	ctx := context.Background()

	// Invariant: balanced manual journal is accepted.
	mj := &accounting.ManualJournal{
		ID: "mj_ok", Narration: "Test",
		Lines: []accounting.ManualJournalLine{
			{AccountID: "bank", DebitAmount: 5000},
			{AccountID: "rev", CreditAmount: 5000},
		},
	}
	if err := engine.PostManualJournal(ctx, mj); err != nil {
		t.Fatalf("balanced manual journal: %v", err)
	}
	if mj.Status != "POSTED" {
		t.Errorf("status = %s, want POSTED", mj.Status)
	}

	// Invariant: unbalanced manual journal is rejected.
	mjBad := &accounting.ManualJournal{
		ID: "mj_bad",
		Lines: []accounting.ManualJournalLine{
			{AccountID: "bank", DebitAmount: 5000},
			{AccountID: "rev", CreditAmount: 3000},
		},
	}
	if err := engine.PostManualJournal(ctx, mjBad); err == nil {
		t.Fatal("unbalanced manual journal should be rejected")
	}

	// Invariant: unknown accounts are rejected.
	mjUnk := &accounting.ManualJournal{
		ID: "mj_unk",
		Lines: []accounting.ManualJournalLine{
			{AccountID: "nonexistent", DebitAmount: 1000},
			{AccountID: "rev", CreditAmount: 1000},
		},
	}
	if err := engine.PostManualJournal(ctx, mjUnk); err == nil {
		t.Fatal("manual journal with unknown account should be rejected")
	}
}
