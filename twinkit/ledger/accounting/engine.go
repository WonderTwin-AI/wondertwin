package accounting

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/store/journal"
)

// Engine is the accounting ledger engine. It enforces accounting invariants
// and delegates platform-specific behavior to hooks.
type Engine struct {
	mu         sync.RWMutex
	journal    *journal.Journal
	hooks      AccountingHooks
	clock      *store.Clock
	accounts   map[string]*Account
	acctOrder  []string // insertion order
	periodLock time.Time // reject entries before this time
	acctCounter int

	// Well-known account IDs for AR/AP. Set by CreateAccount or configurable.
	arAccountID string
	apAccountID string
}

// Option configures the Engine.
type Option func(*Engine)

// WithJournal sets the journal store.
func WithJournal(j *journal.Journal) Option {
	return func(e *Engine) { e.journal = j }
}

// WithHooks sets the accounting hooks implementation.
func WithHooks(h AccountingHooks) Option {
	return func(e *Engine) { e.hooks = h }
}

// WithClock sets the clock for timestamps.
func WithClock(c *store.Clock) Option {
	return func(e *Engine) { e.clock = c }
}

// WithPeriodLock sets the initial period lock date.
func WithPeriodLock(t time.Time) Option {
	return func(e *Engine) { e.periodLock = t }
}

// NewEngine creates a new accounting engine with the given options.
func NewEngine(opts ...Option) *Engine {
	e := &Engine{
		accounts: make(map[string]*Account),
	}
	for _, opt := range opts {
		opt(e)
	}
	if e.clock == nil {
		e.clock = store.NewClock()
	}
	if e.journal == nil {
		e.journal = journal.New(e.clock)
	}
	return e
}

// Journal returns the underlying journal for direct access (e.g., reports).
func (e *Engine) Journal() *journal.Journal {
	return e.journal
}

// CreateAccount adds an account to the chart of accounts.
func (e *Engine) CreateAccount(acct Account) (*Account, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if acct.Name == "" {
		return nil, fmt.Errorf("account name is required")
	}
	if acct.Type == "" {
		return nil, fmt.Errorf("account type is required")
	}

	if acct.ID == "" {
		e.acctCounter++
		acct.ID = fmt.Sprintf("acct_%06d", e.acctCounter)
	}

	if _, exists := e.accounts[acct.ID]; exists {
		return nil, fmt.Errorf("account %s already exists", acct.ID)
	}

	e.accounts[acct.ID] = &acct
	e.acctOrder = append(e.acctOrder, acct.ID)

	// Track well-known accounts by class.
	switch acct.Class {
	case ClassCurrentAsset:
		if acct.Name == "Accounts Receivable" || acct.Code == "200" {
			e.arAccountID = acct.ID
		}
	case ClassCurrentLiability:
		if acct.Name == "Accounts Payable" || acct.Code == "800" {
			e.apAccountID = acct.ID
		}
	}

	return &acct, nil
}

// GetAccount returns an account by ID.
func (e *Engine) GetAccount(id string) (*Account, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	acct, ok := e.accounts[id]
	if !ok {
		return nil, fmt.Errorf("account %s not found", id)
	}
	return acct, nil
}

// ListAccounts returns all accounts in creation order.
func (e *Engine) ListAccounts() []Account {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]Account, 0, len(e.acctOrder))
	for _, id := range e.acctOrder {
		result = append(result, *e.accounts[id])
	}
	return result
}

// SetReceivableAccount explicitly sets the Accounts Receivable account ID.
func (e *Engine) SetReceivableAccount(id string) { e.arAccountID = id }

// SetPayableAccount explicitly sets the Accounts Payable account ID.
func (e *Engine) SetPayableAccount(id string) { e.apAccountID = id }

func (e *Engine) receivableAccountID() string {
	if e.arAccountID != "" {
		return e.arAccountID
	}
	return "acct_receivable"
}

func (e *Engine) payableAccountID() string {
	if e.apAccountID != "" {
		return e.apAccountID
	}
	return "acct_payable"
}

