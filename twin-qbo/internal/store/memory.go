package store

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
	"github.com/wondertwin-ai/wondertwin/twinkit/state/journal"
)

// MemoryStore holds all QBO twin state in memory.
type MemoryStore struct {
	Customers    *pkgstate.Store[Customer]
	Vendors      *pkgstate.Store[Vendor]
	Accounts     *pkgstate.Store[Account]
	Items        *pkgstate.Store[Item]
	Invoices     *pkgstate.Store[Invoice]
	Bills        *pkgstate.Store[Bill]
	Payments     *pkgstate.Store[Payment]
	BillPayments *pkgstate.Store[BillPayment]
	CreditMemos  *pkgstate.Store[CreditMemo]
	VendorCredits *pkgstate.Store[VendorCredit]
	SalesReceipts *pkgstate.Store[SalesReceipt]
	Deposits     *pkgstate.Store[Deposit]
	Transfers    *pkgstate.Store[Transfer]
	JournalEntries *pkgstate.Store[JournalEntry]
	Estimates    *pkgstate.Store[Estimate]
	Purchases    *pkgstate.Store[Purchase]
	CompanyInfos   *pkgstate.Store[CompanyInfo]
	Employees      *pkgstate.Store[Employee]
	Classes        *pkgstate.Store[Class]
	Departments    *pkgstate.Store[Department]
	Terms          *pkgstate.Store[Term]
	PaymentMethods *pkgstate.Store[PaymentMethod]
	TaxCodes       *pkgstate.Store[TaxCode]
	TaxRates       *pkgstate.Store[TaxRate]
	PreferencesStore *pkgstate.Store[Preferences]
	RefundReceipts   *pkgstate.Store[RefundReceipt]
	PurchaseOrders   *pkgstate.Store[PurchaseOrder]
	TimeActivities         *pkgstate.Store[TimeActivity]
	RecurringTransactions  *pkgstate.Store[RecurringTransaction]
	Journal                *journal.Journal
	Clock        *pkgstate.Clock
	idCounter    atomic.Uint64
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	clock := pkgstate.NewClock()
	return &MemoryStore{
		Customers:     pkgstate.New[Customer]("cust"),
		Vendors:       pkgstate.New[Vendor]("vend"),
		Accounts:      pkgstate.New[Account]("acct"),
		Items:         pkgstate.New[Item]("item"),
		Invoices:      pkgstate.New[Invoice]("inv"),
		Bills:         pkgstate.New[Bill]("bill"),
		Payments:      pkgstate.New[Payment]("pmt"),
		BillPayments:  pkgstate.New[BillPayment]("bpmt"),
		CreditMemos:   pkgstate.New[CreditMemo]("cm"),
		VendorCredits: pkgstate.New[VendorCredit]("vc"),
		SalesReceipts: pkgstate.New[SalesReceipt]("sr"),
		Deposits:      pkgstate.New[Deposit]("dep"),
		Transfers:     pkgstate.New[Transfer]("xfer"),
		JournalEntries: pkgstate.New[JournalEntry]("je"),
		Estimates:     pkgstate.New[Estimate]("est"),
		Purchases:     pkgstate.New[Purchase]("pur"),
		CompanyInfos:    pkgstate.New[CompanyInfo]("co"),
		Employees:       pkgstate.New[Employee]("emp"),
		Classes:         pkgstate.New[Class]("cls"),
		Departments:     pkgstate.New[Department]("dept"),
		Terms:           pkgstate.New[Term]("term"),
		PaymentMethods:  pkgstate.New[PaymentMethod]("pmeth"),
		TaxCodes:        pkgstate.New[TaxCode]("tc"),
		TaxRates:        pkgstate.New[TaxRate]("tr"),
		PreferencesStore: pkgstate.New[Preferences]("pref"),
		RefundReceipts:   pkgstate.New[RefundReceipt]("rr"),
		PurchaseOrders:   pkgstate.New[PurchaseOrder]("po"),
		TimeActivities:        pkgstate.New[TimeActivity]("ta"),
		RecurringTransactions: pkgstate.New[RecurringTransaction]("rt"),
		Journal:               journal.New(clock),
		Clock:         clock,
	}
}

