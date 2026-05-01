package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
)

// TestDeterminism_Resend asserts byte-for-byte reproducibility per
// seed across resource creation (domain + API key + audience), the
// canonical leaky path (resendID-driven hex tokens), and a 404
// lookup.
func TestDeterminism_Resend(t *testing.T) {
	hdr := map[string]string{"Authorization": "Bearer re_sim_test_123"}
	scenario := []testutil.Request{
		{
			Method:  "POST",
			Path:    "/domains",
			Headers: hdr,
			Body:    `{"name":"acme.example","region":"us-east-1"}`,
		},
		{
			Method:  "POST",
			Path:    "/api-keys",
			Headers: hdr,
			Body:    `{"name":"production"}`,
		},
		{
			Method:  "POST",
			Path:    "/audiences",
			Headers: hdr,
			Body:    `{"name":"newsletter"}`,
		},
		{
			Method:  "GET",
			Path:    "/emails/missing",
			Headers: hdr,
		},
	}
	testutil.AssertDeterministic(t, newDeterministicResend(t), 42, scenario)
}

func newDeterministicResend(t *testing.T) func(int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
	t.Helper()
	return func(seed int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
		cfg := &twincore.Config{Name: "twin-resend-determinism"}
		twin := twincore.New(cfg)
		memStore := store.New()
		memStore.Rand = twin.Rand

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
