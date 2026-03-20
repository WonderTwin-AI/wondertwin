package store

import "time"

// Org represents an organization.
type Org struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Tier      string    `json:"tier"` // "pro" | "enterprise"
	CreatedAt time.Time `json:"created_at"`
	Active    bool      `json:"active"`
}

// License represents a twin license issued to an org.
type License struct {
	ID            string     `json:"id"` // full license key
	OrgID         string     `json:"org_id"`
	Tier          string     `json:"tier"`
	TwinName      string     `json:"twin_name"`
	TwinClass     string     `json:"twin_class"` // "standard" | "complex"
	BillingPeriod string     `json:"billing_period"` // "monthly" | "annual"
	Active        bool       `json:"active"`
	IssuedAt      time.Time  `json:"issued_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	RevokeReason  string     `json:"revoke_reason,omitempty"`
}

// IsExpired returns true if the license has an expiry date that has passed.
func (l *License) IsExpired() bool {
	if l.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*l.ExpiresAt)
}

// IsRevoked returns true if the license has been revoked.
func (l *License) IsRevoked() bool {
	return l.RevokedAt != nil
}

// Token represents an issued JIT auth token.
type Token struct {
	ID        string    `json:"id"`
	LicenseID string    `json:"license_id"`
	OrgID     string    `json:"org_id"`
	TwinName  string    `json:"twin_name"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
}
