package store

import (
	"encoding/json"
	"sync"

	pkgstore "github.com/wondertwin-ai/wondertwin/twinkit/store"
)

// MemoryStore holds all Clerk twin state in memory.
type MemoryStore struct {
	mu sync.RWMutex

	Users         *pkgstore.Store[User]
	Sessions      *pkgstore.Store[Session]
	Organizations *pkgstore.Store[Organization]
	OrgMembers    *pkgstore.Store[OrgMembership]
	Clients       *pkgstore.Store[Client]
	SignIns       *pkgstore.Store[SignIn]

	Clock *pkgstore.Clock
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	return &MemoryStore{
		Users:         pkgstore.New[User]("user"),
		Sessions:      pkgstore.New[Session]("sess"),
		Organizations: pkgstore.New[Organization]("org"),
		OrgMembers:    pkgstore.New[OrgMembership]("orgmem"),
		Clients:       pkgstore.New[Client]("client"),
		SignIns:       pkgstore.New[SignIn]("sini"),
		Clock:         pkgstore.NewClock(),
	}
}

// stateSnapshot is the JSON-serializable state for admin endpoints.
type stateSnapshot struct {
	Users         map[string]User          `json:"users"`
	Sessions      map[string]Session       `json:"sessions"`
	Organizations map[string]Organization  `json:"organizations"`
	OrgMembers    map[string]OrgMembership `json:"org_members"`
	Clients       map[string]Client        `json:"clients,omitempty"`
	SignIns       map[string]SignIn         `json:"sign_ins,omitempty"`
}

// Snapshot returns the full state as a JSON-serializable value.
func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Users:         s.Users.Snapshot(),
		Sessions:      s.Sessions.Snapshot(),
		Organizations: s.Organizations.Snapshot(),
		OrgMembers:    s.OrgMembers.Snapshot(),
		Clients:       s.Clients.Snapshot(),
		SignIns:       s.SignIns.Snapshot(),
	}
}

// LoadState replaces the full state from a JSON body.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}

	s.Users.LoadSnapshot(snap.Users)
	s.Sessions.LoadSnapshot(snap.Sessions)
	s.Organizations.LoadSnapshot(snap.Organizations)
	s.OrgMembers.LoadSnapshot(snap.OrgMembers)
	if snap.Clients != nil {
		s.Clients.LoadSnapshot(snap.Clients)
	}
	if snap.SignIns != nil {
		s.SignIns.LoadSnapshot(snap.SignIns)
	}
	return nil
}

// Reset clears all state.
func (s *MemoryStore) Reset() {
	s.Users.Reset()
	s.Sessions.Reset()
	s.Organizations.Reset()
	s.OrgMembers.Reset()
	s.Clients.Reset()
	s.SignIns.Reset()
	s.Clock.Reset()
}
