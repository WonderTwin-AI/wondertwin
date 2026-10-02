package store_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

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
	src.DndStatuses["U1"] = store.DndStatus{}

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
		if got != 1 {
			t.Errorf("%s: %d items after a snapshot round trip, want 1", name, got)
		}
	}
	if _, ok := dst.DndStatuses["U1"]; !ok {
		t.Error("DndStatuses did not survive a snapshot round trip")
	}
}

func TestSnapshotDoesNotShareTheDndMap(t *testing.T) {
	s := store.New()
	s.DndStatuses["U1"] = store.DndStatus{}
	snap := s.Snapshot()
	s.DndStatuses["U2"] = store.DndStatus{}
	data, _ := json.Marshal(snap)
	if strings.Contains(string(data), "U2") {
		t.Error("a snapshot changed after it was taken")
	}
}
