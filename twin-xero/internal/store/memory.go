package store

import (
	"encoding/json"

	pkgstore "github.com/wondertwin-ai/wondertwin/twinkit/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/store/journal"
)

// MemoryStore holds all Xero twin state in memory.
type MemoryStore struct {
	Contacts       *pkgstore.Store[Contact]
	Accounts       *pkgstore.Store[Account]
	Invoices       *pkgstore.Store[Invoice]
	CreditNotes    *pkgstore.Store[CreditNote]
	Payments       *pkgstore.Store[Payment]
	BankTxns       *pkgstore.Store[BankTransaction]
	ManualJournals *pkgstore.Store[ManualJournal]
	Items               *pkgstore.Store[Item]
	TaxRates            *pkgstore.Store[TaxRate]
	TrackingCategories  *pkgstore.Store[TrackingCategory]
	Prepayments         *pkgstore.Store[Prepayment]
	Overpayments        *pkgstore.Store[Overpayment]
	Journal             *journal.Journal
	Clock               *pkgstore.Clock
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	clock := pkgstore.NewClock()
	return &MemoryStore{
		Contacts:       pkgstore.New[Contact]("con"),
		Accounts:       pkgstore.New[Account]("acct"),
		Invoices:       pkgstore.New[Invoice]("inv"),
		CreditNotes:    pkgstore.New[CreditNote]("cn"),
		Payments:       pkgstore.New[Payment]("pmt"),
		BankTxns:       pkgstore.New[BankTransaction]("btxn"),
		ManualJournals: pkgstore.New[ManualJournal]("mj"),
		Items:              pkgstore.New[Item]("item"),
		TaxRates:           pkgstore.New[TaxRate]("tax"),
		TrackingCategories: pkgstore.New[TrackingCategory]("tc"),
		Prepayments:        pkgstore.New[Prepayment]("pp"),
		Overpayments:       pkgstore.New[Overpayment]("op"),
		Journal:            journal.New(clock),
		Clock:          clock,
	}
}

type stateSnapshot struct {
	Contacts       map[string]Contact         `json:"contacts"`
	Accounts       map[string]Account         `json:"accounts"`
	Invoices       map[string]Invoice         `json:"invoices"`
	CreditNotes    map[string]CreditNote      `json:"credit_notes"`
	Payments       map[string]Payment         `json:"payments"`
	BankTxns       map[string]BankTransaction `json:"bank_transactions"`
	ManualJournals map[string]ManualJournal   `json:"manual_journals"`
	Items              map[string]Item              `json:"items"`
	TaxRates           map[string]TaxRate           `json:"tax_rates"`
	TrackingCategories map[string]TrackingCategory  `json:"tracking_categories"`
	Prepayments        map[string]Prepayment        `json:"prepayments"`
	Overpayments       map[string]Overpayment       `json:"overpayments"`
}

// Snapshot returns the full state as a JSON-serializable value.
func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Contacts:       s.Contacts.Snapshot(),
		Accounts:       s.Accounts.Snapshot(),
		Invoices:       s.Invoices.Snapshot(),
		CreditNotes:    s.CreditNotes.Snapshot(),
		Payments:       s.Payments.Snapshot(),
		BankTxns:       s.BankTxns.Snapshot(),
		ManualJournals: s.ManualJournals.Snapshot(),
		Items:              s.Items.Snapshot(),
		TaxRates:           s.TaxRates.Snapshot(),
		TrackingCategories: s.TrackingCategories.Snapshot(),
		Prepayments:        s.Prepayments.Snapshot(),
		Overpayments:       s.Overpayments.Snapshot(),
	}
}

// LoadState replaces the full state from a JSON body.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	s.Contacts.LoadSnapshot(snap.Contacts)
	s.Accounts.LoadSnapshot(snap.Accounts)
	s.Invoices.LoadSnapshot(snap.Invoices)
	s.CreditNotes.LoadSnapshot(snap.CreditNotes)
	s.Payments.LoadSnapshot(snap.Payments)
	s.BankTxns.LoadSnapshot(snap.BankTxns)
	s.ManualJournals.LoadSnapshot(snap.ManualJournals)
	s.Items.LoadSnapshot(snap.Items)
	s.TaxRates.LoadSnapshot(snap.TaxRates)
	s.TrackingCategories.LoadSnapshot(snap.TrackingCategories)
	s.Prepayments.LoadSnapshot(snap.Prepayments)
	s.Overpayments.LoadSnapshot(snap.Overpayments)
	return nil
}

// Reset clears all state.
func (s *MemoryStore) Reset() {
	s.Contacts.Reset()
	s.Accounts.Reset()
	s.Invoices.Reset()
	s.CreditNotes.Reset()
	s.Payments.Reset()
	s.BankTxns.Reset()
	s.ManualJournals.Reset()
	s.Items.Reset()
	s.TaxRates.Reset()
	s.TrackingCategories.Reset()
	s.Prepayments.Reset()
	s.Overpayments.Reset()
	s.Journal.Reset()
	s.Clock.Reset()
}

// FindAccountByCode looks up an account by its code.
func (s *MemoryStore) FindAccountByCode(code string) (Account, bool) {
	items := s.Accounts.Filter(func(_ string, a Account) bool {
		return a.Code == code
	})
	if len(items) == 0 {
		return Account{}, false
	}
	return items[0], true
}
