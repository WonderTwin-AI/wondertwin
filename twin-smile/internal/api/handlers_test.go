package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-smile/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-smile/internal/store"
)

func setupSmile(t *testing.T) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-smile-test"}
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

// seedCustomer loads a single customer into the store via admin state endpoint.
func seedCustomer(t *testing.T, tc *testutil.TwinClient, id string, c store.Customer) {
	t.Helper()
	ac := testutil.NewAdminClient(tc)
	ac.LoadState(map[string]any{
		"customers": map[string]any{
			id: c,
		},
		"redemptions": map[string]any{},
	}).AssertStatus(200)
}

// defaultCustomer returns a test customer with sensible defaults.
func defaultCustomer() store.Customer {
	return store.Customer{
		ID:              "cust_000001",
		Email:           "alice@example.com",
		FirstName:       "Alice",
		LastName:        "Smith",
		PointsBalance:   5000,
		Tier:            "gold",
		PointsPerDollar: 100,
		CreatedAt:       1700000000,
		UpdatedAt:       1700000000,
	}
}

// --- Get Customer Tests ---

func TestGetCustomerHappyPath(t *testing.T) {
	_, tc := setupSmile(t)
	cust := defaultCustomer()
	seedCustomer(t, tc, "cust_000001", cust)

	resp := tc.Get("/v1/customers/cust_000001")
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["id"] != "cust_000001" {
		t.Errorf("expected id cust_000001, got %v", m["id"])
	}
	if m["email"] != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %v", m["email"])
	}
	if m["first_name"] != "Alice" {
		t.Errorf("expected first_name Alice, got %v", m["first_name"])
	}
	if m["last_name"] != "Smith" {
		t.Errorf("expected last_name Smith, got %v", m["last_name"])
	}
	if m["points_balance"] != float64(5000) {
		t.Errorf("expected points_balance 5000, got %v", m["points_balance"])
	}
	if m["tier"] != "gold" {
		t.Errorf("expected tier gold, got %v", m["tier"])
	}
	if m["points_per_dollar"] != float64(100) {
		t.Errorf("expected points_per_dollar 100, got %v", m["points_per_dollar"])
	}
}

func TestGetCustomerNotFound(t *testing.T) {
	_, tc := setupSmile(t)

	resp := tc.Get("/v1/customers/nonexistent")
	resp.AssertStatus(404)
	resp.AssertBodyContains("customer not found")
}

// --- Redeem Points Tests ---

func TestRedeemPointsHappyPath(t *testing.T) {
	_, tc := setupSmile(t)
	cust := defaultCustomer()
	seedCustomer(t, tc, "cust_000001", cust)

	resp := tc.Post("/v1/points/redeem", map[string]any{
		"customer_id": "cust_000001",
		"points":      500,
	})
	resp.AssertStatus(201)

	m := resp.JSONMap()
	if m["customer_id"] != "cust_000001" {
		t.Errorf("expected customer_id cust_000001, got %v", m["customer_id"])
	}
	if m["points"] != float64(500) {
		t.Errorf("expected points 500, got %v", m["points"])
	}
	if m["status"] != "completed" {
		t.Errorf("expected status completed, got %v", m["status"])
	}
	// value_cents = 500 * 100 / 100 = 500
	if m["value_cents"] != float64(500) {
		t.Errorf("expected value_cents 500, got %v", m["value_cents"])
	}
	if m["id"] == nil || m["id"] == "" {
		t.Error("expected non-empty redemption id")
	}

	// Verify customer balance was debited
	custResp := tc.Get("/v1/customers/cust_000001")
	custResp.AssertStatus(200)
	cm := custResp.JSONMap()
	if cm["points_balance"] != float64(4500) {
		t.Errorf("expected points_balance 4500 after redeeming 500, got %v", cm["points_balance"])
	}
}

func TestRedeemPointsInsufficientBalance(t *testing.T) {
	_, tc := setupSmile(t)
	cust := defaultCustomer()
	cust.PointsBalance = 100
	seedCustomer(t, tc, "cust_000001", cust)

	resp := tc.Post("/v1/points/redeem", map[string]any{
		"customer_id": "cust_000001",
		"points":      500,
	})
	resp.AssertStatus(422)
	resp.AssertBodyContains("insufficient points balance")
}

func TestRedeemPointsCustomerNotFound(t *testing.T) {
	_, tc := setupSmile(t)

	resp := tc.Post("/v1/points/redeem", map[string]any{
		"customer_id": "nonexistent",
		"points":      100,
	})
	resp.AssertStatus(404)
	resp.AssertBodyContains("customer not found")
}

func TestRedeemPointsMissingCustomerID(t *testing.T) {
	_, tc := setupSmile(t)

	resp := tc.Post("/v1/points/redeem", map[string]any{
		"points": 100,
	})
	resp.AssertStatus(422)
	resp.AssertBodyContains("customer_id is required")
}

