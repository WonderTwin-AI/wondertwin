package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/state"
	"github.com/wondertwin-ai/wondertwin/twinkit/state/journal"
)

// testHooks records hook invocations for testing.
type testHooks struct {
	NoOpAccountingHooks
	journalEntries [][]JournalEntryInfo
	transitions    []transitionRecord
	payments       []paymentRecord
}

type transitionRecord struct {
	DocID string
	From  DocumentStatus
	To    DocumentStatus
}

type paymentRecord struct {
	PaymentID string
	DocID     string
}

func (h *testHooks) OnJournalEntryCreated(_ context.Context, doc *Document, entries []JournalEntryInfo) error {
	h.journalEntries = append(h.journalEntries, entries)
	return nil
}

func (h *testHooks) OnDocumentStateTransition(_ context.Context, doc *Document, from, to DocumentStatus) error {
	h.transitions = append(h.transitions, transitionRecord{DocID: doc.ID, From: from, To: to})
	return nil
}

func (h *testHooks) OnPaymentApplied(_ context.Context, pmt *Payment, doc *Document) error {
	h.payments = append(h.payments, paymentRecord{PaymentID: pmt.ID, DocID: doc.ID})
	return nil
}

func setupEngine() (*Engine, *testHooks, *state.Clock) {
	clock := state.NewClock()
	hooks := &testHooks{}
	j := journal.New(clock)
	e := NewEngine(WithJournal(j), WithHooks(hooks), WithClock(clock))

	// Set up chart of accounts.
	e.CreateAccount(Account{ID: "acct_ar", Name: "Accounts Receivable", Type: AccountTypeAsset, Class: ClassCurrentAsset, Currency: "USD"})
	e.CreateAccount(Account{ID: "acct_ap", Name: "Accounts Payable", Type: AccountTypeLiability, Class: ClassCurrentLiability, Currency: "USD"})
	e.CreateAccount(Account{ID: "acct_rev", Name: "Sales Revenue", Type: AccountTypeRevenue, Class: ClassRevenue, Currency: "USD"})
	e.CreateAccount(Account{ID: "acct_exp", Name: "Office Expenses", Type: AccountTypeExpense, Class: ClassExpense, Currency: "USD"})
	e.CreateAccount(Account{ID: "acct_bank", Name: "Business Bank", Type: AccountTypeAsset, Class: ClassCurrentAsset, Currency: "USD"})

	e.SetReceivableAccount("acct_ar")
	e.SetPayableAccount("acct_ap")

	return e, hooks, clock
}

func makeInvoice(id, currency string, amount int64) *Document {
	return &Document{
		ID:       id,
		Type:     DocTypeInvoice,
		Status:   StatusDraft,
		Currency: currency,
		Date:     time.Now(),
		LineItems: []LineItem{
			{Description: "Widget", AccountID: "acct_rev", Quantity: 100, UnitAmount: amount},
		},
	}
}

func makeBill(id, currency string, amount int64) *Document {
	return &Document{
		ID:       id,
		Type:     DocTypeBill,
		Status:   StatusDraft,
		Currency: currency,
		Date:     time.Now(),
		LineItems: []LineItem{
			{Description: "Office supplies", AccountID: "acct_exp", Quantity: 100, UnitAmount: amount},
		},
	}
}

// --- State Transition Tests ---

func TestTransition_DraftToSubmitted(t *testing.T) {
	e, hooks, _ := setupEngine()
	inv := makeInvoice("inv_001", "USD", 10000)
	ComputeLineItemTotals(inv)

	if err := e.TransitionDocument(context.Background(), inv, StatusSubmitted); err != nil {
		t.Fatal(err)
	}
	if inv.Status != StatusSubmitted {
		t.Errorf("status = %s, want SUBMITTED", inv.Status)
	}
	if len(hooks.transitions) != 1 || hooks.transitions[0].To != StatusSubmitted {
		t.Error("expected transition hook to fire")
	}
}

