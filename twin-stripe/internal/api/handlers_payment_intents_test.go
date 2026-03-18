package api_test

import (
	"testing"
)

func TestCreatePaymentIntentWithConfirmCreatesCharge(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a payment method first.
	resp := stripePost(tc, "/v1/payment_methods?type=card&card[number]=4242424242424242&card[exp_month]=12&card[exp_year]=2030&card[cvc]=123", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	// Create PI with confirm=true + payment_method.
	resp = stripePost(tc, "/v1/payment_intents?amount=5000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)

	pi := resp.JSONMap()
	if pi["status"] != "succeeded" {
		t.Fatalf("expected status=succeeded, got %v", pi["status"])
	}
	if pi["amount_received"].(float64) != 5000 {
		t.Errorf("expected amount_received=5000, got %v", pi["amount_received"])
	}
	chargeID, ok := pi["latest_charge"].(string)
	if !ok || chargeID == "" {
		t.Fatal("expected latest_charge to be set")
	}

	// Verify the charge was created with correct fields.
	resp = stripeGet(tc, "/v1/charges/"+chargeID)
	resp.AssertStatus(200)
	ch := resp.JSONMap()
	if ch["amount"].(float64) != 5000 {
		t.Errorf("charge amount: expected 5000, got %v", ch["amount"])
	}
	if ch["currency"] != "usd" {
		t.Errorf("charge currency: expected usd, got %v", ch["currency"])
	}
	if ch["captured"] != true {
		t.Errorf("expected charge captured=true, got %v", ch["captured"])
	}
	if ch["paid"] != true {
		t.Errorf("expected charge paid=true, got %v", ch["paid"])
	}
	if ch["payment_intent"] != pi["id"] {
		t.Errorf("charge payment_intent: expected %v, got %v", pi["id"], ch["payment_intent"])
	}
	if ch["status"] != "succeeded" {
		t.Errorf("charge status: expected succeeded, got %v", ch["status"])
	}
}

func TestConfirmPaymentIntentTransitionsStatus(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a payment method.
	resp := stripePost(tc, "/v1/payment_methods?type=card&card[number]=4242424242424242&card[exp_month]=12&card[exp_year]=2030&card[cvc]=123", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	// Create PI without confirming.
	resp = stripePost(tc, "/v1/payment_intents?amount=3000&currency=usd&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	pi := resp.JSONMap()
	piID := pi["id"].(string)
	if pi["status"] != "requires_confirmation" {
		t.Fatalf("expected requires_confirmation, got %v", pi["status"])
	}

	// Confirm it.
	resp = stripePost(tc, "/v1/payment_intents/"+piID+"/confirm", nil)
	resp.AssertStatus(200)
	pi = resp.JSONMap()
	if pi["status"] != "succeeded" {
		t.Fatalf("expected succeeded after confirm, got %v", pi["status"])
	}
	if pi["amount_received"].(float64) != 3000 {
		t.Errorf("expected amount_received=3000, got %v", pi["amount_received"])
	}
	if pi["latest_charge"] == nil || pi["latest_charge"] == "" {
		t.Error("expected latest_charge after confirm")
	}
}

func TestConfirmPaymentIntentWithPaymentMethod(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a PM.
	resp := stripePost(tc, "/v1/payment_methods?type=card&card[number]=4242424242424242&card[exp_month]=12&card[exp_year]=2030&card[cvc]=123", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	// Create PI without a payment method.
	resp = stripePost(tc, "/v1/payment_intents?amount=1500&currency=usd", nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)
	if resp.JSONMap()["status"] != "requires_payment_method" {
		t.Fatalf("expected requires_payment_method, got %v", resp.JSONMap()["status"])
	}

	// Confirm with payment_method on the confirm call.
	resp = stripePost(tc, "/v1/payment_intents/"+piID+"/confirm?payment_method="+pmID, nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "succeeded" {
		t.Fatalf("expected succeeded, got %v", resp.JSONMap()["status"])
	}
	if resp.JSONMap()["payment_method"] != pmID {
		t.Errorf("expected payment_method=%s, got %v", pmID, resp.JSONMap()["payment_method"])
	}
}

func TestCancelPaymentIntentRejectsSucceeded(t *testing.T) {
	_, tc := setupStripe(t)

	// Create and confirm a PI.
	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=2000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)
	if resp.JSONMap()["status"] != "succeeded" {
		t.Fatalf("expected succeeded, got %v", resp.JSONMap()["status"])
	}

	// Cancel should fail.
	resp = stripePost(tc, "/v1/payment_intents/"+piID+"/cancel", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("payment_intent_unexpected_state")
}

func TestCancelPaymentIntentRejectsCanceled(t *testing.T) {
	_, tc := setupStripe(t)

	// Create PI.
	resp := stripePost(tc, "/v1/payment_intents?amount=1000&currency=usd", nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)

	// Cancel once.
	resp = stripePost(tc, "/v1/payment_intents/"+piID+"/cancel", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "canceled" {
		t.Fatalf("expected canceled, got %v", resp.JSONMap()["status"])
	}

	// Cancel again should fail.
	resp = stripePost(tc, "/v1/payment_intents/"+piID+"/cancel", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("payment_intent_unexpected_state")
}

func TestCancelPaymentIntentSetsFields(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_intents?amount=1000&currency=usd", nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents/"+piID+"/cancel?cancellation_reason=requested_by_customer", nil)
	resp.AssertStatus(200)
	pi := resp.JSONMap()
	if pi["status"] != "canceled" {
		t.Errorf("expected canceled, got %v", pi["status"])
	}
	if pi["canceled_at"] == nil || pi["canceled_at"].(float64) == 0 {
		t.Error("expected canceled_at to be set")
	}
	if pi["cancellation_reason"] != "requested_by_customer" {
		t.Errorf("expected cancellation_reason=requested_by_customer, got %v", pi["cancellation_reason"])
	}
}

func TestPaymentIntentBalanceCredited(t *testing.T) {
	_, tc := setupStripe(t)

	// Check balance before.
	resp := stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	balBefore := getAvailableBalance(t, resp.JSONMap(), "usd")

	// Create a confirmed PI.
	resp = stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=7500&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)

	// Check balance after.
	resp = stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	balAfter := getAvailableBalance(t, resp.JSONMap(), "usd")

	if balAfter-balBefore != 7500 {
		t.Errorf("expected balance increase of 7500, got %v (before=%v, after=%v)", balAfter-balBefore, balBefore, balAfter)
	}
}

func TestPaymentIntentRequiresAmountAndCurrency(t *testing.T) {
	_, tc := setupStripe(t)

	// Missing both.
	resp := stripePost(tc, "/v1/payment_intents", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("parameter_missing")

	// Missing currency.
	resp = stripePost(tc, "/v1/payment_intents?amount=1000", nil)
	resp.AssertStatus(400)

	// Missing amount.
	resp = stripePost(tc, "/v1/payment_intents?currency=usd", nil)
	resp.AssertStatus(400)
}

func TestCreatePaymentIntentCaptureMethodDefault(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_intents?amount=1000&currency=usd", nil)
	resp.AssertStatus(200)
	pi := resp.JSONMap()
	if pi["capture_method"] != "automatic" {
		t.Errorf("expected capture_method=automatic, got %v", pi["capture_method"])
	}
	if pi["confirmation_method"] != "automatic" {
		t.Errorf("expected confirmation_method=automatic, got %v", pi["confirmation_method"])
	}
}

func TestConfirmPaymentIntentRejectsInvalidStatus(t *testing.T) {
	_, tc := setupStripe(t)

	// Create and confirm a PI to put it in succeeded state.
	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=1000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)

	// Try to confirm again — should fail.
	resp = stripePost(tc, "/v1/payment_intents/"+piID+"/confirm", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("payment_intent_unexpected_state")
}

// getAvailableBalance extracts the available balance for a given currency.
func getAvailableBalance(t *testing.T, bal map[string]any, currency string) float64 {
	t.Helper()
	avail, ok := bal["available"].([]any)
	if !ok {
		t.Fatal("expected available array in balance")
	}
	for _, a := range avail {
		am := a.(map[string]any)
		if am["currency"] == currency {
			return am["amount"].(float64)
		}
	}
	return 0
}
