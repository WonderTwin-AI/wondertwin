package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/store"
)

// TestDeterminism_PostHog asserts that twin-posthog produces
// byte-for-byte identical canonical replay JSONL across runs with
// the same seed. PostHog's surface is event capture + feature flag
// decide; both endpoints assign deterministic counter IDs from
// state.Store.NextID, and timestamps come from the simulated clock
// pinned by /admin/runs/start.
func TestDeterminism_PostHog(t *testing.T) {
	scenario := []testutil.Request{
		{
			Method: "POST",
			Path:   "/capture/",
			Body:   `{"api_key":"phc_sim_test","event":"signup","distinct_id":"alice","properties":{"plan":"pro"}}`,
		},
		{
			Method: "POST",
			Path:   "/capture/",
			Body:   `{"api_key":"phc_sim_test","event":"checkout","distinct_id":"alice","properties":{"amount":42}}`,
		},
		{
			Method: "POST",
			Path:   "/decide/",
			Body:   `{"api_key":"phc_sim_test","distinct_id":"alice"}`,
		},
		{
			Method: "POST",
			Path:   "/batch/",
			Body:   `{"api_key":"phc_sim_test","batch":[{"event":"page_view","distinct_id":"alice"}]}`,
		},
	}
	testutil.AssertDeterministic(t, newDeterministicPostHog(t), 42, scenario)
}

func newDeterministicPostHog(t *testing.T) func(int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
	t.Helper()
	return func(seed int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
		cfg := &twincore.Config{Name: "twin-posthog-determinism"}
		twin := twincore.New(cfg)
		memStore := store.New()

		handler := api.NewHandler(memStore, twin.Middleware(), nil)
		handler.Routes(twin.Router)

		adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
		adminHandler.WireFromTwin(twin)
		adminHandler.Routes(twin.Router)

		srv := httptest.NewServer(twin.Router)
		tc := testutil.NewTwinClient(t, srv)
		ac := testutil.NewAdminClient(tc)
		return tc, ac, func() {
			srv.Close()
			twin.Close()
		}
	}
}
