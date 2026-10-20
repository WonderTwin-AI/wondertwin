// Package store holds the Xero app emulator's state in memory.
package store

import (
	"encoding/json"
	"sync/atomic"

	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// MemoryStore holds all Xero emulator state in memory. Each slice that adds a
// record type adds its store here, with its snapshot, load and reset.
type MemoryStore struct {
	Clock         *pkgstate.Clock
	AccessTokens  *pkgstate.Store[AccessToken]
	RefreshTokens *pkgstate.Store[RefreshToken]
	Connections   *pkgstate.Store[Connection]
	// Organisations is keyed by tenant id; Accounts holds one chart per tenant.
	Organisations *pkgstate.Store[Organisation]
	Accounts      *Tenanted[Account]
	Contacts      *Tenanted[Contact]
	Invoices      *Tenanted[Invoice]
	Webhooks      *pkgstate.Store[Webhook]
	// WebhookSeq numbers the events of every delivery, in order.
	WebhookSeq atomic.Int64
	onReset    []func()
	// StrictAuth makes the emulator refuse bearer and refresh tokens it did not
	// issue, as Xero does. Off by default: localstack/finance-ops seeds made-up
	// tokens and relies on xero-local accepting any. It is configuration, so
	// neither reset nor a state file touches it.
	StrictAuth atomic.Bool
	// Rand is the deterministic random source behind NewID. New seeds it so a
	// bare store works; main.go replaces it with twin.Rand.
	Rand *sim.Rand
}

// New creates a MemoryStore holding the default connection.
func New() *MemoryStore {
	s := &MemoryStore{
		Clock:         pkgstate.NewClock(),
		AccessTokens:  pkgstate.New[AccessToken]("at"),
		RefreshTokens: pkgstate.New[RefreshToken]("rt"),
		Connections:   pkgstate.New[Connection]("conn"),
		Organisations: pkgstate.New[Organisation]("org"),
		Accounts:      NewTenanted[Account](),
		Webhooks:      pkgstate.New[Webhook]("wh"),
		Contacts:      NewTenanted[Contact](),
		Invoices:      NewTenanted[Invoice](),
		Rand:          sim.DefaultRand(),
	}
	s.seedConnection()
	s.seedTenant()
	return s
}

// snapshot is the JSON form of the store. A field a state file leaves out
// leaves that store as it is.
type snapshot struct {
	AccessTokens  map[string]AccessToken        `json:"access_tokens"`
	RefreshTokens map[string]RefreshToken       `json:"refresh_tokens"`
	Connections   map[string]Connection         `json:"connections"`
	Organisations map[string]Organisation       `json:"organisations"`
	Accounts      map[string]map[string]Account `json:"accounts"`
	Webhooks      map[string]Webhook            `json:"webhooks"`
	Contacts      map[string]map[string]Contact `json:"contacts"`
	Invoices      map[string]map[string]Invoice `json:"invoices"`
}

// Snapshot returns the full state for GET /admin/state.
func (s *MemoryStore) Snapshot() any {
	return snapshot{
		AccessTokens:  s.AccessTokens.Snapshot(),
		RefreshTokens: s.RefreshTokens.Snapshot(),
		Connections:   s.Connections.Snapshot(),
		Organisations: s.Organisations.Snapshot(),
		Accounts:      s.Accounts.Snapshot(),
		Webhooks:      s.Webhooks.Snapshot(),
		Contacts:      s.Contacts.Snapshot(),
		Invoices:      s.Invoices.Snapshot(),
	}
}

// LoadState loads a snapshot or seed file for POST /admin/state.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if snap.AccessTokens != nil {
		s.AccessTokens.LoadSnapshot(snap.AccessTokens)
	}
	if snap.RefreshTokens != nil {
		s.RefreshTokens.LoadSnapshot(snap.RefreshTokens)
	}
	if snap.Connections != nil {
		s.Connections.LoadSnapshot(snap.Connections)
	}
	if snap.Organisations != nil {
		s.Organisations.LoadSnapshot(snap.Organisations)
	}
	if snap.Accounts != nil {
		s.Accounts.Load(snap.Accounts)
	}
	if snap.Webhooks != nil {
		s.Webhooks.LoadSnapshot(snap.Webhooks)
	}
	if snap.Contacts != nil {
		s.Contacts.Load(snap.Contacts)
	}
	if snap.Invoices != nil {
		s.Invoices.Load(snap.Invoices)
	}
	return nil
}

// Reset clears every store and the simulated clock, and restores the default
// connection.
func (s *MemoryStore) Reset() {
	s.Clock.Reset()
	s.AccessTokens.Reset()
	s.RefreshTokens.Reset()
	s.Connections.Reset()
	s.Organisations.Reset()
	s.Accounts.Reset()
	s.Webhooks.Reset()
	s.WebhookSeq.Store(0)
	for _, f := range s.onReset {
		f()
	}
	s.Contacts.Reset()
	s.Invoices.Reset()
	s.seedConnection()
	s.seedTenant()
}

// OnReset registers a function Reset calls last, so state the store does not own
// (the webhook dispatcher's queue and delivery log) is cleared with it.
func (s *MemoryStore) OnReset(f func()) { s.onReset = append(s.onReset, f) }
