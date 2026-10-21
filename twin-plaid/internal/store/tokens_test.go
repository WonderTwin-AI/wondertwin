package store

import (
	"errors"
	"sync"
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

func TestTokensConcurrentExchangeSpendsThePublicTokenOnce(t *testing.T) {
	s := seeded(1)
	pt := s.CreatePublicToken("ins_1", []string{"auth"}, "")
	const n = 20
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, errs[i] = s.ExchangePublicToken(pt.Token)
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, ErrUnknownToken):
			t.Fatalf("losing exchange error = %v, want ErrUnknownToken", err)
		}
	}
	if ok != 1 || s.Items.Count() != 1 {
		t.Fatalf("%d exchanges succeeded, %d Items; want 1 and 1", ok, s.Items.Count())
	}
}

func TestTokensRotateIsAtomicUnderConcurrency(t *testing.T) {
	s := seeded(1)
	_, at, _ := s.ExchangePublicToken(s.CreatePublicToken("ins_1", []string{"auth"}, "").Token)

	var wg sync.WaitGroup
	results := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.RotateAccessToken(at.Token)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	ok := 0
	for err := range results {
		if err == nil {
			ok++
		} else if !errors.Is(err, ErrUnknownAccessToken) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 || s.AccessTokens.Count() != 1 {
		t.Fatalf("%d rotations succeeded and %d tokens exist, want 1 and 1", ok, s.AccessTokens.Count())
	}
}

func TestTokensRemovedItemReadsAsRemovedNotUnknown(t *testing.T) {
	s := seeded(1)
	it, at, _ := s.ExchangePublicToken(s.CreatePublicToken("ins_1", []string{"auth"}, "").Token)
	s.RemoveItem(it.ID)
	if _, err := s.ItemForAccessToken(at.Token); !errors.Is(err, ErrItemRemoved) {
		t.Fatalf("removed Item error = %v, want ErrItemRemoved", err)
	}
	if _, err := s.ItemForAccessToken("access-sandbox-nope"); !errors.Is(err, ErrUnknownAccessToken) {
		t.Fatalf("unknown token error = %v, want ErrUnknownAccessToken", err)
	}
}
