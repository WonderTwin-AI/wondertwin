package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// TestDeterminism_GitHub asserts byte-for-byte reproducibility per
// seed across the heaviest community-twin surface: repo create,
// issue create, PR create, webhook register, webhook delivery
// fetch, app installation access-token issue (which embeds a
// MakeSHA(store.Now()) — only stable when the clock is pinned),
// and a 404 lookup.
func TestDeterminism_GitHub(t *testing.T) {
	hdr := map[string]string{"Authorization": "Bearer ghp_test_token"}
	scenario := []testutil.Request{
		{
			Method:  "POST",
			Path:    "/user/repos",
			Headers: hdr,
			Body:    `{"name":"hello-world"}`,
		},
		{
			Method:  "POST",
			Path:    "/repos/octocat/hello-world/issues",
			Headers: hdr,
			Body:    `{"title":"first issue","body":"opening up"}`,
		},
		{
			Method:  "POST",
			Path:    "/repos/octocat/hello-world/pulls",
			Headers: hdr,
			Body:    `{"title":"first pr","head":"feature","base":"main"}`,
		},
		{
			Method:  "POST",
			Path:    "/repos/octocat/hello-world/hooks",
			Headers: hdr,
			Body:    `{"config":{"url":"https://example.com/webhook","content_type":"json"}}`,
		},
		{
			Method:  "POST",
			Path:    "/app/installations/1/access_tokens",
			Headers: hdr,
		},
		{
			Method:  "POST",
			Path:    "/repos/octocat/hello-world/issues/1/comments",
			Headers: hdr,
			Body:    `{"body":"+1 from the bot"}`,
		},
		{
			Method:  "GET",
			Path:    "/repos/octocat/hello-world/issues/9999",
			Headers: hdr,
		},
		{
			Method:  "GET",
			Path:    "/rate_limit",
			Headers: hdr,
		},
	}
	testutil.AssertDeterministic(t, newDeterministicGitHub(t), 42, scenario)
}

func newDeterministicGitHub(t *testing.T) func(int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
	t.Helper()
	return func(seed int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
		cfg := &twincore.Config{Name: "twin-github-determinism"}
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