// NextID generates a QBO-style numeric string ID.
func (s *MemoryStore) NextID() string {
	return fmt.Sprintf("%d", s.idCounter.Add(1))
}

// Now returns the current time formatted as QBO ISO 8601.
func (s *MemoryStore) Now() string {
	return s.Clock.Now().Format(time.RFC3339)
}

// NewMetaData creates a MetaData with the current time.
func (s *MemoryStore) NewMetaData() MetaData {
	now := s.Now()
	return MetaData{CreateTime: now, LastUpdatedTime: now}
}

type stateSnapshot struct {
	Customers     map[string]Customer     `json:"customers"`
	Vendors       map[string]Vendor       `json:"vendors"`
	Accounts      map[string]Account      `json:"accounts"`
	Items         map[string]Item         `json:"items"`
	Invoices      map[string]Invoice      `json:"invoices"`
	Bills         map[string]Bill         `json:"bills"`
	Payments      map[string]Payment      `json:"payments"`
	BillPayments  map[string]BillPayment  `json:"bill_payments"`
	CreditMemos   map[string]CreditMemo   `json:"credit_memos"`
	VendorCredits map[string]VendorCredit `json:"vendor_credits"`
	SalesReceipts map[string]SalesReceipt `json:"sales_receipts"`
	Deposits      map[string]Deposit      `json:"deposits"`
	Transfers     map[string]Transfer     `json:"transfers"`
	JournalEntries map[string]JournalEntry `json:"journal_entries"`
	Estimates     map[string]Estimate     `json:"estimates"`
	Purchases     map[string]Purchase     `json:"purchases"`
	CompanyInfos   map[string]CompanyInfo   `json:"company_info"`
	Employees      map[string]Employee      `json:"employees"`
	Classes        map[string]Class         `json:"classes"`
	Departments    map[string]Department    `json:"departments"`
	Terms          map[string]Term          `json:"terms"`
	PaymentMethods map[string]PaymentMethod `json:"payment_methods"`
	TaxCodes       map[string]TaxCode       `json:"tax_codes"`
	TaxRates       map[string]TaxRate       `json:"tax_rates"`
	Preferences    map[string]Preferences   `json:"preferences"`
	RefundReceipts map[string]RefundReceipt `json:"refund_receipts"`
	PurchaseOrders map[string]PurchaseOrder `json:"purchase_orders"`
	TimeActivities        map[string]TimeActivity        `json:"time_activities"`
	RecurringTransactions map[string]RecurringTransaction `json:"recurring_transactions"`
}

func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Customers:     s.Customers.Snapshot(),
		Vendors:       s.Vendors.Snapshot(),
		Accounts:      s.Accounts.Snapshot(),
		Items:         s.Items.Snapshot(),
		Invoices:      s.Invoices.Snapshot(),
		Bills:         s.Bills.Snapshot(),
		Payments:      s.Payments.Snapshot(),
		BillPayments:  s.BillPayments.Snapshot(),
		CreditMemos:   s.CreditMemos.Snapshot(),
		VendorCredits: s.VendorCredits.Snapshot(),
		SalesReceipts: s.SalesReceipts.Snapshot(),
		Deposits:      s.Deposits.Snapshot(),
		Transfers:     s.Transfers.Snapshot(),
		JournalEntries: s.JournalEntries.Snapshot(),
		Estimates:     s.Estimates.Snapshot(),
		Purchases:     s.Purchases.Snapshot(),
		CompanyInfos:   s.CompanyInfos.Snapshot(),
		Employees:      s.Employees.Snapshot(),
		Classes:        s.Classes.Snapshot(),
		Departments:    s.Departments.Snapshot(),
		Terms:          s.Terms.Snapshot(),
		PaymentMethods: s.PaymentMethods.Snapshot(),
		TaxCodes:       s.TaxCodes.Snapshot(),
		TaxRates:       s.TaxRates.Snapshot(),
		Preferences:    s.PreferencesStore.Snapshot(),
		RefundReceipts: s.RefundReceipts.Snapshot(),
		PurchaseOrders: s.PurchaseOrders.Snapshot(),
		TimeActivities:        s.TimeActivities.Snapshot(),
		RecurringTransactions: s.RecurringTransactions.Snapshot(),
	}
}