func TestRedeemPointsMissingPoints(t *testing.T) {
	_, tc := setupSmile(t)

	resp := tc.Post("/v1/points/redeem", map[string]any{
		"customer_id": "cust_000001",
	})
	resp.AssertStatus(422)
	resp.AssertBodyContains("points must be a positive integer")
}

func TestRedeemPointsZeroPoints(t *testing.T) {
	_, tc := setupSmile(t)

	resp := tc.Post("/v1/points/redeem", map[string]any{
		"customer_id": "cust_000001",
		"points":      0,
	})
	resp.AssertStatus(422)
	resp.AssertBodyContains("points must be a positive integer")
}

func TestRedeemPointsNegativePoints(t *testing.T) {
	_, tc := setupSmile(t)

	resp := tc.Post("/v1/points/redeem", map[string]any{
		"customer_id": "cust_000001",
		"points":      -10,
	})
	resp.AssertStatus(422)
	resp.AssertBodyContains("points must be a positive integer")
}

// --- Idempotency Tests ---

func TestRedeemPointsIdempotency(t *testing.T) {
	_, tc := setupSmile(t)
	cust := defaultCustomer()
	seedCustomer(t, tc, "cust_000001", cust)

	body := map[string]any{
		"customer_id":    "cust_000001",
		"points":         500,
		"idempotency_key": "idem-key-001",
	}

	// First redemption
	resp1 := tc.Post("/v1/points/redeem", body)
	resp1.AssertStatus(201)
	m1 := resp1.JSONMap()

	// Second redemption with same idempotency key should return the original
	resp2 := tc.Post("/v1/points/redeem", body)
	resp2.AssertStatus(200)
	m2 := resp2.JSONMap()

	if m1["id"] != m2["id"] {
		t.Errorf("expected same redemption ID for idempotent request, got %v and %v", m1["id"], m2["id"])
	}

	// Verify points were only debited once
	custResp := tc.Get("/v1/customers/cust_000001")
	custResp.AssertStatus(200)
	cm := custResp.JSONMap()
	if cm["points_balance"] != float64(4500) {
		t.Errorf("expected points_balance 4500 (single debit), got %v", cm["points_balance"])
	}
}

func TestRedeemPointsDifferentIdempotencyKeys(t *testing.T) {
	_, tc := setupSmile(t)
	cust := defaultCustomer()
	seedCustomer(t, tc, "cust_000001", cust)

	// First redemption
	resp1 := tc.Post("/v1/points/redeem", map[string]any{
		"customer_id":    "cust_000001",
		"points":         500,
		"idempotency_key": "key-A",
	})
	resp1.AssertStatus(201)

	// Second redemption with different key should create a new redemption
	resp2 := tc.Post("/v1/points/redeem", map[string]any{
		"customer_id":    "cust_000001",
		"points":         500,
		"idempotency_key": "key-B",
	})
	resp2.AssertStatus(201)

	m1 := resp1.JSONMap()
	m2 := resp2.JSONMap()
	if m1["id"] == m2["id"] {
		t.Error("expected different redemption IDs for different idempotency keys")
	}

	// Verify points debited twice
	custResp := tc.Get("/v1/customers/cust_000001")
	custResp.AssertStatus(200)
	cm := custResp.JSONMap()
	if cm["points_balance"] != float64(4000) {
		t.Errorf("expected points_balance 4000 (two debits of 500), got %v", cm["points_balance"])
	}
}

// --- Refund Points Tests ---

func TestRefundPointsHappyPath(t *testing.T) {
	_, tc := setupSmile(t)
	cust := defaultCustomer()
	seedCustomer(t, tc, "cust_000001", cust)

	// First create a redemption
	redeemResp := tc.Post("/v1/points/redeem", map[string]any{
		"customer_id": "cust_000001",
		"points":      500,
	})
	redeemResp.AssertStatus(201)
	redemption := redeemResp.JSONMap()
	redemptionID := redemption["id"].(string)

	// Now refund it
	refundResp := tc.Post("/v1/points/refund", map[string]any{
		"redemption_id": redemptionID,
	})
	refundResp.AssertStatus(200)

	rm := refundResp.JSONMap()
	if rm["status"] != "refunded" {
		t.Errorf("expected status refunded, got %v", rm["status"])
	}
	if rm["id"] != redemptionID {
		t.Errorf("expected redemption id %s, got %v", redemptionID, rm["id"])
	}

	// Verify points were restored
	custResp := tc.Get("/v1/customers/cust_000001")
	custResp.AssertStatus(200)
	cm := custResp.JSONMap()
	if cm["points_balance"] != float64(5000) {
		t.Errorf("expected points_balance 5000 after full refund, got %v", cm["points_balance"])
	}
}