func TestTransition_SubmittedToAuthorised(t *testing.T) {
	e, hooks, _ := setupEngine()
	inv := makeInvoice("inv_001", "USD", 10000)
	ComputeLineItemTotals(inv)

	e.TransitionDocument(context.Background(), inv, StatusSubmitted)
	if err := e.TransitionDocument(context.Background(), inv, StatusAuthorised); err != nil {
		t.Fatal(err)
	}
	if inv.Status != StatusAuthorised {
		t.Errorf("status = %s, want AUTHORISED", inv.Status)
	}
	// Should have generated journal entries.
	if len(hooks.journalEntries) != 1 {
		t.Fatalf("expected 1 journal entry set, got %d", len(hooks.journalEntries))
	}
	// AmountDue should be set.
	if inv.AmountDue != inv.Total {
		t.Errorf("AmountDue = %d, want %d", inv.AmountDue, inv.Total)
	}
}

func TestTransition_InvalidTransition(t *testing.T) {
	e, _, _ := setupEngine()
	inv := makeInvoice("inv_001", "USD", 10000)
	ComputeLineItemTotals(inv)

	// Can't go from DRAFT to AUTHORISED.
	if err := e.TransitionDocument(context.Background(), inv, StatusAuthorised); err == nil {
		t.Fatal("expected error for invalid transition DRAFT→AUTHORISED")
	}

	// Can't go from DRAFT to PAID.
	if err := e.TransitionDocument(context.Background(), inv, StatusPaid); err == nil {
		t.Fatal("expected error for invalid transition DRAFT→PAID")
	}
}

func TestTransition_TerminalStates(t *testing.T) {
	e, _, _ := setupEngine()

	tests := []struct {
		name string
		from DocumentStatus
	}{
		{"PAID is terminal", StatusPaid},
		{"VOIDED is terminal", StatusVoided},
		{"DELETED is terminal", StatusDeleted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := &Document{ID: "d", Status: tt.from, Currency: "USD"}
			if err := e.TransitionDocument(context.Background(), doc, StatusSubmitted); err == nil {
				t.Errorf("expected error transitioning from terminal state %s", tt.from)
			}
		})
	}
}

func TestTransition_AuthorisedToVoided(t *testing.T) {
	e, _, _ := setupEngine()
	inv := makeInvoice("inv_001", "USD", 10000)
	ComputeLineItemTotals(inv)

	e.TransitionDocument(context.Background(), inv, StatusSubmitted)
	e.TransitionDocument(context.Background(), inv, StatusAuthorised)
	if err := e.TransitionDocument(context.Background(), inv, StatusVoided); err != nil {
		t.Fatalf("expected AUTHORISED→VOIDED to be valid: %v", err)
	}
}

func TestTransition_DraftToDeleted(t *testing.T) {
	e, _, _ := setupEngine()
	inv := makeInvoice("inv_001", "USD", 10000)
	ComputeLineItemTotals(inv)

	if err := e.TransitionDocument(context.Background(), inv, StatusDeleted); err != nil {
		t.Fatalf("expected DRAFT→DELETED to be valid: %v", err)
	}
}

// --- Invoice Journal Entry Tests ---

func TestInvoiceAuthorised_JournalEntries(t *testing.T) {
	e, _, _ := setupEngine()
	inv := makeInvoice("inv_001", "USD", 50000)
	ComputeLineItemTotals(inv)
	ctx := context.Background()

	e.TransitionDocument(ctx, inv, StatusSubmitted)
	e.TransitionDocument(ctx, inv, StatusAuthorised)

	// AR should be debited.
	d, _, _ := e.journal.Balance("acct_ar")
	if d != 50000 {
		t.Errorf("AR debit = %d, want 50000", d)
	}

	// Revenue should be credited.
	_, c, _ := e.journal.Balance("acct_rev")
	if c != 50000 {
		t.Errorf("Revenue credit = %d, want 50000", c)
	}
}

