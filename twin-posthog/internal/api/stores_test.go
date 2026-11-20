package api_test

import (
	"testing"
	"time"
)

// TestStoresResetState holds admin/reset and admin/state for the typed event,
// person, alias and group stores: what capture writes is in the state export,
// reset clears all of it, and loading the export brings it back unchanged.
func TestStoresResetState(t *testing.T) {
	_, tc := setupPostHog(t)

	tc.Post("/batch", map[string]any{
		"api_key": "phc_state_token",
		"batch": []map[string]any{
			{"event": "signup", "distinct_id": "u1"},
			{"event": "$identify", "distinct_id": "u1", "properties": map[string]any{
				"$set": map[string]any{"plan": "pro"},
			}},
			{"event": "$create_alias", "distinct_id": "u1", "properties": map[string]any{"alias": "u1-alias"}},
			{"event": "$groupidentify", "distinct_id": "u1", "properties": map[string]any{
				"$group_type": "org", "$group_key": "acme", "$group_set": map[string]any{"name": "Acme"},
			}},
		},
	}).AssertStatus(200)

	snapshot := tc.Get("/admin/state")
	snapshot.AssertStatus(200)
	state := snapshot.JSONMap()

	counts := map[string]int{"events": 4, "persons": 1, "aliases": 1, "groups": 1}
	for store, want := range counts {
		got, ok := state[store].(map[string]any)
		if !ok || len(got) != want {
			t.Fatalf("state[%s]: want %d records, got %v", store, want, state[store])
		}
	}

	t.Run("events carry the token and the emulator clock", func(t *testing.T) {
		for id, raw := range state["events"].(map[string]any) {
			evt := raw.(map[string]any)
			if evt["token"] != "phc_state_token" {
				t.Errorf("event %s: token = %v, want phc_state_token", id, evt["token"])
			}
			received, _ := evt["received_at"].(string)
			if _, err := time.Parse(time.RFC3339, received); err != nil {
				t.Errorf("event %s: received_at %q is not RFC3339: %v", id, received, err)
			}
		}
	})

	t.Run("reset clears every store", func(t *testing.T) {
		tc.Post("/admin/reset", nil).AssertStatus(200)
		cleared := tc.Get("/admin/state").JSONMap()
		for store := range counts {
			if got, _ := cleared[store].(map[string]any); len(got) != 0 {
				t.Errorf("after reset, %s still holds %v", store, got)
			}
		}
	})

	t.Run("loading the exported state restores every store", func(t *testing.T) {
		tc.Post("/admin/state", state).AssertStatus(200)
		restored := tc.Get("/admin/state").JSONMap()
		for store, want := range counts {
			if got, _ := restored[store].(map[string]any); len(got) != want {
				t.Errorf("after load, %s holds %d records, want %d", store, len(got), want)
			}
		}
		for id, raw := range restored["events"].(map[string]any) {
			if raw.(map[string]any)["token"] != "phc_state_token" {
				t.Errorf("event %s lost its token across state load", id)
			}
		}
	})
}
