package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-shopify/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-shopify/internal/store"
)

// TestDeterminism_Shopify asserts byte-for-byte reproducibility per
// seed across the canonical commerce surface: shop info, product
// create, variant create, customer create, and a 404 lookup.
func TestDeterminism_Shopify(t *testing.T) {
	hdr := map[string]string{"X-Shopify-Access-Token": "shpat_test_token"}
	scenario := []testutil.Request{
		{
			Method:  "GET",
			Path:    "/admin/api/2024-04/shop.json",
			Headers: hdr,
		},
		{
			Method:  "POST",
			Path:    "/admin/api/2024-04/products.json",
			Headers: hdr,
			Body:    `{"product":{"title":"Snowboard","vendor":"Acme","product_type":"Sports"}}`,
		},
		{
			Method:  "POST",
			Path:    "/admin/api/2024-04/customers.json",
			Headers: hdr,
			Body:    `{"customer":{"first_name":"Alice","last_name":"Smith","email":"alice@example.com"}}`,
		},
		{
			Method:  "GET",
			Path:    "/admin/api/2024-04/products.json",
			Headers: hdr,
		},
		{
			Method:  "GET",
			Path:    "/admin/api/2024-04/products/9999.json",
			Headers: hdr,
		},
	}
	testutil.AssertDeterministic(t, newDeterministicShopify(t), 42, scenario)
}

func newDeterministicShopify(t *testing.T) func(int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
	t.Helper()
	return func(seed int64) (*testutil.TwinClient, *testutil.AdminClient, func()) {
		cfg := &twincore.Config{Name: "twin-shopify-determinism"}
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
