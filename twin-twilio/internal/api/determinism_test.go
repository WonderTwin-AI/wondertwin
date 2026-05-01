package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-twilio/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-twilio/internal/store"
)

// TestDeterminism_Twilio asserts that twin-twilio is byte-for-byte
// reproducible per seed. Scenario covers SMS send, Verify Service +
// Verification creation, OTP check (the canonical leaky path), and
// a 404 lookup. The canonicalizer in testutil strips timestamps.
func TestDeterminism_Twilio(t *testing.T) {
	scenario := []testutil.Request{
		{
			Method:  "POST",
			Path:    "/2010-04-01/Accounts/AC_test_sim/Messages.json",
			Headers: twilioAuthHeader(),
			Body:    "From=%2B15005550006&To=%2B15555551234&Body=Hello",
		},
		{
			Method:  "POST",
			Path:    "/v2/Services",
			Headers: twilioAuthHeader(),
			Body:    "FriendlyName=otp-service",
		},
		{
			Method:  "POST",
			Path:    "/v2/Services/VA_sim/Verifications",
			Headers: twilioAuthHeader(),
			Body:    "To=%2B15555551234&Channel=sms",
		},
		{
			Method:  "GET",
			Path:    "/2010-04-01/Accounts/AC_test_sim/Messages/SM_missing.json",
			Headers: twilioAuthHeader(),
		},
	}
	testutil.AssertDeterministic(t, newDeterministicTwilio(t), 42, scenario)
}

func twilioAuthHeader() map[string]string {
	return map[string]string{"Authorization": basicAuthHeader}
}

func newDeterministicTwilio(t *testing.T) func(int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
	t.Helper()
	return func(seed int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
		cfg := &twincore.Config{Name: "twin-twilio-determinism"}
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
