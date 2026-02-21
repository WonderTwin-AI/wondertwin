package api_test

import (
	"encoding/base64"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-loyaltylion/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-loyaltylion/internal/store"
)

func setupLoyaltyLion(t *testing.T) (*testutil.TwinClient, *testutil.AdminClient, *twincore.Middleware) {
	t.Helper()
	memStore := store.New()
	memStore.SeedDefaults()
	cfg := &twincore.Config{Name: "twin-loyaltylion-test"}
	twin := twincore.New(cfg)
	mw := twin.Middleware()
	handler := api.NewHandler(memStore, mw)
	handler.Routes(twin.Router)
	adminHandler := admin.NewHandler(memStore, mw, memStore.Clock)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	tc := testutil.NewTwinClient(t, srv)
	ac := testutil.NewAdminClient(tc)
	return tc, ac, mw
}

func basicAuth(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

var authAlpha = map[string]string{
	"Authorization": basicAuth("ll_test_key_alpha", "ll_test_secret_alpha"),
}

var authBeta = map[string]string{
	"Authorization": basicAuth("ll_test_key_beta", "ll_test_secret_beta"),
}

func llGet(tc *testutil.TwinClient, path string, headers map[string]string) *testutil.Response {
	return tc.DoWithHeaders("GET", path, nil, headers)
}

func llPost(tc *testutil.TwinClient, path string, body any, headers map[string]string) *testutil.Response {
	return tc.DoWithHeaders("POST", path, body, headers)
}

// --- Auth Tests ---

func TestAuthRequired(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)
	resp := tc.Get("/v2/customers")
	resp.AssertStatus(401)
}

func TestAuthInvalidKey(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)
	headers := map[string]string{
		"Authorization": basicAuth("bad_key", "bad_secret"),
	}
	resp := llGet(tc, "/v2/customers", headers)
	resp.AssertStatus(401)
}

func TestAuthInvalidSecret(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)
	headers := map[string]string{
		"Authorization": basicAuth("ll_test_key_alpha", "wrong_secret"),
	}
	resp := llGet(tc, "/v2/customers", headers)
	resp.AssertStatus(401)
}

func TestAuthMalformed(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)
	headers := map[string]string{
		"Authorization": "Bearer some_token",
	}
	resp := llGet(tc, "/v2/customers", headers)
	resp.AssertStatus(401)
}

// --- Customer CRUD Tests ---

func TestListCustomers(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)
	resp := llGet(tc, "/v2/customers", authAlpha)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	customers, ok := m["customers"].([]any)
	if !ok {
		t.Fatal("expected customers array")
	}
	if len(customers) != 3 {
		t.Errorf("expected 3 customers for merchant alpha, got %d", len(customers))
	}
}

func TestSearchCustomerByEmail(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)

	// Found
	resp := llGet(tc, "/v2/customers?email=sarah@example.com", authAlpha)
	resp.AssertStatus(200)
	m := resp.JSONMap()
	customers := m["customers"].([]any)
	if len(customers) != 1 {
		t.Fatalf("expected 1 customer, got %d", len(customers))
	}
	c := customers[0].(map[string]any)
	if c["email"] != "sarah@example.com" {
		t.Errorf("expected sarah@example.com, got %v", c["email"])
	}

	// Not found
	resp = llGet(tc, "/v2/customers?email=nobody@example.com", authAlpha)
	resp.AssertStatus(200)
	m = resp.JSONMap()
	customers = m["customers"].([]any)
	if len(customers) != 0 {
		t.Errorf("expected 0 customers, got %d", len(customers))
	}
}

func TestCreateCustomer(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)
	resp := llPost(tc, "/v2/customers", map[string]any{
		"merchant_id": "new-cust-001",
		"email":       "new@example.com",
	}, authAlpha)
	resp.AssertStatus(201)

	m := resp.JSONMap()
	if m["email"] != "new@example.com" {
		t.Errorf("expected new@example.com, got %v", m["email"])
	}
	if m["merchant_id"] != "new-cust-001" {
		t.Errorf("expected new-cust-001, got %v", m["merchant_id"])
	}
	if m["points_approved"] != float64(0) {
		t.Errorf("expected 0 points_approved, got %v", m["points_approved"])
	}
}

func TestGetCustomerByID(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)

	// Get customer 1 (Sarah)
	resp := llGet(tc, "/v2/customers/1", authAlpha)
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["email"] != "sarah@example.com" {
		t.Errorf("expected sarah@example.com, got %v", m["email"])
	}
}

func TestGetCustomerNotFound(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)
	resp := llGet(tc, "/v2/customers/999", authAlpha)
	resp.AssertStatus(404)
}

// --- Points Tests ---

