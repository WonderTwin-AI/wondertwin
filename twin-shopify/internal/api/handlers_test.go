package api_test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-shopify/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-shopify/internal/store"
)

const v = "/admin/api/2025-04"

func setupShopify(t *testing.T) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-shopify-test"}
	twin := twincore.New(cfg)
	handler := api.NewHandler(memStore, twin.Middleware())
	handler.Routes(twin.Router)
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	tc := testutil.NewTwinClient(t, srv)
	return srv, tc
}

var shopifyHeaders = map[string]string{
	"X-Shopify-Access-Token": "shpat_test_token",
	"Content-Type":           "application/json",
}

func sGet(tc *testutil.TwinClient, path string) *testutil.Response {
	return tc.DoWithHeaders("GET", path, nil, shopifyHeaders)
}

func sPost(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("POST", path, body, shopifyHeaders)
}

func sPut(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("PUT", path, body, shopifyHeaders)
}

func sDelete(tc *testutil.TwinClient, path string) *testutil.Response {
	return tc.DoWithHeaders("DELETE", path, nil, shopifyHeaders)
}

// --- Auth ---

func TestAuthRequired(t *testing.T) {
	_, tc := setupShopify(t)
	resp := tc.Get(v + "/shop.json")
	resp.AssertStatus(401)
}

// --- Shop ---

func TestGetShop(t *testing.T) {
	_, tc := setupShopify(t)
	resp := sGet(tc, v+"/shop.json")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	shop := m["shop"].(map[string]any)
	if shop["name"] != "WonderTwin Store" {
		t.Errorf("expected name=WonderTwin Store, got %v", shop["name"])
	}
}

// --- Products ---

func TestProductCRUD(t *testing.T) {
	_, tc := setupShopify(t)

	// Create
	resp := sPost(tc, v+"/products.json", map[string]any{
		"product": map[string]any{
			"title":   "Test Widget",
			"vendor":  "ACME",
			"status":  "active",
		},
	})
	resp.AssertStatus(201)
	product := resp.JSONMap()["product"].(map[string]any)
	id := int(product["id"].(float64))
	if product["title"] != "Test Widget" {
		t.Error("expected title=Test Widget")
	}

	// Get
	resp = sGet(tc, fmt.Sprintf("%s/products/%d.json", v, id))
	resp.AssertStatus(200)

	// Update
	resp = sPut(tc, fmt.Sprintf("%s/products/%d.json", v, id), map[string]any{
		"product": map[string]any{"title": "Updated Widget"},
	})
	resp.AssertStatus(200)
	if resp.JSONMap()["product"].(map[string]any)["title"] != "Updated Widget" {
		t.Error("expected updated title")
	}

	// List
	resp = sGet(tc, v+"/products.json")
	resp.AssertStatus(200)
	products := resp.JSONMap()["products"].([]any)
	if len(products) != 1 {
		t.Errorf("expected 1 product, got %d", len(products))
	}

	// Count
	resp = sGet(tc, v+"/products/count.json")
	if resp.JSONMap()["count"].(float64) != 1 {
		t.Error("expected count=1")
	}

	// Delete
	sDelete(tc, fmt.Sprintf("%s/products/%d.json", v, id)).AssertStatus(200)

	resp = sGet(tc, v+"/products/count.json")
	if resp.JSONMap()["count"].(float64) != 0 {
		t.Error("expected count=0 after delete")
	}
}

// --- Orders ---

func TestOrderLifecycle(t *testing.T) {
	_, tc := setupShopify(t)

	resp := sPost(tc, v+"/orders.json", map[string]any{
		"order": map[string]any{
			"email":    "test@example.com",
			"currency": "USD",
		},
	})
	resp.AssertStatus(201)
	order := resp.JSONMap()["order"].(map[string]any)
	id := int(order["id"].(float64))

	// Close
	sPost(tc, fmt.Sprintf("%s/orders/%d/close.json", v, id), nil).AssertStatus(200)

	// Open
	sPost(tc, fmt.Sprintf("%s/orders/%d/open.json", v, id), nil).AssertStatus(200)

	// Cancel
	sPost(tc, fmt.Sprintf("%s/orders/%d/cancel.json", v, id), nil).AssertStatus(200)

	resp = sGet(tc, fmt.Sprintf("%s/orders/%d.json", v, id))
	if resp.JSONMap()["order"].(map[string]any)["cancelled"].(bool) != true {
		t.Error("expected cancelled=true")
	}
}