func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	s.Customers.LoadSnapshot(snap.Customers)
	s.Vendors.LoadSnapshot(snap.Vendors)
	s.Accounts.LoadSnapshot(snap.Accounts)
	s.Items.LoadSnapshot(snap.Items)
	s.Invoices.LoadSnapshot(snap.Invoices)
	s.Bills.LoadSnapshot(snap.Bills)
	s.Payments.LoadSnapshot(snap.Payments)
	s.BillPayments.LoadSnapshot(snap.BillPayments)
	s.CreditMemos.LoadSnapshot(snap.CreditMemos)
	s.VendorCredits.LoadSnapshot(snap.VendorCredits)
	s.SalesReceipts.LoadSnapshot(snap.SalesReceipts)
	s.Deposits.LoadSnapshot(snap.Deposits)
	s.Transfers.LoadSnapshot(snap.Transfers)
	s.JournalEntries.LoadSnapshot(snap.JournalEntries)
	s.Estimates.LoadSnapshot(snap.Estimates)
	s.Purchases.LoadSnapshot(snap.Purchases)
	s.CompanyInfos.LoadSnapshot(snap.CompanyInfos)
	s.Employees.LoadSnapshot(snap.Employees)
	s.Classes.LoadSnapshot(snap.Classes)
	s.Departments.LoadSnapshot(snap.Departments)
	s.Terms.LoadSnapshot(snap.Terms)
	s.PaymentMethods.LoadSnapshot(snap.PaymentMethods)
	s.TaxCodes.LoadSnapshot(snap.TaxCodes)
	s.TaxRates.LoadSnapshot(snap.TaxRates)
	s.PreferencesStore.LoadSnapshot(snap.Preferences)
	s.RefundReceipts.LoadSnapshot(snap.RefundReceipts)
	s.PurchaseOrders.LoadSnapshot(snap.PurchaseOrders)
	s.TimeActivities.LoadSnapshot(snap.TimeActivities)
	s.RecurringTransactions.LoadSnapshot(snap.RecurringTransactions)
	return nil
}

func (s *MemoryStore) Reset() {
	s.Customers.Reset()
	s.Vendors.Reset()
	s.Accounts.Reset()
	s.Items.Reset()
	s.Invoices.Reset()
	s.Bills.Reset()
	s.Payments.Reset()
	s.BillPayments.Reset()
	s.CreditMemos.Reset()
	s.VendorCredits.Reset()
	s.SalesReceipts.Reset()
	s.Deposits.Reset()
	s.Transfers.Reset()
	s.JournalEntries.Reset()
	s.Estimates.Reset()
	s.Purchases.Reset()
	s.CompanyInfos.Reset()
	s.Employees.Reset()
	s.Classes.Reset()
	s.Departments.Reset()
	s.Terms.Reset()
	s.PaymentMethods.Reset()
	s.TaxCodes.Reset()
	s.TaxRates.Reset()
	s.PreferencesStore.Reset()
	s.RefundReceipts.Reset()
	s.PurchaseOrders.Reset()
	s.TimeActivities.Reset()
	s.RecurringTransactions.Reset()
	s.Journal.Reset()
	s.Clock.Reset()
	s.idCounter.Store(0)
}