func TestGetPointsBalance(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)
	resp := llGet(tc, "/v2/customers/cust-001/points", authAlpha)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["points_approved"] != float64(4200) {
		t.Errorf("expected 4200 approved, got %v", m["points_approved"])
	}
	if m["points_pending"] != float64(100) {
		t.Errorf("expected 100 pending, got %v", m["points_pending"])
	}
}

func TestAddPoints(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)
	resp := llPost(tc, "/v2/customers/cust-001/points", map[string]any{
		"points": 500,
		"reason": "Manual bonus",
	}, authAlpha)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["points_approved"] != float64(4700) {
		t.Errorf("expected 4700 approved after adding 500, got %v", m["points_approved"])
	}
}

func TestRemovePoints(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)
	resp := llPost(tc, "/v2/customers/cust-001/points/remove", map[string]any{
		"points": 200,
		"reason": "Manual debit",
	}, authAlpha)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["points_approved"] != float64(4000) {
		t.Errorf("expected 4000 approved after removing 200, got %v", m["points_approved"])
	}
	if m["points_spent"] != float64(3700) {
		t.Errorf("expected 3700 spent, got %v", m["points_spent"])
	}
}

func TestRemovePointsInsufficientBalance(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)
	// Alex has 0 approved points
	resp := llPost(tc, "/v2/customers/cust-002/points/remove", map[string]any{
		"points": 100,
		"reason": "Too much",
	}, authAlpha)
	resp.AssertStatus(422)
	resp.AssertBodyContains("insufficient_points")
}

// --- Rewards & Redemption Tests ---

func TestListAvailableRewards(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)
	resp := llGet(tc, "/v2/customers/cust-001/available_rewards", authAlpha)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	rewards, ok := m["rewards"].([]any)
	if !ok {
		t.Fatal("expected rewards array")
	}
	if len(rewards) != 4 {
		t.Errorf("expected 4 rewards for merchant alpha, got %d", len(rewards))
	}
}

func TestClaimRewardHappyPath(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)

	// Jamie has 15000 points, claim $5 Off (500 pts, reward id 1)
	resp := llPost(tc, "/v2/customers/cust-003/claimed_rewards", map[string]any{
		"reward_id":  1,
		"multiplier": 1,
	}, authAlpha)
	resp.AssertStatus(201)

	m := resp.JSONMap()
	claimed := m["claimed_reward"].(map[string]any)
	if claimed["point_cost"] != float64(500) {
		t.Errorf("expected 500 point_cost, got %v", claimed["point_cost"])
	}
	redeemable := claimed["redeemable"].(map[string]any)
	code := redeemable["code"].(string)
	if len(code) == 0 {
		t.Error("expected non-empty discount code")
	}
	if redeemable["fulfilled"] != false {
		t.Error("expected fulfilled=false")
	}

	// Verify points were debited
	resp = llGet(tc, "/v2/customers/cust-003/points", authAlpha)
	resp.AssertStatus(200)
	points := resp.JSONMap()
	if points["points_approved"] != float64(14500) {
		t.Errorf("expected 14500 approved after claiming 500, got %v", points["points_approved"])
	}
}

func TestClaimRewardInsufficientPoints(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)

	// Alex has 0 points, try to claim $5 Off (500 pts)
	resp := llPost(tc, "/v2/customers/cust-002/claimed_rewards", map[string]any{
		"reward_id":  1,
		"multiplier": 1,
	}, authAlpha)
	resp.AssertStatus(422)
	resp.AssertBodyContains("insufficient_points")
}

func TestRedemptionReversal(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)

	// Jamie claims $5 Off
	resp := llPost(tc, "/v2/customers/cust-003/claimed_rewards", map[string]any{
		"reward_id":  1,
		"multiplier": 1,
	}, authAlpha)
	resp.AssertStatus(201)
	claimed := resp.JSONMap()["claimed_reward"].(map[string]any)
	claimID := claimed["id"].(float64)

	// Check points after claim
	resp = llGet(tc, "/v2/customers/cust-003/points", authAlpha)
	afterClaim := resp.JSONMap()
	approvedAfterClaim := afterClaim["points_approved"].(float64)

	// Refund
	resp = llPost(tc, fmt.Sprintf("/v2/customers/cust-003/claimed_rewards/%d/refund", int(claimID)), nil, authAlpha)
	resp.AssertStatus(200)
	refunded := resp.JSONMap()["claimed_reward"].(map[string]any)
	if refunded["refunded"] != true {
		t.Error("expected refunded=true")
	}

	// Verify points restored
	resp = llGet(tc, "/v2/customers/cust-003/points", authAlpha)
	afterRefund := resp.JSONMap()
	if afterRefund["points_approved"].(float64) != approvedAfterClaim+500 {
		t.Errorf("expected points restored after refund, got %v", afterRefund["points_approved"])
	}
}