// --- Transactions ---

func TestTransactions(t *testing.T) {
	_, tc := setupShopify(t)

	resp := sPost(tc, v+"/orders.json", map[string]any{
		"order": map[string]any{"email": "tx@example.com"},
	})
	orderID := int(resp.JSONMap()["order"].(map[string]any)["id"].(float64))

	resp = sPost(tc, fmt.Sprintf("%s/orders/%d/transactions.json", v, orderID), map[string]any{
		"transaction": map[string]any{
			"kind": "sale", "amount": "100.00",
		},
	})
	resp.AssertStatus(201)

	resp = sGet(tc, fmt.Sprintf("%s/orders/%d/transactions.json", v, orderID))
	txns := resp.JSONMap()["transactions"].([]any)
	if len(txns) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(txns))
	}
}

// --- Customers ---

func TestCustomerCRUD(t *testing.T) {
	_, tc := setupShopify(t)

	resp := sPost(tc, v+"/customers.json", map[string]any{
		"customer": map[string]any{
			"email":      "alice@example.com",
			"first_name": "Alice",
			"last_name":  "Smith",
		},
	})
	resp.AssertStatus(201)
	id := int(resp.JSONMap()["customer"].(map[string]any)["id"].(float64))

	// Search
	resp = sGet(tc, v+"/customers/search.json?query=alice")
	customers := resp.JSONMap()["customers"].([]any)
	if len(customers) != 1 {
		t.Errorf("expected 1 customer in search, got %d", len(customers))
	}

	// Delete
	sDelete(tc, fmt.Sprintf("%s/customers/%d.json", v, id)).AssertStatus(200)
}

// --- Collections ---

func TestCustomCollections(t *testing.T) {
	_, tc := setupShopify(t)

	resp := sPost(tc, v+"/custom_collections.json", map[string]any{
		"custom_collection": map[string]any{"title": "Summer Sale"},
	})
	resp.AssertStatus(201)
	id := int(resp.JSONMap()["custom_collection"].(map[string]any)["id"].(float64))

	resp = sGet(tc, v+"/custom_collections.json")
	cols := resp.JSONMap()["custom_collections"].([]any)
	if len(cols) != 1 {
		t.Errorf("expected 1 collection, got %d", len(cols))
	}

	sDelete(tc, fmt.Sprintf("%s/custom_collections/%d.json", v, id)).AssertStatus(200)
}

// --- Inventory ---

func TestInventoryLevels(t *testing.T) {
	_, tc := setupShopify(t)

	// Seed a location
	tc.Post("/admin/state", json.RawMessage(`{"locations":{"loc_001":{"id":1,"name":"Warehouse","active":true}}}`)).AssertStatus(200)

	resp := sGet(tc, v+"/locations.json")
	resp.AssertStatus(200)
	locations := resp.JSONMap()["locations"].([]any)
	if len(locations) != 1 {
		t.Errorf("expected 1 location, got %d", len(locations))
	}

	// Set inventory level
	resp = sPost(tc, v+"/inventory_levels/set.json", map[string]any{
		"inventory_item_id": 1,
		"location_id":       1,
		"available":          50,
	})
	resp.AssertStatus(200)
}

// --- Webhooks ---

func TestWebhookCRUD(t *testing.T) {
	_, tc := setupShopify(t)

	resp := sPost(tc, v+"/webhooks.json", map[string]any{
		"webhook": map[string]any{
			"topic":   "orders/create",
			"address": "https://example.com/hook",
			"format":  "json",
		},
	})
	resp.AssertStatus(201)
	id := int(resp.JSONMap()["webhook"].(map[string]any)["id"].(float64))

	resp = sGet(tc, v+"/webhooks.json")
	hooks := resp.JSONMap()["webhooks"].([]any)
	if len(hooks) != 1 {
		t.Errorf("expected 1 webhook, got %d", len(hooks))
	}

	sDelete(tc, fmt.Sprintf("%s/webhooks/%d.json", v, id)).AssertStatus(200)
}