func TestBillAuthorised_JournalEntries(t *testing.T) {
	e, _, _ := setupEngine()
	bill := makeBill("bill_001", "USD", 20000)
	ComputeLineItemTotals(bill)
	ctx := context.Background()

	e.TransitionDocument(ctx, bill, StatusSubmitted)
	e.TransitionDocument(ctx, bill, StatusAuthorised)

	// Expense should be debited.
	d, _, _ := e.journal.Balance("acct_exp")
	if d != 20000 {
		t.Errorf("Expense debit = %d, want 20000", d)
	}

	// AP should be credited.
	_, c, _ := e.journal.Balance("acct_ap")
	if c != 20000 {
		t.Errorf("AP credit = %d, want 20000", c)
	}
}

// --- Payment Tests ---

func TestPayment_FullPayment(t *testing.T) {
	e, hooks, _ := setupEngine()
	inv := makeInvoice("inv_001", "USD", 10000)
	ComputeLineItemTotals(inv)
	ctx := context.Background()

	e.TransitionDocument(ctx, inv, StatusSubmitted)
	e.TransitionDocument(ctx, inv, StatusAuthorised)

	pmt := &Payment{
		ID:            "pmt_001",
		DocumentID:    "inv_001",
		DocumentType:  DocTypeInvoice,
		BankAccountID: "acct_bank",
		Amount:        10000,
		Currency:      "USD",
	}

	if err := e.ApplyPayment(ctx, pmt, inv); err != nil {
		t.Fatal(err)
	}

	if inv.Status != StatusPaid {
		t.Errorf("status = %s, want PAID", inv.Status)
	}
	if inv.AmountDue != 0 {
		t.Errorf("AmountDue = %d, want 0", inv.AmountDue)
	}

	// Bank should be debited.
	d, _, _ := e.journal.Balance("acct_bank")
	if d != 10000 {
		t.Errorf("Bank debit = %d, want 10000", d)
	}

	// AR should have credit from payment (and debit from invoice).
	d2, c2, net := e.journal.Balance("acct_ar")
	if net != 0 {
		t.Errorf("AR net = %d, want 0 (d=%d c=%d)", net, d2, c2)
	}

	if len(hooks.payments) != 1 {
		t.Error("expected payment hook to fire")
	}
}

func TestPayment_PartialPayment(t *testing.T) {
	e, _, _ := setupEngine()
	inv := makeInvoice("inv_001", "USD", 10000)
	ComputeLineItemTotals(inv)
	ctx := context.Background()

	e.TransitionDocument(ctx, inv, StatusSubmitted)
	e.TransitionDocument(ctx, inv, StatusAuthorised)

	pmt := &Payment{
		ID: "pmt_001", DocumentID: "inv_001", DocumentType: DocTypeInvoice,
		BankAccountID: "acct_bank", Amount: 6000, Currency: "USD",
	}

	if err := e.ApplyPayment(ctx, pmt, inv); err != nil {
		t.Fatal(err)
	}

	if inv.Status != StatusAuthorised {
		t.Errorf("status = %s, want AUTHORISED (partial payment)", inv.Status)
	}
	if inv.AmountDue != 4000 {
		t.Errorf("AmountDue = %d, want 4000", inv.AmountDue)
	}

	// Second partial payment to complete.
	pmt2 := &Payment{
		ID: "pmt_002", DocumentID: "inv_001", DocumentType: DocTypeInvoice,
		BankAccountID: "acct_bank", Amount: 4000, Currency: "USD",
	}
	if err := e.ApplyPayment(ctx, pmt2, inv); err != nil {
		t.Fatal(err)
	}
	if inv.Status != StatusPaid {
		t.Errorf("status = %s, want PAID after full payment", inv.Status)
	}
}

func TestPayment_ExceedsAmountDue(t *testing.T) {
	e, _, _ := setupEngine()
	inv := makeInvoice("inv_001", "USD", 10000)
	ComputeLineItemTotals(inv)
	ctx := context.Background()

	e.TransitionDocument(ctx, inv, StatusSubmitted)
	e.TransitionDocument(ctx, inv, StatusAuthorised)

	pmt := &Payment{
		ID: "pmt_001", DocumentID: "inv_001", DocumentType: DocTypeInvoice,
		BankAccountID: "acct_bank", Amount: 20000, Currency: "USD",
	}

	if err := e.ApplyPayment(ctx, pmt, inv); err == nil {
		t.Fatal("expected error for payment exceeding amount due")
	}
}

