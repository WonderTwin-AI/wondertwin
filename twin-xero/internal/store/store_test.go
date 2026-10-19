package store_test

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
)

var guid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestStoreNewIDIsALowercaseGUID(t *testing.T) {
	s := store.New()
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id := s.NewID()
		if !guid.MatchString(id) {
			t.Fatalf("%q is not a lowercase v4 GUID", id)
		}
		if seen[id] {
			t.Fatalf("%q issued twice", id)
		}
		seen[id] = true
	}
}

func TestStoreNewIDReplaysForTheSameSeed(t *testing.T) {
	a, b := store.New(), store.New()
	a.Rand, b.Rand = sim.NewRand(7), sim.NewRand(7)
	for i := 0; i < 5; i++ {
		if x, y := a.NewID(), b.NewID(); x != y {
			t.Fatalf("draw %d: %s != %s", i, x, y)
		}
	}
}

func TestStoreTenantedKeepsTenantsApart(t *testing.T) {
	ts := store.NewTenanted[string]()
	ts.For("t1").Set("a", "one")
	ts.For("t2").Set("a", "two")

	if got, _ := ts.For("t1").Get("a"); got != "one" {
		t.Fatalf("t1 a = %q", got)
	}
	if _, ok := ts.For("t3").Get("a"); ok {
		t.Fatal("a record leaked into a tenant that never wrote it")
	}
}

func TestStoreTenantedSnapshotLoadAndReset(t *testing.T) {
	ts := store.NewTenanted[string]()
	ts.For("t1").Set("a", "one")
	ts.For("t2").Set("b", "two")
	snap := ts.Snapshot()

	ts.Reset()
	if n := ts.For("t1").Count(); n != 0 {
		t.Fatalf("t1 holds %d after reset", n)
	}

	ts.Load(snap)
	if got, ok := ts.For("t2").Get("b"); !ok || got != "two" {
		t.Fatalf("t2 b = %q, %v after load", got, ok)
	}
	if n := ts.For("t1").Count(); n != 1 {
		t.Fatalf("t1 holds %d after load, want 1", n)
	}
}

func TestStoreAccessTokensSnapshotLoadAndReset(t *testing.T) {
	s := store.New()
	s.AccessTokens.Set("tok", store.AccessToken{ClientID: "app", Scope: "accounting.contacts"})
	s.RefreshTokens.Set("rt", store.RefreshToken{ClientID: "app", Used: true})
	snap := s.Snapshot()

	s.Reset()
	if s.AccessTokens.Count() != 0 || s.RefreshTokens.Count() != 0 {
		t.Fatal("reset left tokens behind")
	}
	b, _ := json.Marshal(snap)
	if err := s.LoadState(b); err != nil {
		t.Fatal(err)
	}
	if got, ok := s.AccessTokens.Get("tok"); !ok || got.ClientID != "app" {
		t.Fatalf("token = %+v, %v after load", got, ok)
	}
	if got, ok := s.RefreshTokens.Get("rt"); !ok || !got.Used {
		t.Fatalf("refresh token = %+v, %v after load", got, ok)
	}
	if err := s.LoadState([]byte(`{}`)); err != nil || s.AccessTokens.Count() != 1 || s.RefreshTokens.Count() != 1 {
		t.Fatal("a state file that omits the tokens must leave them alone")
	}
}