// --- Metafields ---

func TestMetafieldCRUD(t *testing.T) {
	_, tc := setupShopify(t)

	resp := sPost(tc, v+"/metafields.json", map[string]any{
		"metafield": map[string]any{
			"namespace": "custom",
			"key":       "color",
			"value":     "red",
			"type":      "single_line_text_field",
		},
	})
	resp.AssertStatus(201)
	id := int(resp.JSONMap()["metafield"].(map[string]any)["id"].(float64))

	resp = sGet(tc, v+"/metafields.json")
	mfs := resp.JSONMap()["metafields"].([]any)
	if len(mfs) != 1 {
		t.Errorf("expected 1 metafield, got %d", len(mfs))
	}

	sDelete(tc, fmt.Sprintf("%s/metafields/%d.json", v, id)).AssertStatus(200)
}

// --- Draft Orders ---

func TestDraftOrderLifecycle(t *testing.T) {
	_, tc := setupShopify(t)

	resp := sPost(tc, v+"/draft_orders.json", map[string]any{
		"draft_order": map[string]any{
			"email": "draft@example.com",
		},
	})
	resp.AssertStatus(201)
	id := int(resp.JSONMap()["draft_order"].(map[string]any)["id"].(float64))

	// Complete
	resp = sPost(tc, fmt.Sprintf("%s/draft_orders/%d/complete.json", v, id), nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["draft_order"].(map[string]any)["status"] != "completed" {
		t.Error("expected status=completed")
	}
}

// --- Pages ---

func TestPageCRUD(t *testing.T) {
	_, tc := setupShopify(t)

	resp := sPost(tc, v+"/pages.json", map[string]any{
		"page": map[string]any{
			"title":     "About Us",
			"body_html": "<p>Hello</p>",
		},
	})
	resp.AssertStatus(201)
	id := int(resp.JSONMap()["page"].(map[string]any)["id"].(float64))

	sDelete(tc, fmt.Sprintf("%s/pages/%d.json", v, id)).AssertStatus(200)
}

// --- Price Rules + Discount Codes ---

func TestPriceRulesAndDiscountCodes(t *testing.T) {
	_, tc := setupShopify(t)

	resp := sPost(tc, v+"/price_rules.json", map[string]any{
		"price_rule": map[string]any{
			"title":      "10% Off",
			"value_type": "percentage",
			"value":      "-10.0",
		},
	})
	resp.AssertStatus(201)
	prID := int(resp.JSONMap()["price_rule"].(map[string]any)["id"].(float64))

	resp = sPost(tc, fmt.Sprintf("%s/price_rules/%d/discount_codes.json", v, prID), map[string]any{
		"discount_code": map[string]any{"code": "SAVE10"},
	})
	resp.AssertStatus(201)

	resp = sGet(tc, fmt.Sprintf("%s/price_rules/%d/discount_codes.json", v, prID))
	codes := resp.JSONMap()["discount_codes"].([]any)
	if len(codes) != 1 {
		t.Errorf("expected 1 discount code, got %d", len(codes))
	}
}

// --- Admin ---

func TestAdminHealth(t *testing.T) {
	_, tc := setupShopify(t)
	tc.Get("/admin/health").AssertStatus(200)
}

func TestAdminReset(t *testing.T) {
	_, tc := setupShopify(t)

	sPost(tc, v+"/products.json", map[string]any{
		"product": map[string]any{"title": "Will Reset"},
	})

	tc.Post("/admin/reset", nil).AssertStatus(200)

	resp := tc.Get("/admin/products")
	if resp.JSONMap()["total"].(float64) != 0 {
		t.Error("expected 0 products after reset")
	}
}
