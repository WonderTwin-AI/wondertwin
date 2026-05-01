package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-logodev/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-logodev/internal/store"
)

// TestDeterminism_Logodev asserts byte-for-byte reproducibility per
// seed across the basic asset-lookup surface plus the request log.
// The pre-#234 leak (RecordRequest using time.Now()) is exactly what
// this scenario exercises.
func TestDeterminism_Logodev(t *testing.T) {
	scenario := []testutil.Request{
		{Method: "GET", Path: "/example.com?token=pk_test_123"},
		{Method: "GET", Path: "/wikipedia.org?token=pk_test_123&size=64"},
		{Method: "GET", Path: "/api/v1/search?q=tesla&token=pk_test_123"},
		{Method: "GET", Path: "/missing.invalid?token=pk_test_123"},
	}
	testutil.AssertDeterministic(t, newDeterministicLogodev(t), 42, scenario)
}

func newDeterministicLogodev(t *testing.T) func(int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
	t.Helper()
	return func(seed int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
		cfg := &twincore.Config{Name: "twin-logodev-determinism"}
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
