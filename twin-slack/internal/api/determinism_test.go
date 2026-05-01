package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// TestDeterminism_Slack asserts that twin-slack produces byte-for-
// byte identical canonical replay JSONL across runs with the same
// seed. Scenario covers chat post, conversations create, OAuth
// access (returns a fixed token — already deterministic), pins add,
// and a 404 lookup.
func TestDeterminism_Slack(t *testing.T) {
	hdr := map[string]string{"Authorization": "Bearer xoxb-test-token"}
	scenario := []testutil.Request{
		{
			Method:  "POST",
			Path:    "/api/conversations.create",
			Headers: hdr,
			Body:    `{"name":"announce"}`,
		},
		{
			Method:  "POST",
			Path:    "/api/chat.postMessage",
			Headers: hdr,
			Body:    `{"channel":"C_GENERAL","text":"hello world"}`,
		},
		{
			Method:  "POST",
			Path:    "/api/oauth.v2.access",
			Headers: hdr,
		},
		{
			Method:  "POST",
			Path:    "/api/auth.test",
			Headers: hdr,
		},
		{
			Method:  "POST",
			Path:    "/api/conversations.info",
			Headers: hdr,
			Body:    `{"channel":"C_MISSING"}`,
		},
	}
	testutil.AssertDeterministic(t, newDeterministicSlack(t), 42, scenario)
}

func newDeterministicSlack(t *testing.T) func(int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
	t.Helper()
	return func(seed int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
		cfg := &twincore.Config{Name: "twin-slack-determinism"}
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
