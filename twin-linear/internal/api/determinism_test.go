package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-linear/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-linear/internal/store"
)

// TestDeterminism_Linear asserts byte-for-byte reproducibility per
// seed across the GraphQL surface (issue create, viewer query) plus
// a 401 path. Team is pre-seeded via /admin/state so issueCreate has
// a parent team; the seed is deterministic content.
func TestDeterminism_Linear(t *testing.T) {
	hdr := map[string]string{"Authorization": "lin_api_test_key"}
	teamSeed := `{"teams":{"team_001":{"id":"team_001","name":"Engineering","key":"ENG"}}}`
	scenario := []testutil.Request{
		{
			Method:  "POST",
			Path:    "/admin/state",
			Headers: hdr,
			Body:    teamSeed,
		},
		{
			Method:  "POST",
			Path:    "/graphql",
			Headers: hdr,
			Body:    `{"query":"{ viewer { id } }"}`,
		},
		{
			Method:  "POST",
			Path:    "/graphql",
			Headers: hdr,
			Body:    `{"query":"mutation { issueCreate(input: {title: \"Bug report\", teamId: \"team_001\"}) { success issue { id title identifier } } }"}`,
		},
		{
			Method:  "POST",
			Path:    "/graphql",
			Headers: hdr,
			Body:    `{"query":"mutation { issueCreate(input: {title: \"Second issue\", teamId: \"team_001\"}) { success issue { id title identifier } } }"}`,
		},
		{
			Method: "POST",
			Path:   "/graphql",
			Body:   `{"query":"{ viewer { id } }"}`,
		},
	}
	testutil.AssertDeterministic(t, newDeterministicLinear(t), 42, scenario)
}

func newDeterministicLinear(t *testing.T) func(int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
	t.Helper()
	return func(seed int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
		cfg := &twincore.Config{Name: "twin-linear-determinism"}
		twin := twincore.New(cfg)
		memStore := store.New()

		handler := api.NewHandler(memStore, twin.Middleware())
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
