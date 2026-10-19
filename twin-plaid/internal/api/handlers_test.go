package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func setupPlaid(t *testing.T) (*testutil.TwinClient, *testutil.AdminClient, *store.MemoryStore) {
	t.Helper()
	cfg := &twincore.Config{Name: "twin-plaid-test"}
	twin := twincore.New(cfg)
	memStore := store.New()
	memStore.Rand = twin.Rand

	api.NewHandler(memStore, twin.Middleware()).Routes(twin.Router)

	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.SetConfigProvider(twin)
	adminHandler.Routes(twin.Router)

	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	tc := testutil.NewTwinClient(t, srv)
	return tc, testutil.NewAdminClient(tc), memStore
}

func TestHealthIsExactlyOK(t *testing.T) {
	_, ac, _ := setupPlaid(t)

	resp := ac.Health().AssertStatus(200)
	if got := strings.TrimSpace(string(resp.Body)); got != `{"status":"ok"}` {
		t.Fatalf("health body = %s, want {\"status\":\"ok\"}", got)
	}
}

func TestAdminStateRoundTrip(t *testing.T) {
	_, ac, _ := setupPlaid(t)

	ac.GetState().AssertStatus(200)
	ac.LoadState(map[string]any{}).AssertStatus(200)
	ac.LoadState(map[string]any{"unknown": true}).AssertStatus(200)
}

func TestAdminStateRejectsMalformedJSON(t *testing.T) {
	tc, _, _ := setupPlaid(t)

	tc.Post("/admin/state", "{not json").AssertStatus(400)
}

func TestAdminResetClearsClock(t *testing.T) {
	_, ac, mem := setupPlaid(t)

	ac.AdvanceTime("24h").AssertStatus(200)
	if mem.Clock.Offset() != 24*time.Hour {
		t.Fatalf("offset = %s, want 24h", mem.Clock.Offset())
	}
	ac.Reset().AssertStatus(200)
	if mem.Clock.Offset() != 0 {
		t.Fatalf("offset after reset = %s, want 0", mem.Clock.Offset())
	}
}

func TestAdminConfigNamesTheEmulator(t *testing.T) {
	tc, _, _ := setupPlaid(t)

	resp := tc.Get("/admin/config").AssertStatus(200)
	if got := resp.JSONMap()["name"]; got != "twin-plaid-test" {
		t.Fatalf("config name = %v, want twin-plaid-test", got)
	}
}

func TestAdminQuirksIsAnEmptyListNotNull(t *testing.T) {
	tc, _, _ := setupPlaid(t)

	resp := tc.Get("/admin/quirks").AssertStatus(200)
	if got := strings.TrimSpace(string(resp.Body)); got != "[]" {
		t.Fatalf("quirks body = %s, want []", got)
	}
}

func TestOriginFollowsForwardedHeaders(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"direct", nil, "http://127.0.0.1:4116"},
		{"forwarded host and proto", map[string]string{
			"X-Forwarded-Host": "plaid.localhost.localstack.cloud:4566", "X-Forwarded-Proto": "https",
		}, "https://plaid.localhost.localstack.cloud:4566"},
		{"first of a list", map[string]string{
			"X-Forwarded-Host": "plaid.localhost.localstack.cloud:4566, 10.0.0.1", "X-Forwarded-Proto": "http, https",
		}, "http://plaid.localhost.localstack.cloud:4566"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4116/x", nil)
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			if got := api.Origin(r); got != tc.want {
				t.Fatalf("Origin = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestUnknownRoutesAnswerPlaidNotFound(t *testing.T) {
	tc, _, _ := setupPlaid(t)

	resp := tc.Post("/does/not/exist", "{}").AssertStatus(404)
	m := resp.JSONMap()
	if m["error_code"] != "NOT_FOUND" || m["error_message"] != "invalid route: /does/not/exist" {
		t.Fatalf("body = %v", m)
	}
	tc.Get("/accounts/get").AssertStatus(404)
}