func TestPayment_WrongStatus(t *testing.T) {
	e, _, _ := setupEngine()
	inv := makeInvoice("inv_001", "USD", 10000)
	ComputeLineItemTotals(inv)

	pmt := &Payment{
		ID: "pmt_001", DocumentID: "inv_001", DocumentType: DocTypeInvoice,
		BankAccountID: "acct_bank", Amount: 10000, Currency: "USD",
	}

	if err := e.ApplyPayment(context.Background(), pmt, inv); err == nil {
		t.Fatal("expected error for payment on DRAFT document")
	}
}

func TestPayment_BillPayment(t *testing.T) {
	e, _, _ := setupEngine()
	bill := makeBill("bill_001", "USD", 5000)
	ComputeLineItemTotals(bill)
	ctx := context.Background()

	e.TransitionDocument(ctx, bill, StatusSubmitted)
	e.TransitionDocument(ctx, bill, StatusAuthorised)

	pmt := &Payment{
		ID: "pmt_001", DocumentID: "bill_001", DocumentType: DocTypeBill,
		BankAccountID: "acct_bank", Amount: 5000, Currency: "USD",
	}

	if err := e.ApplyPayment(ctx, pmt, bill); err != nil {
		t.Fatal(err)
	}

	// AP should be debited (payment reduces liability).
	d, _, _ := e.journal.Balance("acct_ap")
	if d != 5000 {
		t.Errorf("AP debit = %d, want 5000", d)
	}

	// Bank should be credited (money leaving).
	_, c, _ := e.journal.Balance("acct_bank")
	if c != 5000 {
		t.Errorf("Bank credit = %d, want 5000", c)
	}
}

// --- Period Lock Tests ---

func TestPeriodLock_RejectsOldDocuments(t *testing.T) {
	e, _, _ := setupEngine()
	lockDate := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	e.LockPeriod(lockDate)

	inv := makeInvoice("inv_001", "USD", 10000)
	inv.Date = time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC) // before lock
	ComputeLineItemTotals(inv)

	if err := e.TransitionDocument(context.Background(), inv, StatusSubmitted); err == nil {
		t.Fatal("expected error for document in locked period")
	}
}

func TestPeriodLock_AllowsNewDocuments(t *testing.T) {
	e, _, _ := setupEngine()
	lockDate := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	e.LockPeriod(lockDate)

	inv := makeInvoice("inv_001", "USD", 10000)
	inv.Date = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC) // after lock
	ComputeLineItemTotals(inv)

	if err := e.TransitionDocument(context.Background(), inv, StatusSubmitted); err != nil {
		t.Fatalf("expected no error for document after lock: %v", err)
	}
}

func TestPeriodLock_RejectsManualJournal(t *testing.T) {
	e, _, _ := setupEngine()
	lockDate := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	e.LockPeriod(lockDate)

	mj := &ManualJournal{
		ID:   "mj_001",
		Date: time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
		Lines: []ManualJournalLine{
			{AccountID: "acct_bank", DebitAmount: 1000},
			{AccountID: "acct_rev", CreditAmount: 1000},
		},
	}

	if err := e.PostManualJournal(context.Background(), mj); err == nil {
		t.Fatal("expected error for manual journal in locked period")
	}
}

// --- Manual Journal Tests ---

func TestManualJournal_Balanced(t *testing.T) {
	e, _, _ := setupEngine()
	mj := &ManualJournal{
		ID:        "mj_001",
		Narration: "Adjustment",
		Lines: []ManualJournalLine{
			{AccountID: "acct_bank", DebitAmount: 5000},
			{AccountID: "acct_rev", CreditAmount: 5000},
		},
	}

	if err := e.PostManualJournal(context.Background(), mj); err != nil {
		t.Fatal(err)
	}
	if mj.Status != "POSTED" {
		t.Errorf("status = %s, want POSTED", mj.Status)
	}

	d, _, _ := e.journal.Balance("acct_bank")
	if d != 5000 {
		t.Errorf("Bank debit = %d, want 5000", d)
	}
}