func TestClaimRewardIdempotency(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)

	body := map[string]any{
		"reward_id":  1,
		"multiplier": 1,
	}

	// First claim
	resp1 := llPost(tc, "/v2/customers/cust-003/claimed_rewards", body, authAlpha)
	resp1.AssertStatus(201)
	claimed1 := resp1.JSONMap()["claimed_reward"].(map[string]any)

	// Second identical claim within 60s window — should return same result
	resp2 := llPost(tc, "/v2/customers/cust-003/claimed_rewards", body, authAlpha)
	resp2.AssertStatus(200)
	claimed2 := resp2.JSONMap()["claimed_reward"].(map[string]any)

	if claimed1["id"] != claimed2["id"] {
		t.Errorf("expected idempotent response with same ID, got %v and %v", claimed1["id"], claimed2["id"])
	}

	// Verify points were only debited once
	resp := llGet(tc, "/v2/customers/cust-003/points", authAlpha)
	points := resp.JSONMap()
	// Jamie started with 15000, claimed 500 once
	if points["points_approved"] != float64(14500) {
		t.Errorf("expected 14500 (single debit), got %v", points["points_approved"])
	}
}

func TestDiscountCodeUniqueness(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)

	codes := make(map[string]bool)
	for i := 0; i < 5; i++ {
		// Claim different rewards to avoid idempotency
		rewardID := (i % 4) + 1
		resp := llPost(tc, "/v2/customers/cust-003/claimed_rewards", map[string]any{
			"reward_id":  rewardID,
			"multiplier": i + 1, // Different multiplier to avoid idempotency
		}, authAlpha)
		if resp.StatusCode == 201 {
			claimed := resp.JSONMap()["claimed_reward"].(map[string]any)
			code := claimed["redeemable"].(map[string]any)["code"].(string)
			if codes[code] {
				t.Errorf("duplicate discount code: %s", code)
			}
			codes[code] = true
		}
	}
}

// --- Multi-Merchant Isolation Tests ---

func TestMultiMerchantIsolation(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)

	// Same email, different merchants
	respA := llGet(tc, "/v2/customers?email=sarah@example.com", authAlpha)
	respA.AssertStatus(200)
	customersA := respA.JSONMap()["customers"].([]any)

	respB := llGet(tc, "/v2/customers?email=sarah@example.com", authBeta)
	respB.AssertStatus(200)
	customersB := respB.JSONMap()["customers"].([]any)

	if len(customersA) != 1 || len(customersB) != 1 {
		t.Fatalf("expected 1 customer each, got A=%d B=%d", len(customersA), len(customersB))
	}

	sarahA := customersA[0].(map[string]any)
	sarahB := customersB[0].(map[string]any)

	// Different IDs
	if sarahA["id"] == sarahB["id"] {
		t.Error("expected different customer IDs across merchants")
	}

	// Different points
	if sarahA["points_approved"] == sarahB["points_approved"] {
		t.Error("expected different points across merchants")
	}

	// Different merchant_ids
	if sarahA["merchant_id"] == sarahB["merchant_id"] {
		t.Error("expected different merchant_ids")
	}
}

func TestMerchantCantSeeOtherMerchantCustomers(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)

	// Morgan only exists in merchant B
	resp := llGet(tc, "/v2/customers?email=morgan@example.com", authAlpha)
	resp.AssertStatus(200)
	customers := resp.JSONMap()["customers"].([]any)
	if len(customers) != 0 {
		t.Errorf("expected 0 customers for morgan in merchant A, got %d", len(customers))
	}

	// But visible in merchant B
	resp = llGet(tc, "/v2/customers?email=morgan@example.com", authBeta)
	resp.AssertStatus(200)
	customers = resp.JSONMap()["customers"].([]any)
	if len(customers) != 1 {
		t.Errorf("expected 1 customer for morgan in merchant B, got %d", len(customers))
	}
}

// --- Points Expiration Test ---

func TestPointsExpiration(t *testing.T) {
	tc, ac, _ := setupLoyaltyLion(t)

	// Sarah has 4200 approved with 500 expiring in 30 days
	resp := llGet(tc, "/v2/customers/cust-001/points", authAlpha)
	resp.AssertStatus(200)
	before := resp.JSONMap()
	if before["points_approved"] != float64(4200) {
		t.Fatalf("expected 4200 before expiration, got %v", before["points_approved"])
	}

	// Advance clock past 30 days
	ac.AdvanceTime("744h").AssertStatus(200) // 31 days

	// Check balance again — expired points should be moved
	resp = llGet(tc, "/v2/customers/cust-001/points", authAlpha)
	resp.AssertStatus(200)
	after := resp.JSONMap()
	if after["points_approved"] != float64(3700) {
		t.Errorf("expected 3700 approved after expiration of 500, got %v", after["points_approved"])
	}
	if after["points_expired"] != float64(500) {
		t.Errorf("expected 500 expired, got %v", after["points_expired"])
	}
}

