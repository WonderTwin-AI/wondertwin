package store

import (
	"encoding/json"
	"sync"
	"testing"
)

// populated makes a store holding one of everything a snapshot carries.
func populated(seed int64) *MemoryStore {
	s := seeded(seed)
	s.CreatePublicToken("ins_1", []string{"auth"}, "") // left unspent
	spent := s.CreatePublicToken("ins_1", []string{"auth", "transactions"}, "")
	it, _, _ := s.ExchangePublicToken(spent.Token)
	s.Accounts.Set("acct-1", Account{ID: "acct-1", ItemID: it.ID})
	s.CreateLinkToken(LinkToken{UserID: "u1"})
	s.CreateProcessorToken(it.ID, "acct-1", "stripe")
	return s
}

func TestSnapshotRoundTripsEveryTokenStore(t *testing.T) {
	a := populated(1)
	raw, err := json.Marshal(a.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	b := seeded(2)
	if err := b.LoadState(raw); err != nil {
		t.Fatal(err)
	}
	for name, counts := range map[string][2]int{
		"items":            {a.Items.Count(), b.Items.Count()},
		"public tokens":    {a.PublicTokens.Count(), b.PublicTokens.Count()},
		"access tokens":    {a.AccessTokens.Count(), b.AccessTokens.Count()},
		"link tokens":      {a.LinkTokens.Count(), b.LinkTokens.Count()},
		"processor tokens": {a.ProcessorTokens.Count(), b.ProcessorTokens.Count()},
	} {
		if counts[0] == 0 || counts[0] != counts[1] {
			t.Errorf("%s: %d before, %d after the round trip", name, counts[0], counts[1])
		}
	}
	for _, at := range a.AccessTokens.List() {
		if got, ok := b.AccessTokens.Get(at.Token); !ok || got.ItemID != at.ItemID {
			t.Errorf("access token %s did not survive: %+v", at.Token, got)
		}
	}
	for _, pt := range a.PublicTokens.List() {
		if got, ok := b.PublicTokens.Get(pt.Token); !ok || !got.ExpiresAt.Equal(pt.ExpiresAt) {
			t.Errorf("public token %s did not survive: %+v", pt.Token, got)
		}
	}
}

// The Rand is not in a snapshot, so a process started with the same seed that
// loads a snapshot draws the values that made it again.
func TestSeededReloadNeverReissuesAnIssuedIDOrToken(t *testing.T) {
	a := populated(7)
	raw, _ := json.Marshal(a.Snapshot())
	b := seeded(7)
	if err := b.LoadState(raw); err != nil {
		t.Fatal(err)
	}

	pt := b.CreatePublicToken("ins_1", []string{"auth"}, "")
	if _, taken := a.PublicTokens.Get(pt.Token); taken {
		t.Fatalf("public token %s was issued twice", pt.Token)
	}
	if _, taken := a.LinkTokens.Get(b.CreateLinkToken(LinkToken{UserID: "u2"}).Token); taken {
		t.Fatal("a link token was issued twice")
	}
	it, at, err := b.ExchangePublicToken(pt.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, taken := a.Items.Get(it.ID); taken {
		t.Fatalf("item id %s was issued twice", it.ID)
	}
	if _, taken := a.AccessTokens.Get(at.Token); taken {
		t.Fatalf("access token %s was issued twice", at.Token)
	}
	if b.Items.Count() != a.Items.Count()+1 {
		t.Fatalf("the new Item replaced a loaded one: %d Items, want %d", b.Items.Count(), a.Items.Count()+1)
	}
}

func TestUpdateItemKeepsConcurrentChangesToDifferentFields(t *testing.T) {
	s := seeded(1)
	s.PutItem(Item{ID: "item-1"})
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.UpdateItem("item-1", func(it *Item) { it.NotifiedChanges++ })
		}()
		go func() {
			defer wg.Done()
			s.UpdateItem("item-1", func(it *Item) { it.SyncCalled, it.Webhook = true, "https://example.test/hook" })
		}()
	}
	wg.Wait()
	it, _ := s.GetItem("item-1")
	if it.NotifiedChanges != 200 || !it.SyncCalled || it.Webhook != "https://example.test/hook" {
		t.Fatalf("an update was lost: %+v", it)
	}
	if _, ok := s.UpdateItem("gone", func(*Item) { t.Fatal("ran on a missing Item") }); ok {
		t.Fatal("UpdateItem reported a missing Item as found")
	}
}