func TestManualJournal_Unbalanced(t *testing.T) {
	e, _, _ := setupEngine()
	mj := &ManualJournal{
		ID: "mj_001",
		Lines: []ManualJournalLine{
			{AccountID: "acct_bank", DebitAmount: 5000},
			{AccountID: "acct_rev", CreditAmount: 3000},
		},
	}

	if err := e.PostManualJournal(context.Background(), mj); err == nil {
		t.Fatal("expected error for unbalanced manual journal")
	}
}

func TestManualJournal_UnknownAccount(t *testing.T) {
	e, _, _ := setupEngine()
	mj := &ManualJournal{
		ID: "mj_001",
		Lines: []ManualJournalLine{
			{AccountID: "nonexistent", DebitAmount: 1000},
			{AccountID: "acct_rev", CreditAmount: 1000},
		},
	}

	if err := e.PostManualJournal(context.Background(), mj); err == nil {
		t.Fatal("expected error for unknown account")
	}
}

// --- Report Tests ---

func TestTrialBalance(t *testing.T) {
	e, _, _ := setupEngine()
	inv := makeInvoice("inv_001", "USD", 10000)
	ComputeLineItemTotals(inv)
	ctx := context.Background()

	e.TransitionDocument(ctx, inv, StatusSubmitted)
	e.TransitionDocument(ctx, inv, StatusAuthorised)

	tb := e.TrialBalanceReport(time.Time{})
	if tb.TotalDebit != tb.TotalCredit {
		t.Errorf("trial balance unequal: debit=%d credit=%d", tb.TotalDebit, tb.TotalCredit)
	}
	if len(tb.Rows) < 2 {
		t.Errorf("expected at least 2 rows, got %d", len(tb.Rows))
	}
}

func TestProfitAndLoss(t *testing.T) {
	e, _, clock := setupEngine()
	inv := makeInvoice("inv_001", "USD", 10000)
	ComputeLineItemTotals(inv)
	ctx := context.Background()

	e.TransitionDocument(ctx, inv, StatusSubmitted)
	e.TransitionDocument(ctx, inv, StatusAuthorised)

	from := clock.Now().Add(-time.Hour)
	to := clock.Now().Add(time.Hour)
	pnl := e.ProfitAndLossReport(from, to)

	if pnl.Revenue.Total != 10000 {
		t.Errorf("revenue = %d, want 10000", pnl.Revenue.Total)
	}
	if pnl.NetProfit != 10000 {
		t.Errorf("net profit = %d, want 10000", pnl.NetProfit)
	}
}

func TestBalanceSheet(t *testing.T) {
	e, _, _ := setupEngine()
	inv := makeInvoice("inv_001", "USD", 10000)
	ComputeLineItemTotals(inv)
	ctx := context.Background()

	e.TransitionDocument(ctx, inv, StatusSubmitted)
	e.TransitionDocument(ctx, inv, StatusAuthorised)

	bs := e.BalanceSheetReport(time.Time{})
	if bs.Assets.Total != 10000 {
		t.Errorf("assets = %d, want 10000 (AR)", bs.Assets.Total)
	}
}

// --- Account Tests ---

func TestCreateAccount(t *testing.T) {
	e := NewEngine()
	acct, err := e.CreateAccount(Account{Name: "Test", Type: AccountTypeAsset, Class: ClassCurrentAsset})
	if err != nil {
		t.Fatal(err)
	}
	if acct.ID != "acct_000001" {
		t.Errorf("ID = %s, want acct_000001", acct.ID)
	}
}

func TestCreateAccount_Duplicate(t *testing.T) {
	e, _, _ := setupEngine()
	_, err := e.CreateAccount(Account{ID: "acct_ar", Name: "Dup", Type: AccountTypeAsset})
	if err == nil {
		t.Fatal("expected error for duplicate account")
	}
}

