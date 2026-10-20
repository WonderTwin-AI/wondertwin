package store

import (
	"encoding/json"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
)

func seeded(seed int64) *MemoryStore {
	s := New()
	s.Rand = sim.NewRand(seed)
	return s
}

func TestIDsAre37Base62CharactersAndNeverSequential(t *testing.T) {
	re := regexp.MustCompile(`^[0-9A-Za-z]{37}$`)
	s := seeded(1)
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id := s.NewID()
		if !re.MatchString(id) {
			t.Fatalf("id %q is not 37 base62 characters", id)
		}
		if seen[id] {
			t.Fatalf("id %q issued twice", id)
		}
		seen[id] = true
	}
}

func TestIDsAreReissuedBySeed(t *testing.T) {
	if seeded(5).NewID() != seeded(5).NewID() {
		t.Fatal("same seed gave different ids")
	}
	if seeded(5).NewID() == seeded(6).NewID() {
		t.Fatal("different seeds gave the same id")
	}
}

func TestAccountsForItemFollowPositionNotInsertion(t *testing.T) {
	s := seeded(1)
	s.PutAccount(Account{ID: "c", ItemID: "item1", Position: 2, Name: "Savings"})
	s.PutAccount(Account{ID: "a", ItemID: "item1", Position: 0, Name: "Checking"})
	s.PutAccount(Account{ID: "b", ItemID: "item2", Position: 0, Name: "Other item"})
	s.PutAccount(Account{ID: "d", ItemID: "item1", Position: 1, Name: "Credit"})

	var names []string
	for _, a := range s.AccountsForItem("item1") {
		names = append(names, a.Name)
	}
	if want := []string{"Checking", "Credit", "Savings"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("accounts = %v, want %v", names, want)
	}
}

func TestRemoveItemDeletesItsAccountsOnly(t *testing.T) {
	s := seeded(1)
	s.PutItem(Item{ID: "item1"})
	s.PutItem(Item{ID: "item2"})
	s.PutAccount(Account{ID: "a", ItemID: "item1"})
	s.PutAccount(Account{ID: "b", ItemID: "item2"})

	if !s.RemoveItem("item1") {
		t.Fatal("RemoveItem reported the item missing")
	}
	if _, ok := s.GetItem("item1"); ok {
		t.Fatal("item still present")
	}
	if _, ok := s.GetAccount("a"); ok {
		t.Fatal("account of the removed item still present")
	}
	if _, ok := s.GetAccount("b"); !ok {
		t.Fatal("account of another item was removed")
	}
	if s.RemoveItem("item1") {
		t.Fatal("removing a missing item reported success")
	}
}

func TestInstitutionsListInIDOrder(t *testing.T) {
	s := seeded(1)
	for _, id := range []string{"ins_56", "ins_109508", "ins_127287"} {
		s.PutInstitution(Institution{ID: id, Name: id})
	}
	var ids []string
	for _, in := range s.ListInstitutions() {
		ids = append(ids, in.ID)
	}
	if want := []string{"ins_109508", "ins_127287", "ins_56"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
}

func TestStateRoundTripsAndResetClears(t *testing.T) {
	s := seeded(1)
	one := 12.5
	s.PutInstitution(Institution{ID: "ins_1", Name: "Bank", Products: []string{"auth"}, OAuth: true})
	s.PutItem(Item{ID: "item1", InstitutionID: "ins_1", Webhook: "http://x/hook",
		CreatedAt: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)})
	s.PutAccount(Account{ID: "a", ItemID: "item1", Position: 1, Name: "Checking",
		PersistentID: "p1", Balances: Balances{Available: &one, ISOCurrencyCode: "USD"}})

	raw, err := json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	loaded := seeded(2)
	if err := loaded.LoadState(raw); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Snapshot(), loaded.Snapshot()) {
		t.Fatalf("state changed across a round trip:\n%s", raw)
	}

	loaded.Reset()
	if loaded.Items.Count() != 0 || loaded.Accounts.Count() != 0 || loaded.Institutions.Count() != 0 {
		t.Fatal("Reset left records behind")
	}
}

func TestLoadStateLeavesOmittedStoresAlone(t *testing.T) {
	s := seeded(1)
	s.PutItem(Item{ID: "item1"})
	if err := s.LoadState([]byte(`{"institutions": {"ins_1": {"institution_id": "ins_1", "name": "Bank"}}}`)); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetItem("item1"); !ok {
		t.Fatal("a state file without items cleared the items")
	}
	if _, ok := s.GetInstitution("ins_1"); !ok {
		t.Fatal("institution was not loaded")
	}
}