func TestRefundPointsPartial(t *testing.T) {
	_, tc := setupSmile(t)
	cust := defaultCustomer()
	seedCustomer(t, tc, "cust_000001", cust)

	// Create a redemption of 500 points
	redeemResp := tc.Post("/v1/points/redeem", map[string]any{
		"customer_id": "cust_000001",
		"points":      500,
	})
	redeemResp.AssertStatus(201)
	redemption := redeemResp.JSONMap()
	redemptionID := redemption["id"].(string)

	// Refund only 200 points
	refundResp := tc.Post("/v1/points/refund", map[string]any{
		"redemption_id": redemptionID,
		"points":        200,
	})
	refundResp.AssertStatus(200)

	// Verify only 200 points were restored (5000 - 500 + 200 = 4700)
	custResp := tc.Get("/v1/customers/cust_000001")
	custResp.AssertStatus(200)
	cm := custResp.JSONMap()
	if cm["points_balance"] != float64(4700) {
		t.Errorf("expected points_balance 4700 after partial refund, got %v", cm["points_balance"])
	}
}

func TestRefundPointsAlreadyRefunded(t *testing.T) {
	_, tc := setupSmile(t)
	cust := defaultCustomer()
	seedCustomer(t, tc, "cust_000001", cust)

	// Create and refund a redemption
	redeemResp := tc.Post("/v1/points/redeem", map[string]any{
		"customer_id": "cust_000001",
		"points":      500,
	})
	redeemResp.AssertStatus(201)
	redemptionID := redeemResp.JSONMap()["id"].(string)

	// First refund
	tc.Post("/v1/points/refund", map[string]any{
		"redemption_id": redemptionID,
	}).AssertStatus(200)

	// Second refund should fail
	resp := tc.Post("/v1/points/refund", map[string]any{
		"redemption_id": redemptionID,
	})
	resp.AssertStatus(422)
	resp.AssertBodyContains("redemption already refunded")
}

func TestRefundPointsRedemptionNotFound(t *testing.T) {
	_, tc := setupSmile(t)

	resp := tc.Post("/v1/points/refund", map[string]any{
		"redemption_id": "nonexistent",
	})
	resp.AssertStatus(404)
	resp.AssertBodyContains("redemption not found")
}

func TestRefundPointsMissingRedemptionID(t *testing.T) {
	_, tc := setupSmile(t)

	resp := tc.Post("/v1/points/refund", map[string]any{})
	resp.AssertStatus(422)
	resp.AssertBodyContains("redemption_id is required")
}

// --- Admin Tests ---

func TestAdminHealthCheck(t *testing.T) {
	_, tc := setupSmile(t)
	ac := testutil.NewAdminClient(tc)
	ac.Health().AssertStatus(200)
}

func TestAdminResetClearsState(t *testing.T) {
	_, tc := setupSmile(t)
	ac := testutil.NewAdminClient(tc)

	// Seed a customer
	cust := defaultCustomer()
	seedCustomer(t, tc, "cust_000001", cust)

	// Verify customer exists
	tc.Get("/v1/customers/cust_000001").AssertStatus(200)

	// Reset all state
	ac.Reset().AssertStatus(200)

	// Customer should no longer exist
	resp := tc.Get("/v1/customers/cust_000001")
	resp.AssertStatus(404)
	resp.AssertBodyContains("customer not found")
}

func TestAdminGetState(t *testing.T) {
	_, tc := setupSmile(t)
	ac := testutil.NewAdminClient(tc)

	// Seed a customer
	cust := defaultCustomer()
	seedCustomer(t, tc, "cust_000001", cust)

	resp := ac.GetState()
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["customers"] == nil {
		t.Error("expected customers in state snapshot")
	}
	if m["redemptions"] == nil {
		t.Error("expected redemptions in state snapshot")
	}

	customers := m["customers"].(map[string]any)
	if _, ok := customers["cust_000001"]; !ok {
		t.Error("expected cust_000001 in state snapshot")
	}
}

func TestAdminLoadState(t *testing.T) {
	_, tc := setupSmile(t)
	ac := testutil.NewAdminClient(tc)

	// Load custom state
	state := map[string]any{
		"customers": map[string]any{
			"cust_custom": map[string]any{
				"id":               "cust_custom",
				"email":            "custom@example.com",
				"first_name":       "Custom",
				"last_name":        "User",
				"points_balance":   9999,
				"tier":             "vip",
				"points_per_dollar": 50,
				"created_at":       1700000000,
				"updated_at":       1700000000,
			},
		},
		"redemptions": map[string]any{},
	}
	ac.LoadState(state).AssertStatus(200)

	// Verify new customer is accessible
	resp := tc.Get("/v1/customers/cust_custom")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["email"] != "custom@example.com" {
		t.Errorf("expected custom@example.com, got %v", m["email"])
	}
	if m["points_balance"] != float64(9999) {
		t.Errorf("expected 9999, got %v", m["points_balance"])
	}
}