func TestCreateAccount_MissingName(t *testing.T) {
	e := NewEngine()
	_, err := e.CreateAccount(Account{Type: AccountTypeAsset})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestListAccounts(t *testing.T) {
	e, _, _ := setupEngine()
	accts := e.ListAccounts()
	if len(accts) != 5 {
		t.Errorf("expected 5 accounts, got %d", len(accts))
	}
}

// --- Validate Document Tests ---

func TestValidateDocument_UnknownAccount(t *testing.T) {
	e, _, _ := setupEngine()
	doc := &Document{
		Currency: "USD",
		LineItems: []LineItem{
			{AccountID: "nonexistent", Quantity: 100, UnitAmount: 100},
		},
	}
	if err := e.ValidateDocument(doc); err == nil {
		t.Fatal("expected error for unknown account in line item")
	}
}

func TestValidateDocument_ComputesTotals(t *testing.T) {
	e, _, _ := setupEngine()
	doc := &Document{
		Currency: "USD",
		LineItems: []LineItem{
			{AccountID: "acct_rev", Quantity: 200, UnitAmount: 5000},
			{AccountID: "acct_rev", Quantity: 100, UnitAmount: 3000},
		},
	}
	if err := e.ValidateDocument(doc); err != nil {
		t.Fatal(err)
	}
	// 200 * 5000 / 100 = 10000, 100 * 3000 / 100 = 3000
	if doc.Total != 13000 {
		t.Errorf("total = %d, want 13000", doc.Total)
	}
}

// --- Hook Invocation Tests ---

func TestHooks_InvokedOnAuthorisation(t *testing.T) {
	e, hooks, _ := setupEngine()
	inv := makeInvoice("inv_001", "USD", 10000)
	ComputeLineItemTotals(inv)
	ctx := context.Background()

	e.TransitionDocument(ctx, inv, StatusSubmitted)
	e.TransitionDocument(ctx, inv, StatusAuthorised)

	if len(hooks.journalEntries) != 1 {
		t.Fatalf("expected 1 journal entry hook call, got %d", len(hooks.journalEntries))
	}
	if len(hooks.transitions) != 2 {
		t.Fatalf("expected 2 transition hooks, got %d", len(hooks.transitions))
	}
	if hooks.transitions[1].From != StatusSubmitted || hooks.transitions[1].To != StatusAuthorised {
		t.Error("unexpected transition record")
	}
}

func TestHooks_InvokedOnPayment(t *testing.T) {
	e, hooks, _ := setupEngine()
	inv := makeInvoice("inv_001", "USD", 10000)
	ComputeLineItemTotals(inv)
	ctx := context.Background()

	e.TransitionDocument(ctx, inv, StatusSubmitted)
	e.TransitionDocument(ctx, inv, StatusAuthorised)

	pmt := &Payment{
		ID: "pmt_001", DocumentID: "inv_001", DocumentType: DocTypeInvoice,
		BankAccountID: "acct_bank", Amount: 10000, Currency: "USD",
	}
	e.ApplyPayment(ctx, pmt, inv)

	if len(hooks.payments) != 1 {
		t.Fatalf("expected 1 payment hook, got %d", len(hooks.payments))
	}
	// Full payment also triggers PAID transition.
	found := false
	for _, tr := range hooks.transitions {
		if tr.To == StatusPaid {
			found = true
		}
	}
	if !found {
		t.Error("expected PAID transition hook")
	}
}

// --- Document Line Item Totals ---

func TestComputeLineItemTotals(t *testing.T) {
	doc := &Document{
		LineItems: []LineItem{
			{Quantity: 100, UnitAmount: 5000}, // 1.00 * 50.00 = 50.00 (5000 cents)
			{Quantity: 250, UnitAmount: 2000}, // 2.50 * 20.00 = 50.00 (5000 cents)
		},
	}
	ComputeLineItemTotals(doc)
	if doc.LineItems[0].Amount != 5000 {
		t.Errorf("line 0 amount = %d, want 5000", doc.LineItems[0].Amount)
	}
	if doc.LineItems[1].Amount != 5000 {
		t.Errorf("line 1 amount = %d, want 5000", doc.LineItems[1].Amount)
	}
	if doc.Total != 10000 {
		t.Errorf("total = %d, want 10000", doc.Total)
	}
	if doc.AmountDue != 10000 {
		t.Errorf("amount_due = %d, want 10000", doc.AmountDue)
	}
}
