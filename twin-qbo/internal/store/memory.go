package store

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	pkgstore "github.com/wondertwin-ai/wondertwin/twinkit/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/store/journal"
)

// MemoryStore holds all QBO twin state in memory.
type MemoryStore struct {
	Customers    *pkgstore.Store[Customer]
	Vendors      *pkgstore.Store[Vendor]
	Accounts     *pkgstore.Store[Account]
	Items        *pkgstore.Store[Item]
	Invoices     *pkgstore.Store[Invoice]
	Bills        *pkgstore.Store[Bill]
	Payments     *pkgstore.Store[Payment]
	BillPayments *pkgstore.Store[BillPayment]
	CreditMemos  *pkgstore.Store[CreditMemo]
	VendorCredits *pkgstore.Store[VendorCredit]
	SalesReceipts *pkgstore.Store[SalesReceipt]
	Deposits     *pkgstore.Store[Deposit]
	Transfers    *pkgstore.Store[Transfer]
	JournalEntries *pkgstore.Store[JournalEntry]
	Estimates    *pkgstore.Store[Estimate]
	Purchases    *pkgstore.Store[Purchase]
	CompanyInfos   *pkgstore.Store[CompanyInfo]
	Employees      *pkgstore.Store[Employee]
	Classes        *pkgstore.Store[Class]
	Departments    *pkgstore.Store[Department]
	Terms          *pkgstore.Store[Term]
	PaymentMethods *pkgstore.Store[PaymentMethod]
	TaxCodes       *pkgstore.Store[TaxCode]
	TaxRates       *pkgstore.Store[TaxRate]
	PreferencesStore *pkgstore.Store[Preferences]
	RefundReceipts   *pkgstore.Store[RefundReceipt]
	PurchaseOrders   *pkgstore.Store[PurchaseOrder]
	TimeActivities   *pkgstore.Store[TimeActivity]
	Journal          *journal.Journal
	Clock        *pkgstore.Clock
	idCounter    atomic.Uint64
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	clock := pkgstore.NewClock()
	return &MemoryStore{
		Customers:     pkgstore.New[Customer]("cust"),
		Vendors:       pkgstore.New[Vendor]("vend"),
		Accounts:      pkgstore.New[Account]("acct"),
		Items:         pkgstore.New[Item]("item"),
		Invoices:      pkgstore.New[Invoice]("inv"),
		Bills:         pkgstore.New[Bill]("bill"),
		Payments:      pkgstore.New[Payment]("pmt"),
		BillPayments:  pkgstore.New[BillPayment]("bpmt"),
		CreditMemos:   pkgstore.New[CreditMemo]("cm"),
		VendorCredits: pkgstore.New[VendorCredit]("vc"),
		SalesReceipts: pkgstore.New[SalesReceipt]("sr"),
		Deposits:      pkgstore.New[Deposit]("dep"),
		Transfers:     pkgstore.New[Transfer]("xfer"),
		JournalEntries: pkgstore.New[JournalEntry]("je"),
		Estimates:     pkgstore.New[Estimate]("est"),
		Purchases:     pkgstore.New[Purchase]("pur"),
		CompanyInfos:    pkgstore.New[CompanyInfo]("co"),
		Employees:       pkgstore.New[Employee]("emp"),
		Classes:         pkgstore.New[Class]("cls"),
		Departments:     pkgstore.New[Department]("dept"),
		Terms:           pkgstore.New[Term]("term"),
		PaymentMethods:  pkgstore.New[PaymentMethod]("pmeth"),
		TaxCodes:        pkgstore.New[TaxCode]("tc"),
		TaxRates:        pkgstore.New[TaxRate]("tr"),
		PreferencesStore: pkgstore.New[Preferences]("pref"),
		RefundReceipts:   pkgstore.New[RefundReceipt]("rr"),
		PurchaseOrders:   pkgstore.New[PurchaseOrder]("po"),
		TimeActivities:   pkgstore.New[TimeActivity]("ta"),
		Journal:          journal.New(clock),
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
	TimeActivities map[string]TimeActivity  `json:"time_activities"`
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
		TimeActivities: s.TimeActivities.Snapshot(),
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
	s.Journal.Reset()
	s.Clock.Reset()
	s.idCounter.Store(0)
}
