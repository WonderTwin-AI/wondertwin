package store

import (
	"encoding/json"
	"fmt"

	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// MemoryStore holds all Increase twin state in memory.
type MemoryStore struct {
	Accounts             *pkgstate.Store[Account]
	AccountNumbers       *pkgstate.Store[AccountNumber]
	ExternalAccounts     *pkgstate.Store[ExternalAccount]
	ACHTransfers         *pkgstate.Store[ACHTransfer]
	InboundACHTransfers  *pkgstate.Store[InboundACHTransfer]
	Transactions         *pkgstate.Store[Transaction]
	PendingTransactions  *pkgstate.Store[PendingTransaction]
	Events               *pkgstate.Store[Event]
	EventSubscriptions   *pkgstate.Store[EventSubscription]
	Clock                *pkgstate.Clock
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	return &MemoryStore{
		Accounts:            pkgstate.New[Account]("account"),
		AccountNumbers:      pkgstate.New[AccountNumber]("account_number"),
		ExternalAccounts:    pkgstate.New[ExternalAccount]("external_account"),
		ACHTransfers:        pkgstate.New[ACHTransfer]("ach_transfer"),
		InboundACHTransfers: pkgstate.New[InboundACHTransfer]("inbound_ach_transfer"),
		Transactions:        pkgstate.New[Transaction]("transaction"),
		PendingTransactions: pkgstate.New[PendingTransaction]("pending_transaction"),
		Events:              pkgstate.New[Event]("event"),
		EventSubscriptions:  pkgstate.New[EventSubscription]("event_subscription"),
		Clock:               pkgstate.NewClock(),
	}
}

// stateSnapshot is the JSON-serializable state for admin endpoints.
type stateSnapshot struct {
	Accounts            map[string]Account            `json:"accounts"`
	AccountNumbers      map[string]AccountNumber      `json:"account_numbers,omitempty"`
	ExternalAccounts    map[string]ExternalAccount    `json:"external_accounts,omitempty"`
	ACHTransfers        map[string]ACHTransfer        `json:"ach_transfers,omitempty"`
	InboundACHTransfers map[string]InboundACHTransfer `json:"inbound_ach_transfers,omitempty"`
	Transactions        map[string]Transaction        `json:"transactions,omitempty"`
	PendingTransactions map[string]PendingTransaction `json:"pending_transactions,omitempty"`
	Events              map[string]Event              `json:"events,omitempty"`
	EventSubscriptions  map[string]EventSubscription  `json:"event_subscriptions,omitempty"`
}

// Snapshot returns the full state as a JSON-serializable value.
func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Accounts:            s.Accounts.Snapshot(),
		AccountNumbers:      s.AccountNumbers.Snapshot(),
		ExternalAccounts:    s.ExternalAccounts.Snapshot(),
		ACHTransfers:        s.ACHTransfers.Snapshot(),
		InboundACHTransfers: s.InboundACHTransfers.Snapshot(),
		Transactions:        s.Transactions.Snapshot(),
		PendingTransactions: s.PendingTransactions.Snapshot(),
		Events:              s.Events.Snapshot(),
		EventSubscriptions:  s.EventSubscriptions.Snapshot(),
	}
}

// LoadState replaces the full state from a JSON body.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	s.Accounts.LoadSnapshot(snap.Accounts)
	if snap.AccountNumbers != nil {
		s.AccountNumbers.LoadSnapshot(snap.AccountNumbers)
	}
	if snap.ExternalAccounts != nil {
		s.ExternalAccounts.LoadSnapshot(snap.ExternalAccounts)
	}
	if snap.ACHTransfers != nil {
		s.ACHTransfers.LoadSnapshot(snap.ACHTransfers)
	}
	if snap.InboundACHTransfers != nil {
		s.InboundACHTransfers.LoadSnapshot(snap.InboundACHTransfers)
	}
	if snap.Transactions != nil {
		s.Transactions.LoadSnapshot(snap.Transactions)
	}
	if snap.PendingTransactions != nil {
		s.PendingTransactions.LoadSnapshot(snap.PendingTransactions)
	}
	if snap.Events != nil {
		s.Events.LoadSnapshot(snap.Events)
	}
	if snap.EventSubscriptions != nil {
		s.EventSubscriptions.LoadSnapshot(snap.EventSubscriptions)
	}
	return nil
}

// Reset clears all state.
func (s *MemoryStore) Reset() {
	s.Accounts.Reset()
	s.AccountNumbers.Reset()
	s.ExternalAccounts.Reset()
	s.ACHTransfers.Reset()
	s.InboundACHTransfers.Reset()
	s.Transactions.Reset()
	s.PendingTransactions.Reset()
	s.Events.Reset()
	s.EventSubscriptions.Reset()
	s.Clock.Reset()
}

// Balance tracking helpers

// CreditAccount adds funds to an account (current + available).
func (s *MemoryStore) CreditAccount(accountID string, amount int64) error {
	acct, ok := s.Accounts.Get(accountID)
	if !ok {
		return fmt.Errorf("account not found: %s", accountID)
	}
	acct.CurrentBalance += amount
	acct.AvailableBalance += amount
	s.Accounts.Set(accountID, acct)
	return nil
}

// DebitAccount removes funds from an account (current + available).
func (s *MemoryStore) DebitAccount(accountID string, amount int64) error {
	acct, ok := s.Accounts.Get(accountID)
	if !ok {
		return fmt.Errorf("account not found: %s", accountID)
	}
	acct.CurrentBalance -= amount
	acct.AvailableBalance -= amount
	s.Accounts.Set(accountID, acct)
	return nil
}

// HoldFunds reduces available balance without affecting current balance.
func (s *MemoryStore) HoldFunds(accountID string, amount int64) error {
	acct, ok := s.Accounts.Get(accountID)
	if !ok {
		return fmt.Errorf("account not found: %s", accountID)
	}
	acct.AvailableBalance -= amount
	s.Accounts.Set(accountID, acct)
	return nil
}

// ReleaseFunds increases available balance without affecting current balance.
func (s *MemoryStore) ReleaseFunds(accountID string, amount int64) error {
	acct, ok := s.Accounts.Get(accountID)
	if !ok {
		return fmt.Errorf("account not found: %s", accountID)
	}
	acct.AvailableBalance += amount
	s.Accounts.Set(accountID, acct)
	return nil
}

// SettleHold converts a held amount to a settled balance change.
func (s *MemoryStore) SettleHold(accountID string, amount int64) error {
	acct, ok := s.Accounts.Get(accountID)
	if !ok {
		return fmt.Errorf("account not found: %s", accountID)
	}
	// Current balance changes, available already reflects the hold
	acct.CurrentBalance -= amount
	s.Accounts.Set(accountID, acct)
	return nil
}
