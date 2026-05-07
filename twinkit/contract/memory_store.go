package contract

import "sync"

// MemoryStore is a reference in-memory StateStore for tests and simple
// twins. Stores deep copies on Save so external mutations don't leak
// into engine state, and returns deep copies from Load.
type MemoryStore struct {
	mu        sync.RWMutex
	contracts map[string]*Contract
	order     []string
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{contracts: make(map[string]*Contract)}
}

// Save stores a deep copy of the contract.
func (m *MemoryStore) Save(c *Contract) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.contracts[c.ID]; !exists {
		m.order = append(m.order, c.ID)
	}
	m.contracts[c.ID] = cloneContract(c)
	return nil
}

// Load returns a deep copy of the contract.
func (m *MemoryStore) Load(id string) (*Contract, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.contracts[id]
	if !ok {
		return nil, ErrContractNotFound
	}
	return cloneContract(c), nil
}

// List returns deep copies of all contracts in insertion order.
func (m *MemoryStore) List() ([]*Contract, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Contract, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, cloneContract(m.contracts[id]))
	}
	return out, nil
}

// Reset clears all contracts.
func (m *MemoryStore) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.contracts = make(map[string]*Contract)
	m.order = nil
}

func cloneContract(c *Contract) *Contract {
	if c == nil {
		return nil
	}
	cp := *c
	cp.Parties = cloneParties(c.Parties)
	cp.Documents = cloneDocuments(c.Documents)
	cp.Metadata = cloneMetadata(c.Metadata)
	if c.AuditLog != nil {
		cp.AuditLog = make([]AuditEntry, len(c.AuditLog))
		for i, a := range c.AuditLog {
			cp.AuditLog[i] = a
			if a.Extras != nil {
				m := make(map[string]any, len(a.Extras))
				for k, v := range a.Extras {
					m[k] = v
				}
				cp.AuditLog[i].Extras = m
			}
		}
	}
	return &cp
}
