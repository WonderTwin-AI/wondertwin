package store

import (
	"errors"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
)

// publicTokenTTL is how long a public token can be exchanged (quirk LIFECYCLE-001).
const publicTokenTTL = 30 * time.Minute

// PublicToken is a Link result waiting to be exchanged. It carries what the
// Item will be made from; the Item itself exists only after the exchange.
type PublicToken struct {
	Token         string    `json:"public_token"`
	InstitutionID string    `json:"institution_id"`
	Products      []string  `json:"products"`
	Webhook       string    `json:"webhook,omitempty"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// AccessToken is bound to one Item and never expires (quirk AUTH-003).
type AccessToken struct {
	Token  string `json:"access_token"`
	ItemID string `json:"item_id"`
}

var (
	// ErrUnknownToken is a public token that was never issued, or was already exchanged.
	ErrUnknownToken = errors.New("unknown public token")
	// ErrExpiredToken is a public token past its 30 minutes.
	ErrExpiredToken = errors.New("expired public token")
	// ErrUnknownAccessToken is an access token that was never issued or was replaced.
	ErrUnknownAccessToken = errors.New("unknown access token")
	// ErrItemRemoved is an access token whose Item was removed (quirk LIFECYCLE-002).
	ErrItemRemoved = errors.New("item removed")
)

// newToken is {kind}-sandbox-{uuid}, the Sandbox token form (quirk IDENTITY-004).
func (s *MemoryStore) newToken(kind string) string {
	return kind + "-sandbox-" + sim.NewIDGenerator(s.Rand).UUID()
}

// CreatePublicToken issues a public token for an Item to be made at exchange.
func (s *MemoryStore) CreatePublicToken(institutionID string, products []string, webhook string) PublicToken {
	t := PublicToken{
		Token: s.newToken("public"), InstitutionID: institutionID, Products: products,
		Webhook: webhook, ExpiresAt: s.Clock.Now().Add(publicTokenTTL),
	}
	s.PublicTokens.Set(t.Token, t)
	return t
}

// ExchangePublicToken spends a public token: it creates the Item and its
// access token. A public token works once; an expired one is spent too.
func (s *MemoryStore) ExchangePublicToken(token string) (Item, AccessToken, error) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	pt, ok := s.PublicTokens.Get(token)
	if !ok {
		return Item{}, AccessToken{}, ErrUnknownToken
	}
	s.PublicTokens.Delete(token)
	if !s.Clock.Now().Before(pt.ExpiresAt) {
		return Item{}, AccessToken{}, ErrExpiredToken
	}
	it := Item{
		ID: s.NewID(), InstitutionID: pt.InstitutionID, Webhook: pt.Webhook,
		Products: pt.Products, BilledProducts: pt.Products,
		UpdateType: "background", CreatedAt: s.Clock.Now(),
	}
	s.PutItem(it)
	at := AccessToken{Token: s.newToken("access"), ItemID: it.ID}
	s.AccessTokens.Set(at.Token, at)
	return it, at, nil
}

// ItemForAccessToken returns the Item an access token is bound to. A removed
// Item keeps its token record, so it reads as ErrItemRemoved and not as an
// unknown token.
func (s *MemoryStore) ItemForAccessToken(token string) (Item, error) {
	at, ok := s.AccessTokens.Get(token)
	if !ok {
		return Item{}, ErrUnknownAccessToken
	}
	it, ok := s.GetItem(at.ItemID)
	if !ok {
		return Item{}, ErrItemRemoved
	}
	return it, nil
}

// RotateAccessToken replaces an access token with a new one for the same Item;
// the old token stops working at once.
func (s *MemoryStore) RotateAccessToken(token string) (AccessToken, error) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	it, err := s.ItemForAccessToken(token)
	if err != nil {
		return AccessToken{}, err
	}
	s.AccessTokens.Delete(token)
	at := AccessToken{Token: s.newToken("access"), ItemID: it.ID}
	s.AccessTokens.Set(at.Token, at)
	return at, nil
}