// ValidateDocument computes line item totals and validates that line items
// reference known accounts.
func (e *Engine) ValidateDocument(doc *Document) error {
	if len(doc.LineItems) == 0 {
		return fmt.Errorf("document must have at least one line item")
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	for i, li := range doc.LineItems {
		if li.AccountID == "" {
			return fmt.Errorf("line item %d: account_id is required", i)
		}
		if _, ok := e.accounts[li.AccountID]; !ok {
			return fmt.Errorf("line item %d: account %s not found", i, li.AccountID)
		}
	}

	ComputeLineItemTotals(doc)
	return nil
}

// TransitionDocument transitions a document's status, enforcing the state machine,
// period lock, and creating journal entries on AUTHORISED.
func (e *Engine) TransitionDocument(ctx context.Context, doc *Document, to DocumentStatus) error {
	from := doc.Status

	if err := ValidateTransition(from, to); err != nil {
		return err
	}

	// Check period lock.
	if !e.periodLock.IsZero() && !doc.Date.IsZero() && doc.Date.Before(e.periodLock) {
		return fmt.Errorf("document date %s is in a locked period (before %s)",
			doc.Date.Format("2006-01-02"), e.periodLock.Format("2006-01-02"))
	}

	now := e.clock.Now()

	// Create journal entries when transitioning to AUTHORISED.
	if to == StatusAuthorised {
		entries, err := e.journalEntriesForAuthorisation(doc)
		if err != nil {
			return fmt.Errorf("journal entries for authorisation: %w", err)
		}
		if err := e.journal.Append(journal.Transaction{Entries: entries}); err != nil {
			return fmt.Errorf("journal append: %w", err)
		}

		// Notify hooks about journal entries.
		if e.hooks != nil {
			infos := make([]JournalEntryInfo, len(entries))
			for i, ent := range entries {
				infos[i] = JournalEntryInfo{
					AccountID:  ent.AccountID,
					Type:       string(ent.Type),
					Amount:     ent.Amount,
					SourceType: ent.SourceType,
					SourceID:   ent.SourceID,
				}
			}
			if err := e.hooks.OnJournalEntryCreated(ctx, doc, infos); err != nil {
				return fmt.Errorf("hook OnJournalEntryCreated: %w", err)
			}
		}
	}

	doc.Status = to
	doc.UpdatedAt = now

	if e.hooks != nil {
		if err := e.hooks.OnDocumentStateTransition(ctx, doc, from, to); err != nil {
			return fmt.Errorf("hook OnDocumentStateTransition: %w", err)
		}
	}

	return nil
}

// journalEntriesForAuthorisation creates the journal entries when a document is authorised.
func (e *Engine) journalEntriesForAuthorisation(doc *Document) ([]journal.Entry, error) {
	if doc.Total <= 0 {
		return nil, fmt.Errorf("document total must be positive")
	}

	sourceType := string(doc.Type)
	var entries []journal.Entry

	switch doc.Type {
	case DocTypeInvoice:
		// Invoice AUTHORISED: Debit Accounts Receivable, Credit Revenue account(s)
		entries = append(entries, journal.Entry{
			AccountID:  e.receivableAccountID(),
			Type:       journal.Debit,
			Amount:     doc.Total,
			Currency:   doc.Currency,
			SourceType: sourceType,
			SourceID:   doc.ID,
		})
		for _, li := range doc.LineItems {
			if li.Amount > 0 {
				entries = append(entries, journal.Entry{
					AccountID:  li.AccountID,
					Type:       journal.Credit,
					Amount:     li.Amount,
					Currency:   doc.Currency,
					SourceType: sourceType,
					SourceID:   doc.ID,
				})
			}
		}

	case DocTypeBill:
		// Bill AUTHORISED: Debit Expense account(s), Credit Accounts Payable
		for _, li := range doc.LineItems {
			if li.Amount > 0 {
				entries = append(entries, journal.Entry{
					AccountID:  li.AccountID,
					Type:       journal.Debit,
					Amount:     li.Amount,
					Currency:   doc.Currency,
					SourceType: sourceType,
					SourceID:   doc.ID,
				})
			}
		}
		entries = append(entries, journal.Entry{
			AccountID:  e.payableAccountID(),
			Type:       journal.Credit,
			Amount:     doc.Total,
			Currency:   doc.Currency,
			SourceType: sourceType,
			SourceID:   doc.ID,
		})

	case DocTypeCreditNote:
		// Credit note AUTHORISED: Debit Revenue, Credit AR (reverse of invoice)
		for _, li := range doc.LineItems {
			if li.Amount > 0 {
				entries = append(entries, journal.Entry{
					AccountID:  li.AccountID,
					Type:       journal.Debit,
					Amount:     li.Amount,
					Currency:   doc.Currency,
					SourceType: sourceType,
					SourceID:   doc.ID,
				})
			}
		}
		entries = append(entries, journal.Entry{
			AccountID:  e.receivableAccountID(),
			Type:       journal.Credit,
			Amount:     doc.Total,
			Currency:   doc.Currency,
			SourceType: sourceType,
			SourceID:   doc.ID,
		})

	default:
		return nil, fmt.Errorf("unsupported document type %s", doc.Type)
	}

	return entries, nil
}

// ApplyPayment applies a payment to a document.
func (e *Engine) ApplyPayment(ctx context.Context, pmt *Payment, doc *Document) error {
	return e.applyPayment(ctx, pmt, doc)
}

// PostManualJournal validates and posts a manual journal entry.
func (e *Engine) PostManualJournal(ctx context.Context, mj *ManualJournal) error {
	if len(mj.Lines) == 0 {
		return fmt.Errorf("manual journal must have at least one line")
	}

	// Check period lock.
	if !e.periodLock.IsZero() && !mj.Date.IsZero() && mj.Date.Before(e.periodLock) {
		return fmt.Errorf("journal date %s is in a locked period (before %s)",
			mj.Date.Format("2006-01-02"), e.periodLock.Format("2006-01-02"))
	}

	// Build journal entries from manual journal lines.
	var entries []journal.Entry
	var totalDebit, totalCredit int64

	e.mu.RLock()
	for i, line := range mj.Lines {
		if line.AccountID == "" {
			e.mu.RUnlock()
			return fmt.Errorf("line %d: account_id is required", i)
		}
		if _, ok := e.accounts[line.AccountID]; !ok {
			e.mu.RUnlock()
			return fmt.Errorf("line %d: account %s not found", i, line.AccountID)
		}

		if line.DebitAmount > 0 {
			entries = append(entries, journal.Entry{
				AccountID:  line.AccountID,
				Type:       journal.Debit,
				Amount:     line.DebitAmount,
				Currency:   "USD", // default currency for manual journals
				SourceType: "manual_journal",
				SourceID:   mj.ID,
			})
			totalDebit += line.DebitAmount
		}
		if line.CreditAmount > 0 {
			entries = append(entries, journal.Entry{
				AccountID:  line.AccountID,
				Type:       journal.Credit,
				Amount:     line.CreditAmount,
				Currency:   "USD",
				SourceType: "manual_journal",
				SourceID:   mj.ID,
			})
			totalCredit += line.CreditAmount
		}
	}
	e.mu.RUnlock()

	if totalDebit != totalCredit {
		return fmt.Errorf("manual journal is unbalanced: debits=%d credits=%d", totalDebit, totalCredit)
	}

	if len(entries) < 2 {
		return fmt.Errorf("manual journal must produce at least 2 journal entries")
	}

	if err := e.journal.Append(journal.Transaction{Entries: entries}); err != nil {
		return fmt.Errorf("journal append: %w", err)
	}

	mj.Status = "POSTED"
	mj.CreatedAt = e.clock.Now()

	return nil
}

// LockPeriod locks all periods before the given time. Subsequent mutations
// to documents or journals dated before this time will be rejected.
func (e *Engine) LockPeriod(before time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.periodLock = before
}

// PeriodLock returns the current period lock time.
func (e *Engine) PeriodLock() time.Time {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.periodLock
}
