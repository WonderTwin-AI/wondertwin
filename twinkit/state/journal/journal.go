// Package journal provides an append-only, double-entry journal store.
// Every transaction must have balanced debits and credits with the same currency.
// Entries are immutable once written. Balances are always computed from history.
package journal

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// EntryType distinguishes debit from credit entries.
type EntryType string

const (
	Debit  EntryType = "debit"
	Credit EntryType = "credit"
)

// Entry represents a single journal entry (one side of a transaction).
type Entry struct {
	ID            string    `json:"id"`
	TransactionID string    `json:"transaction_id"`
	AccountID     string    `json:"account_id"`
	Type          EntryType `json:"type"`
	Amount        int64     `json:"amount"` // always positive; direction determined by Type
	Currency      string    `json:"currency"`
	SourceType    string    `json:"source_type"` // what created this: "invoice", "payment", "manual"
	SourceID      string    `json:"source_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// Transaction is a balanced set of entries written atomically.
type Transaction struct {
	ID        string    `json:"id"`
	Entries   []Entry   `json:"entries"`
	CreatedAt time.Time `json:"created_at"`
}

// Journal is an append-only, double-entry journal store.
// It is thread-safe for concurrent reads and writes.
type Journal struct {
	mu           sync.RWMutex
	transactions []Transaction
	clock        *state.Clock
	txCounter    int
	entryCounter int
}

// New creates a new empty Journal using the given clock for timestamps.
func New(clock *state.Clock) *Journal {
	return &Journal{
		transactions: make([]Transaction, 0),
		clock:        clock,
	}
}

// Append writes a balanced transaction to the journal.
// It validates that:
//   - at least two entries exist
//   - all amounts are positive
//   - all entries share the same currency
//   - total debits equal total credits
//
// Entry IDs, TransactionID, and CreatedAt are assigned by the journal.
func (j *Journal) Append(tx Transaction) error {
	if len(tx.Entries) < 2 {
		return fmt.Errorf("transaction must have at least 2 entries, got %d", len(tx.Entries))
	}

	// Validate amounts and currency consistency.
	currency := tx.Entries[0].Currency
	var totalDebit, totalCredit int64
	for i, e := range tx.Entries {
		if e.Amount <= 0 {
			return fmt.Errorf("entry %d: amount must be positive, got %d", i, e.Amount)
		}
		if e.Currency == "" {
			return fmt.Errorf("entry %d: currency is required", i)
		}
		if e.Currency != currency {
			return fmt.Errorf("entry %d: currency %q does not match transaction currency %q", i, e.Currency, currency)
		}
		if e.AccountID == "" {
			return fmt.Errorf("entry %d: account_id is required", i)
		}
		switch e.Type {
		case Debit:
			totalDebit += e.Amount
		case Credit:
			totalCredit += e.Amount
		default:
			return fmt.Errorf("entry %d: invalid type %q, must be %q or %q", i, e.Type, Debit, Credit)
		}
	}

	if totalDebit != totalCredit {
		return fmt.Errorf("transaction is unbalanced: debits=%d credits=%d", totalDebit, totalCredit)
	}

	// Assign IDs and timestamp atomically.
	j.mu.Lock()
	defer j.mu.Unlock()

	now := j.clock.Now()
	j.txCounter++
	txID := fmt.Sprintf("txn_%06d", j.txCounter)

	entries := make([]Entry, len(tx.Entries))
	for i, e := range tx.Entries {
		j.entryCounter++
		entries[i] = Entry{
			ID:            fmt.Sprintf("je_%06d", j.entryCounter),
			TransactionID: txID,
			AccountID:     e.AccountID,
			Type:          e.Type,
			Amount:        e.Amount,
			Currency:      e.Currency,
			SourceType:    e.SourceType,
			SourceID:      e.SourceID,
			CreatedAt:     now,
		}
	}

	j.transactions = append(j.transactions, Transaction{
		ID:        txID,
		Entries:   entries,
		CreatedAt: now,
	})

	return nil
}

// Balance computes the current balance for an account by summing all entries.
// Returns (debit_total, credit_total, net) where net = debit - credit.
func (j *Journal) Balance(accountID string) (debit, credit, net int64) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.balanceUpTo(accountID, time.Time{})
}

// BalanceAt computes the balance for an account at a specific point in time.
// Only entries with CreatedAt <= at are included.
func (j *Journal) BalanceAt(accountID string, at time.Time) (debit, credit, net int64) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.balanceUpTo(accountID, at)
}

// balanceUpTo computes the balance. If at is zero, all entries are included.
// Must be called with at least a read lock held.
func (j *Journal) balanceUpTo(accountID string, at time.Time) (debit, credit, net int64) {
	useTimeFilter := !at.IsZero()
	for _, tx := range j.transactions {
		if useTimeFilter && tx.CreatedAt.After(at) {
			continue
		}
		for _, e := range tx.Entries {
			if e.AccountID != accountID {
				continue
			}
			switch e.Type {
			case Debit:
				debit += e.Amount
			case Credit:
				credit += e.Amount
			}
		}
	}
	net = debit - credit
	return
}

// Entries returns all entries for an account, ordered by creation time.
func (j *Journal) Entries(accountID string) []Entry {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var result []Entry
	for _, tx := range j.transactions {
		for _, e := range tx.Entries {
			if e.AccountID == accountID {
				result = append(result, e)
			}
		}
	}
	return result
}

// EntriesBetween returns entries for an account within a time range [from, to].
func (j *Journal) EntriesBetween(accountID string, from, to time.Time) []Entry {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var result []Entry
	for _, tx := range j.transactions {
		if tx.CreatedAt.Before(from) || tx.CreatedAt.After(to) {
			continue
		}
		for _, e := range tx.Entries {
			if e.AccountID == accountID {
				result = append(result, e)
			}
		}
	}
	return result
}

// Transactions returns all transactions in append order.
func (j *Journal) Transactions() []Transaction {
	j.mu.RLock()
	defer j.mu.RUnlock()
	out := make([]Transaction, len(j.transactions))
	copy(out, j.transactions)
	return out
}

// snapshot is the JSON-serializable form of the journal.
type snapshot struct {
	Transactions []Transaction `json:"transactions"`
	TxCounter    int           `json:"tx_counter"`
	EntryCounter int           `json:"entry_counter"`
}

// Snapshot returns the full journal state for admin serialization.
func (j *Journal) Snapshot() any {
	j.mu.RLock()
	defer j.mu.RUnlock()
	txs := make([]Transaction, len(j.transactions))
	copy(txs, j.transactions)
	return snapshot{
		Transactions: txs,
		TxCounter:    j.txCounter,
		EntryCounter: j.entryCounter,
	}
}

// LoadSnapshot restores journal state from a JSON byte slice.
func (j *Journal) LoadSnapshot(data []byte) error {
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("journal: load snapshot: %w", err)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.transactions = snap.Transactions
	j.txCounter = snap.TxCounter
	j.entryCounter = snap.EntryCounter
	if j.transactions == nil {
		j.transactions = make([]Transaction, 0)
	}
	return nil
}

// Reset clears all entries and transactions.
func (j *Journal) Reset() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.transactions = make([]Transaction, 0)
	j.txCounter = 0
	j.entryCounter = 0
}
