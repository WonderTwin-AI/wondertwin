package store

import "time"

// AccessToken is an issued access token, keyed in the store by the token
// string. A bearer call is valid while the simulated clock is before ExpiresAt.
type AccessToken struct {
	ClientID    string    `json:"client_id"`
	Scope       string    `json:"scope"`
	AuthEventID string    `json:"authentication_event_id"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// RefreshToken is an issued refresh token, keyed by the token string. Xero
// refresh tokens are single use: redeeming one marks it Used and issues the next.
type RefreshToken struct {
	ClientID    string `json:"client_id"`
	Scope       string `json:"scope"`
	AuthEventID string `json:"authentication_event_id"`
	Used        bool   `json:"used"`
}
