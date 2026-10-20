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

func TestStoreDefaultConnectionSurvivesResetAndLoad(t *testing.T) {
	s := store.New()
	if c, ok := s.Connections.Get(store.DefaultConnectionID); !ok || c.TenantID != store.DefaultTenantID {
		t.Fatalf("a new store must hold the default connection, got %+v %v", c, ok)
	}
	s.Connections.Set("extra", store.Connection{ID: "extra"})
	s.Reset()
	if s.Connections.Count() != 1 {
		t.Fatalf("reset must leave exactly the default connection, found %d", s.Connections.Count())
	}
	if err := s.LoadState([]byte(`{"connections":{"c1":{"id":"c1","tenantId":"t1"}}}`)); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Connections.Get("c1"); !ok || s.Connections.Count() != 1 {
		t.Fatal("a seeded connections map replaces the store")
	}
}

func TestStoreDefaultOrganisationBelongsToTheTenantFinanceOpsSeeds(t *testing.T) {
	if store.DefaultTenantID != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("the default tenant id must be the one localstack/finance-ops seeds, got %s", store.DefaultTenantID)
	}
	s := store.New()
	o, ok := s.Organisations.Get(store.DefaultTenantID)
	if !ok || o.Name != store.DefaultTenantName || o.Version != "GLOBAL" {
		t.Fatalf("organisation = %+v, %v", o, ok)
	}
	s.Organisations.Set("other", store.Organisation{Name: "x"})
	s.Reset()
	if _, ok := s.Organisations.Get("other"); ok || s.Organisations.Count() != 1 {
		t.Fatal("reset must leave exactly the default organisation")
	}
	if err := s.LoadState([]byte(`{"organisations":{"t1":{"Name":"Seeded"}}}`)); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Organisations.Get("t1"); got.Name != "Seeded" || s.Organisations.Count() != 1 {
		t.Fatal("a seeded organisations map replaces the store")
	}
}

func TestStoreChartIsSeededResetAndLoadedPerTenant(t *testing.T) {
	s := store.New()
	if n := s.Accounts.For(store.DefaultTenantID).Count(); n != 8 {
		t.Fatalf("the default chart holds %d accounts, want 8", n)
	}
	s.Accounts.For("other").Set("x", store.Account{Code: "999"})
	s.Reset()
	if s.Accounts.For("other").Count() != 0 || s.Accounts.For(store.DefaultTenantID).Count() != 8 {
		t.Fatal("reset must restore exactly the default chart")
	}
	if err := s.LoadState([]byte(`{"accounts":{"t1":{"a":{"Code":"1"}}}}`)); err != nil {
		t.Fatal(err)
	}
	if s.Accounts.For("t1").Count() != 1 || s.Accounts.For(store.DefaultTenantID).Count() != 0 {
		t.Fatal("a seeded accounts map replaces every tenant's chart")
	}
}

func TestStoreWebhookEmitStateSurvivesSnapshotAndResets(t *testing.T) {
	s := store.New()
	s.Webhooks.Set("w1", store.Webhook{ID: "w1", URL: "http://r", Key: "k", Categories: []string{"CONTACT"}, Status: store.WebhookOK})
	snap := s.Snapshot()

	var hooked int
	s.OnReset(func() { hooked++ })
	s.WebhookSeq.Store(9)
	s.Reset()
	if s.Webhooks.Count() != 0 || s.WebhookSeq.Load() != 0 || hooked != 1 {
		t.Fatalf("reset: %d subscriptions, sequence %d, hook ran %d times", s.Webhooks.Count(), s.WebhookSeq.Load(), hooked)
	}
	b, _ := json.Marshal(snap)
	if err := s.LoadState(b); err != nil {
		t.Fatal(err)
	}
	if w, ok := s.Webhooks.Get("w1"); !ok || w.Key != "k" || w.Status != store.WebhookOK {
		t.Fatalf("subscription = %+v, %v after load", w, ok)
	}
	if err := s.LoadState([]byte(`{}`)); err != nil || s.Webhooks.Count() != 1 {
		t.Fatal("a state file that omits webhooks must leave them alone")
	}
}
