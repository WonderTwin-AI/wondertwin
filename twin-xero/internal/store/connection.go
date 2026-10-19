package store

import "time"

// The default tenant every unseeded emulator starts with. The IDs are fixed so
// they survive a reset: a client that stored the tenant id keeps a valid one.
const (
	DefaultConnectionID  = "4ae5e1de-8c3d-4d3e-9d2a-0a6f1c0de001"
	DefaultTenantID      = "9d1b6c5e-2f4a-4b7c-8e1d-3c0de0000001"
	DefaultAuthEventID   = "7c2f1a9b-5d3e-4f60-a1b2-c3d4e5f60001"
	DefaultTenantName    = "Demo Company (Global)"
	defaultTenantTypeOrg = "ORGANISATION"
)

// Connection is a tenant an app is connected to, as GET /connections lists it.
// Its dates are ISO strings with seven fractional digits and no zone (ident-002).
type Connection struct {
	ID             string    `json:"id"`
	AuthEventID    string    `json:"authEventId"`
	TenantID       string    `json:"tenantId"`
	TenantType     string    `json:"tenantType"`
	TenantName     string    `json:"tenantName"`
	CreatedDateUtc time.Time `json:"createdDateUtc"`
	UpdatedDateUtc time.Time `json:"updatedDateUtc"`
}

// seedConnection restores the default connection, dated by the simulated clock.
func (s *MemoryStore) seedConnection() {
	now := s.Clock.Now().UTC()
	s.Connections.Set(DefaultConnectionID, Connection{
		ID: DefaultConnectionID, AuthEventID: DefaultAuthEventID, TenantID: DefaultTenantID,
		TenantType: defaultTenantTypeOrg, TenantName: DefaultTenantName, CreatedDateUtc: now, UpdatedDateUtc: now,
	})
}
