package store_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// stores returns every *pkgstate.Store field of a MemoryStore, by field name.
func stores(s *store.MemoryStore) map[string]reflect.Value {
	out := map[string]reflect.Value{}
	v := reflect.ValueOf(s).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		if !f.IsExported() || f.Type.Kind() != reflect.Pointer {
			continue
		}
		if strings.HasPrefix(f.Type.Elem().Name(), "Store[") {
			out[f.Name] = v.Field(i)
		}
	}
	return out
}

// Every store survives a snapshot round trip. The test finds the stores by
// reflection, so a store added to MemoryStore without a place in the snapshot
// fails here instead of losing its data on restore.
func TestEveryStoreSurvivesASnapshotRoundTrip(t *testing.T) {
	src := store.New()
	all := stores(src)
	if len(all) < 10 {
		t.Fatalf("found %d stores by reflection; the lookup is broken", len(all))
	}
	for name, st := range all {
		set := st.MethodByName("Set")
		item := reflect.New(set.Type().In(1)).Elem()
		set.Call([]reflect.Value{reflect.ValueOf("seed-" + name), item})
	}

	data, err := json.Marshal(src.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	dst := store.New()
	if err := dst.LoadState(data); err != nil {
		t.Fatal(err)
	}
	for name, st := range stores(dst) {
		got := st.MethodByName("Count").Call(nil)[0].Int()
		want := all[name].MethodByName("Count").Call(nil)[0].Int()
		if got != want {
			t.Errorf("%s: %d items after a snapshot round trip, want %d", name, got, want)
		}
	}
}

// A snapshot taken from the emulator replaces every store, so a store that was
// empty when the snapshot was taken is empty again after it is loaded, and the
// default principals come back.
func TestSnapshotOfAnEmptyStoreClearsIt(t *testing.T) {
	empty := store.New()
	data, err := json.Marshal(empty.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	live := store.New()
	for name, st := range stores(live) {
		set := st.MethodByName("Set")
		set.Call([]reflect.Value{reflect.ValueOf("live-" + name), reflect.New(set.Type().In(1)).Elem()})
	}
	if err := live.LoadState(data); err != nil {
		t.Fatal(err)
	}
	for name, st := range stores(live) {
		got := st.MethodByName("Count").Call(nil)[0].Int()
		want := stores(empty)[name].MethodByName("Count").Call(nil)[0].Int()
		if got != want {
			t.Errorf("%s: %d items after loading a snapshot of an empty store, want %d", name, got, want)
		}
	}
	if _, ok := live.Users.Get(store.DefaultBotUserID); !ok {
		t.Error("the default bot user is gone after a load")
	}
}

// A seed file that names only some stores leaves the others as they are.
func TestSeedFileKeepsTheStoresItLeavesOut(t *testing.T) {
	s := store.New()
	s.Channels.Set("C1", store.Channel{ID: "C1", Name: "kept"})
	if err := s.LoadState([]byte(`{"users":{"U1":{"id":"U1","name":"alice"}}}`)); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Channels.Get("C1"); !ok {
		t.Error("a seed file with only users dropped a channel")
	}
	if _, ok := s.Users.Get("U1"); !ok {
		t.Error("the seeded user is missing")
	}
}

// The simulated clock's offset and pin survive a snapshot round trip.
func TestClockSurvivesASnapshot(t *testing.T) {
	src := store.New()
	at := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	src.Clock.Pin(at)
	src.Clock.Advance(90 * time.Minute)
	data, err := json.Marshal(src.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	dst := store.New()
	if err := dst.LoadState(data); err != nil {
		t.Fatal(err)
	}
	if got, want := dst.Clock.Now(), at.Add(90*time.Minute); !got.Equal(want) {
		t.Errorf("restored clock reads %v, want %v", got, want)
	}

	free := store.New()
	free.Clock.Advance(48 * time.Hour)
	data, _ = json.Marshal(free.Snapshot())
	dst = store.New()
	if err := dst.LoadState(data); err != nil {
		t.Fatal(err)
	}
	if got := dst.Clock.Offset(); got != 48*time.Hour {
		t.Errorf("restored offset %v, want 48h", got)
	}
}