// --- Fault Injection Tests ---

func TestFaultInjectionRedemptionUnavailable(t *testing.T) {
	tc, _, mw := setupLoyaltyLion(t)

	// Inject fault on the exact claimed_rewards path via middleware directly
	claimPath := "/v2/customers/cust-003/claimed_rewards"
	mw.Faults.Set(claimPath, twincore.FaultConfig{
		StatusCode: 503,
		Body:       `{"error": "service_unavailable"}`,
		Rate:       1.0,
	})

	// Balance check should still work (different path)
	resp := llGet(tc, "/v2/customers/cust-003/points", authAlpha)
	resp.AssertStatus(200)

	// But claiming should fail
	resp = llPost(tc, claimPath, map[string]any{
		"reward_id":  1,
		"multiplier": 1,
	}, authAlpha)
	resp.AssertStatus(503)

	// Remove fault
	mw.Faults.Remove(claimPath)

	// Now claim should work
	resp = llPost(tc, claimPath, map[string]any{
		"reward_id":  1,
		"multiplier": 1,
	}, authAlpha)
	resp.AssertStatus(201)
}

// --- Rate Limit Headers Test ---

func TestRateLimitHeaders(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)
	resp := llGet(tc, "/v2/customers", authAlpha)
	resp.AssertStatus(200)

	limit := resp.Headers.Get("X-RateLimit-Limit")
	remaining := resp.Headers.Get("X-RateLimit-Remaining")

	if limit != "20" {
		t.Errorf("expected X-RateLimit-Limit=20, got %s", limit)
	}
	if remaining == "" {
		t.Error("expected X-RateLimit-Remaining header")
	}
}

// --- Activities Test ---

func TestRecordActivity(t *testing.T) {
	tc, _, _ := setupLoyaltyLion(t)
	resp := llPost(tc, "/v2/activities", map[string]any{
		"name":        "purchase",
		"merchant_id": "cust-001",
		"properties": map[string]string{
			"order_id": "ORD-123",
			"total":    "110.57",
			"currency": "USD",
		},
	}, authAlpha)
	resp.AssertStatus(201)

	m := resp.JSONMap()
	if m["name"] != "purchase" {
		t.Errorf("expected name=purchase, got %v", m["name"])
	}
}

// --- Admin Tests ---

func TestAdminHealth(t *testing.T) {
	_, ac, _ := setupLoyaltyLion(t)
	ac.Health().AssertStatus(200)
}

func TestAdminReset(t *testing.T) {
	tc, ac, _ := setupLoyaltyLion(t)

	// Create a new customer
	llPost(tc, "/v2/customers", map[string]any{
		"merchant_id": "temp-001",
		"email":       "temp@example.com",
	}, authAlpha)

	// Reset
	ac.Reset().AssertStatus(200)

	// Should be back to fixtures (3 customers for alpha)
	resp := llGet(tc, "/v2/customers", authAlpha)
	resp.AssertStatus(200)
	customers := resp.JSONMap()["customers"].([]any)
	if len(customers) != 3 {
		t.Errorf("expected 3 customers after reset, got %d", len(customers))
	}
}

func TestAdminGetState(t *testing.T) {
	_, ac, _ := setupLoyaltyLion(t)
	resp := ac.GetState()
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["merchants"] == nil {
		t.Error("expected merchants in state")
	}
	if m["customers"] == nil {
		t.Error("expected customers in state")
	}
}

func TestAdminLoadState(t *testing.T) {
	tc, ac, _ := setupLoyaltyLion(t)

	// Load custom state with only one merchant/customer
	state := map[string]any{
		"merchants": map[string]any{
			"custom_key": map[string]any{
				"api_key":    "custom_key",
				"api_secret": "custom_secret",
				"name":       "Custom Store",
			},
		},
		"customers": map[string]any{
			"100": map[string]any{
				"id":              100,
				"merchant_id":     "c-100",
				"email":           "custom@example.com",
				"points_approved": 9999,
				"points_pending":  0,
				"points_spent":    0,
				"points_expired":  0,
				"created_at":      "2026-01-01T00:00:00Z",
				"updated_at":      "2026-01-01T00:00:00Z",
			},
		},
	}
	ac.LoadState(state).AssertStatus(200)

	// Original auth should fail
	resp := llGet(tc, "/v2/customers", authAlpha)
	resp.AssertStatus(401)

	// Custom auth should work
	customAuth := map[string]string{
		"Authorization": basicAuth("custom_key", "custom_secret"),
	}
	resp = llGet(tc, "/v2/customers?email=custom@example.com", customAuth)
	resp.AssertStatus(200)
}
