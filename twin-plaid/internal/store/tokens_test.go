package store

import (
	"errors"
	"testing"
	"time"
)

func TestTokensExchangeSpendsThePublicTokenOnce(t *testing.T) {
	s := seeded(1)
	pt := s.CreatePublicToken("ins_1", []string{"auth"}, "")
	if s.Items.Count() != 0 || s.AccessTokens.Count() != 0 {
		t.Fatal("creating a public token made an Item or an access token")
	}
	it, at, err := s.ExchangePublicToken(pt.Token)
	if err != nil || at.ItemID != it.ID || s.Items.Count() != 1 {
		t.Fatalf("exchange: %+v %+v %v", it, at, err)
	}
	if _, _, err := s.ExchangePublicToken(pt.Token); !errors.Is(err, ErrUnknownToken) {
		t.Fatalf("second exchange error = %v, want ErrUnknownToken", err)
	}
}

func TestTokensExpireAfterThirtyMinutesOnTheEmulatorClock(t *testing.T) {
	s := seeded(1)
	fresh := s.CreatePublicToken("ins_1", []string{"auth"}, "")
	s.Clock.Advance(29 * time.Minute)
	if _, _, err := s.ExchangePublicToken(fresh.Token); err != nil {
		t.Fatalf("29 minutes in: %v", err)
	}
	stale := s.CreatePublicToken("ins_1", []string{"auth"}, "")
	s.Clock.Advance(30 * time.Minute)
	if _, _, err := s.ExchangePublicToken(stale.Token); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("30 minutes in: %v, want ErrExpiredToken", err)
	}
}

func TestTokensResetClearsThem(t *testing.T) {
	s := seeded(1)
	pt := s.CreatePublicToken("ins_1", []string{"auth"}, "")
	_, _, _ = s.ExchangePublicToken(s.CreatePublicToken("ins_1", []string{"auth"}, "").Token)
	s.Reset()
	if _, ok := s.PublicTokens.Get(pt.Token); ok || s.AccessTokens.Count() != 0 {
		t.Fatal("Reset left tokens behind")
	}
}
