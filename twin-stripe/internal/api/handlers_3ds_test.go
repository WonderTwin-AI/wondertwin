package api_test

import (
	"testing"
)

func TestCard3DSRequiresAction(t *testing.T) {
	_, tc := setupStripe(t)

	// Create PM with 3DS-required card.
	resp := stripePost(tc, "/v1/payment_methods?type=card&card[number]=4000000000003220", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	// Create and confirm PI — should get requires_action, not succeeded.
	resp = stripePost(tc, "/v1/payment_intents?amount=2000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	pi := resp.JSONMap()

	if pi["status"] != "requires_action" {
		t.Fatalf("expected status=requires_action, got %v", pi["status"])
	}
	// Should NOT have a charge yet.
	if pi["latest_charge"] != nil && pi["latest_charge"] != "" {
		t.Error("expected no charge before authentication")
	}
}

func TestCard3DSConfirmRequiresAction(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_methods?type=card&card[number]=4000000000003063", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	// Create PI without confirming.
	resp = stripePost(tc, "/v1/payment_intents?amount=1500&currency=usd&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)

	// Confirm — should get requires_action.
	resp = stripePost(tc, "/v1/payment_intents/"+piID+"/confirm", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "requires_action" {
		t.Fatalf("expected status=requires_action, got %v", resp.JSONMap()["status"])
	}
}

func TestAdminAuthenticateCompletes3DS(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_methods?type=card&card[number]=4000000000003220", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=3000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)
	if resp.JSONMap()["status"] != "requires_action" {
		t.Fatalf("expected requires_action, got %v", resp.JSONMap()["status"])
	}

	// Simulate user completing 3DS.
	resp = tc.Post("/admin/payment_intents/"+piID+"/authenticate", nil)
	resp.AssertStatus(200)
	pi := resp.JSONMap()

	if pi["status"] != "succeeded" {
		t.Errorf("expected status=succeeded after auth, got %v", pi["status"])
	}
	if pi["amount_received"].(float64) != 3000 {
		t.Errorf("expected amount_received=3000, got %v", pi["amount_received"])
	}
	chargeID, ok := pi["latest_charge"].(string)
	if !ok || chargeID == "" {
		t.Fatal("expected latest_charge after authentication")
	}

	// Verify charge exists.
	resp = stripeGet(tc, "/v1/charges/"+chargeID)
	resp.AssertStatus(200)
	if resp.JSONMap()["amount"].(float64) != 3000 {
		t.Errorf("expected charge amount=3000, got %v", resp.JSONMap()["amount"])
	}
}

func TestAuthenticateNonRequiresActionFails(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a regular PI (not requires_action).
	resp := stripePost(tc, "/v1/payment_intents?amount=1000&currency=usd", nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)

	resp = tc.Post("/admin/payment_intents/"+piID+"/authenticate", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("payment_intent_unexpected_state")
}

func TestAuthenticateCreditsBalance(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	before := getAvailableBalance(t, resp.JSONMap(), "usd")

	resp = stripePost(tc, "/v1/payment_methods?type=card&card[number]=4000000000003220", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=5000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)

	// Balance should NOT have changed yet (requires_action).
	resp = stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	during := getAvailableBalance(t, resp.JSONMap(), "usd")
	if during != before {
		t.Errorf("expected balance unchanged during requires_action, before=%v during=%v", before, during)
	}

	// Authenticate.
	tc.Post("/admin/payment_intents/"+piID+"/authenticate", nil).AssertStatus(200)

	resp = stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	after := getAvailableBalance(t, resp.JSONMap(), "usd")
	if after-before != 5000 {
		t.Errorf("expected balance increase of 5000 after auth, got %v", after-before)
	}
}
