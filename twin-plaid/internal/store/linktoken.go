package store

import (
	"errors"
	"time"
)

// Link tokens last 4 hours, 30 minutes in update mode (quirk LIFECYCLE-001).
const (
	linkTokenTTL       = 4 * time.Hour
	linkTokenUpdateTTL = 30 * time.Minute
)

// LinkToken is what a client configured Link with. Link itself is not
// emulated, so the token is only created and read back.
type LinkToken struct {
	Token        string    `json:"link_token"`
	ClientName   string    `json:"client_name"`
	Language     string    `json:"language"`
	CountryCodes []string  `json:"country_codes"`
	Products     []string  `json:"products,omitempty"`
	Webhook      string    `json:"webhook,omitempty"`
	RedirectURI  string    `json:"redirect_uri,omitempty"`
	UserID       string    `json:"client_user_id"`
	UpdateItemID string    `json:"update_item_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// ErrUnknownLinkToken is a link token that was never issued or has expired.
var ErrUnknownLinkToken = errors.New("unknown link token")

// CreateLinkToken issues a link token. A non-empty UpdateItemID is update mode.
func (s *MemoryStore) CreateLinkToken(t LinkToken) LinkToken {
	t.Token, t.CreatedAt = s.newToken("link"), s.Clock.Now()
	t.ExpiresAt = t.CreatedAt.Add(linkTokenTTL)
	if t.UpdateItemID != "" {
		t.ExpiresAt = t.CreatedAt.Add(linkTokenUpdateTTL)
	}
	s.LinkTokens.Set(t.Token, t)
	return t
}

// GetLinkToken returns a link token that has not expired.
func (s *MemoryStore) GetLinkToken(token string) (LinkToken, error) {
	t, ok := s.LinkTokens.Get(token)
	if !ok || !s.Clock.Now().Before(t.ExpiresAt) {
		return LinkToken{}, ErrUnknownLinkToken
	}
	return t, nil
}
