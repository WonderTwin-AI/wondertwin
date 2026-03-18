package api_test

import (
	"testing"
)

func TestCreateChargeDirectly(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/charges?amount=2500&currency=usd&description=Direct+charge", nil)
	resp.AssertStatus(200)

	ch := resp.JSONMap()
	if ch["object"] != "charge" {
		t.Errorf("expected object=charge, got %v", ch["object"])
	}
	if ch["amount"].(float64) != 2500 {
		t.Errorf("expected amount=2500, got %v", ch["amount"])
	}
	if ch["currency"] != "usd" {
		t.Errorf("expected currency=usd, got %v", ch["currency"])
	}
	if ch["captured"] != true {
		t.Errorf("expected captured=true, got %v", ch["captured"])
	}
	if ch["paid"] != true {
		t.Errorf("expected paid=true, got %v", ch["paid"])
	}
	if ch["status"] != "succeeded" {
		t.Errorf("expected status=succeeded, got %v", ch["status"])
	}
	if ch["description"] != "Direct charge" {
		t.Errorf("expected description='Direct charge', got %v", ch["description"])
	}

	// Verify it's retrievable.
	resp = stripeGet(tc, "/v1/charges/"+ch["id"].(string))
	resp.AssertStatus(200)
}

func TestCreateChargeCreditsBalance(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	before := getAvailableBalance(t, resp.JSONMap(), "usd")

	stripePost(tc, "/v1/charges?amount=3000&currency=usd", nil).AssertStatus(200)

	resp = stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	after := getAvailableBalance(t, resp.JSONMap(), "usd")

	if after-before != 3000 {
		t.Errorf("expected balance increase of 3000, got %v", after-before)
	}
}

func TestCreateChargeMissingParams(t *testing.T) {
	_, tc := setupStripe(t)

	// Missing both.
	resp := stripePost(tc, "/v1/charges", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("parameter_missing")

	// Missing currency.
	resp = stripePost(tc, "/v1/charges?amount=1000", nil)
	resp.AssertStatus(400)

	// Missing amount.
	resp = stripePost(tc, "/v1/charges?currency=usd", nil)
	resp.AssertStatus(400)
}

func TestManualCaptureFlow(t *testing.T) {
	_, tc := setupStripe(t)

	// Create payment method.
	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	// Create PI with manual capture.
	resp = stripePost(tc, "/v1/payment_intents?amount=5000&currency=usd&capture_method=manual&payment_method="+pmID+"&confirm=true", nil)
	resp.AssertStatus(200)
	pi := resp.JSONMap()
	piID := pi["id"].(string)

	// After confirm with capture_method=manual, status should be requires_capture.
	if pi["status"] != "requires_capture" {
		t.Fatalf("expected status=requires_capture, got %v", pi["status"])
	}

	// Balance should NOT be credited yet.
	resp = stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	balBeforeCapture := getAvailableBalance(t, resp.JSONMap(), "usd")

	// Capture.
	resp = stripePost(tc, "/v1/payment_intents/"+piID+"/capture", nil)
	resp.AssertStatus(200)
	pi = resp.JSONMap()
	if pi["status"] != "succeeded" {
		t.Fatalf("expected status=succeeded after capture, got %v", pi["status"])
	}
	if pi["amount_received"].(float64) != 5000 {
		t.Errorf("expected amount_received=5000, got %v", pi["amount_received"])
	}

	// Balance should now be credited.
	resp = stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	balAfterCapture := getAvailableBalance(t, resp.JSONMap(), "usd")
	if balAfterCapture-balBeforeCapture != 5000 {
		t.Errorf("expected balance increase of 5000 after capture, got %v", balAfterCapture-balBeforeCapture)
	}

	// Charge should now be captured.
	chargeID := pi["latest_charge"].(string)
	resp = stripeGet(tc, "/v1/charges/"+chargeID)
	resp.AssertStatus(200)
	if resp.JSONMap()["captured"] != true {
		t.Error("expected charge captured=true after capture")
	}
}

func TestManualCaptureConfirmCreatesUncapturedCharge(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=3000&currency=usd&capture_method=manual&payment_method="+pmID+"&confirm=true", nil)
	resp.AssertStatus(200)

	chargeID := resp.JSONMap()["latest_charge"].(string)
	resp = stripeGet(tc, "/v1/charges/"+chargeID)
	resp.AssertStatus(200)
	ch := resp.JSONMap()
	if ch["captured"] != false {
		t.Error("expected charge captured=false before capture (manual capture mode)")
	}
}

func TestPartialCapture(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=10000&currency=usd&capture_method=manual&payment_method="+pmID+"&confirm=true", nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)

	// Capture only 7000 of 10000.
	resp = stripePost(tc, "/v1/payment_intents/"+piID+"/capture?amount_to_capture=7000", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["amount_received"].(float64) != 7000 {
		t.Errorf("expected amount_received=7000, got %v", resp.JSONMap()["amount_received"])
	}
}

func TestRecaptureFails(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=5000&currency=usd&capture_method=manual&payment_method="+pmID+"&confirm=true", nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)

	// First capture succeeds.
	stripePost(tc, "/v1/payment_intents/"+piID+"/capture", nil).AssertStatus(200)

	// Second capture fails (status is now succeeded, not requires_capture).
	resp = stripePost(tc, "/v1/payment_intents/"+piID+"/capture", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("payment_intent_unexpected_state")
}

func TestCaptureOnAutomaticPIFails(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	// Create PI with automatic capture (default).
	resp = stripePost(tc, "/v1/payment_intents?amount=2000&currency=usd&payment_method="+pmID+"&confirm=true", nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)

	// PI is already succeeded (automatic capture), trying to capture should fail.
	resp = stripePost(tc, "/v1/payment_intents/"+piID+"/capture", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("payment_intent_unexpected_state")
}

func TestCaptureExceedingAmountFails(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=5000&currency=usd&capture_method=manual&payment_method="+pmID+"&confirm=true", nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents/"+piID+"/capture?amount_to_capture=9999", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("amount_too_large")
}
