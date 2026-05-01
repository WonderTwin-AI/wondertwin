package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// TestDeterminism_Stripe is the canonical determinism contract for
// twin-stripe: two seeded runs of the same scenario produce
// byte-for-byte identical replay JSONL bundles modulo the fields
// canonicalized by testutil.AssertDeterministic (timestamps,
// durations, run-bound IDs, X-Request-Id headers, /admin/* entries).
func TestDeterminism_Stripe(t *testing.T) {
	scenario := []testutil.Request{
		{
			Method:  "POST",
			Path:    "/v1/customers",
			Headers: map[string]string{"Authorization": "Bearer sk_test_sim_123"},
			Body:    "email=alice@example.com&name=Alice",
		},
		{
			Method:  "POST",
			Path:    "/v1/payment_intents",
			Headers: map[string]string{"Authorization": "Bearer sk_test_sim_123"},
			Body:    "amount=2000&currency=usd",
		},
		{
			Method:  "POST",
			Path:    "/v1/promotion_codes",
			Headers: map[string]string{"Authorization": "Bearer sk_test_sim_123"},
			Body:    "coupon=nope",
		},
		{
			Method:  "GET",
			Path:    "/v1/customers/missing",
			Headers: map[string]string{"Authorization": "Bearer sk_test_sim_123"},
		},
	}
	testutil.AssertDeterministic(t, newDeterministicStripe(t), 42, scenario)
}

// newDeterministicStripe returns a closure suitable for
// testutil.AssertDeterministic. The closure builds a fresh
// twin-stripe per invocation, wired so /admin/runs/start can reseed
// twin.Rand and pin the simulated clock.
func newDeterministicStripe(t *testing.T) func(int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
	t.Helper()
	return func(seed int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
		cfg := &twincore.Config{Name: "twin-stripe-determinism"}
		twin := twincore.New(cfg)
		memStore := store.New()
		memStore.Rand = twin.Rand

		dispatcher := webhook.NewDispatcher(webhook.Config{})
		handler := api.NewHandler(memStore, dispatcher, twin.Middleware(), nil)
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
