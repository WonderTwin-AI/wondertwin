package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-hubspot/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-hubspot/internal/store"
)

// TestDeterminism_HubSpot asserts byte-for-byte reproducibility per
// seed across the canonical CRM surface: contact / company / deal
// creation, batch read, properties listing, and a 404 lookup.
func TestDeterminism_HubSpot(t *testing.T) {
	hdr := map[string]string{"Authorization": "Bearer pat-test-token"}
	scenario := []testutil.Request{
		{
			Method:  "POST",
			Path:    "/crm/v3/objects/contacts",
			Headers: hdr,
			Body:    `{"properties":{"email":"alice@example.com","firstname":"Alice"}}`,
		},
		{
			Method:  "POST",
			Path:    "/crm/v3/objects/companies",
			Headers: hdr,
			Body:    `{"properties":{"name":"Acme","domain":"acme.example"}}`,
		},
		{
			Method:  "POST",
			Path:    "/crm/v3/objects/deals",
			Headers: hdr,
			Body:    `{"properties":{"dealname":"Acme expansion","amount":"10000"}}`,
		},
		{
			Method:  "GET",
			Path:    "/crm/v3/objects/contacts",
			Headers: hdr,
		},
		{
			Method:  "GET",
			Path:    "/crm/v3/objects/contacts/9999",
			Headers: hdr,
		},
	}
	testutil.AssertDeterministic(t, newDeterministicHubSpot(t), 42, scenario)
}

func newDeterministicHubSpot(t *testing.T) func(int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
	t.Helper()
	return func(seed int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
		cfg := &twincore.Config{Name: "twin-hubspot-determinism"}
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
