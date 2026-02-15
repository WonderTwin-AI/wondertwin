package store

import (
	"encoding/json"
	"fmt"
	"sync"

	pkgstore "github.com/wondertwin-ai/wondertwin/twinkit/store"
)

// MemoryStore holds all Stripe twin state in memory.
type MemoryStore struct {
	mu sync.RWMutex

	Accounts         *pkgstore.Store[Account]
	ExternalAccts    *pkgstore.Store[ExternalAccount]
	Transfers        *pkgstore.Store[Transfer]
	Payouts          *pkgstore.Store[Payout]
	Events           *pkgstore.Store[Event]

	// Per-account balances (account ID -> balance)
	Balances         map[string]*AccountBalance

	// Platform balance (the main Stripe account)
	PlatformBalance  *AccountBalance

	Clock            *pkgstore.Clock
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	return &MemoryStore{
		Accounts:        pkgstore.New[Account]("acct"),
		ExternalAccts:   pkgstore.New[ExternalAccount]("ba"),
		Transfers:       pkgstore.New[Transfer]("tr"),
		Payouts:         pkgstore.New[Payout]("po"),
		Events:          pkgstore.New[Event]("evt"),
		Balances:        make(map[string]*AccountBalance),
		PlatformBalance: NewAccountBalance(),
		Clock:           pkgstore.NewClock(),
	}
}

// GetOrCreateBalance returns the balance for an account, creating it if needed.
func (s *MemoryStore) GetOrCreateBalance(accountID string) *AccountBalance {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.Balances[accountID]; ok {
		return b
	}
	b := NewAccountBalance()
	s.Balances[accountID] = b
	return b
}

// GetBalance returns the balance for an account or the platform balance.
func (s *MemoryStore) GetBalance(accountID string) *Balance {
	var ab *AccountBalance
	if accountID == "" {
		ab = s.PlatformBalance
	} else {
		ab = s.GetOrCreateBalance(accountID)
	}

	balance := &Balance{
		Object:   "balance",
		Livemode: false,
	}
	for currency, amount := range ab.Available {
		balance.Available = append(balance.Available, BalanceAmount{Amount: amount, Currency: currency})
	}
	for currency, amount := range ab.Pending {
		balance.Pending = append(balance.Pending, BalanceAmount{Amount: amount, Currency: currency})
	}
	if len(balance.Available) == 0 {
		balance.Available = []BalanceAmount{{Amount: 0, Currency: "usd"}}
	}
	if len(balance.Pending) == 0 {
		balance.Pending = []BalanceAmount{{Amount: 0, Currency: "usd"}}
	}
	return balance
}

// CreditBalance adds to an account's available balance.
func (s *MemoryStore) CreditBalance(accountID string, currency string, amount int64) {
	b := s.GetOrCreateBalance(accountID)
	s.mu.Lock()
	defer s.mu.Unlock()
	b.Available[currency] += amount
}

// DebitBalance subtracts from an account's available balance.
func (s *MemoryStore) DebitBalance(accountID string, currency string, amount int64) error {
	b := s.GetOrCreateBalance(accountID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if b.Available[currency] < amount {
		return fmt.Errorf("insufficient funds: available %d, requested %d", b.Available[currency], amount)
	}
	b.Available[currency] -= amount
	return nil
}

// stateSnapshot is the JSON-serializable state for admin endpoints.
type stateSnapshot struct {
	Accounts        map[string]Account          `json:"accounts"`
	ExternalAccts   map[string]ExternalAccount  `json:"external_accounts"`
	Transfers       map[string]Transfer         `json:"transfers"`
	Payouts         map[string]Payout           `json:"payouts"`
	Events          map[string]Event            `json:"events"`
	Balances        map[string]*AccountBalance  `json:"balances"`
	PlatformBalance *AccountBalance             `json:"platform_balance"`
}

// Snapshot returns the full state as a JSON-serializable value.
func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Accounts:        s.Accounts.Snapshot(),
		ExternalAccts:   s.ExternalAccts.Snapshot(),
		Transfers:       s.Transfers.Snapshot(),
		Payouts:         s.Payouts.Snapshot(),
		Events:          s.Events.Snapshot(),
		Balances:        s.snapshotBalances(),
		PlatformBalance: s.PlatformBalance,
	}
}

func (s *MemoryStore) snapshotBalances() map[string]*AccountBalance {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*AccountBalance, len(s.Balances))
	for k, v := range s.Balances {
		out[k] = v
	}
	return out
}

// LoadState replaces the full state from a JSON body.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}

	s.Accounts.LoadSnapshot(snap.Accounts)
	s.ExternalAccts.LoadSnapshot(snap.ExternalAccts)
	s.Transfers.LoadSnapshot(snap.Transfers)
	s.Payouts.LoadSnapshot(snap.Payouts)
	s.Events.LoadSnapshot(snap.Events)

	s.mu.Lock()
	defer s.mu.Unlock()
	if snap.Balances != nil {
		s.Balances = snap.Balances
	}
	if snap.PlatformBalance != nil {
		s.PlatformBalance = snap.PlatformBalance
	}
	return nil
}

// Reset clears all state.
func (s *MemoryStore) Reset() {
	s.Accounts.Reset()
	s.ExternalAccts.Reset()
	s.Transfers.Reset()
	s.Payouts.Reset()
	s.Events.Reset()
	s.Clock.Reset()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.Balances = make(map[string]*AccountBalance)
	s.PlatformBalance = NewAccountBalance()
}
