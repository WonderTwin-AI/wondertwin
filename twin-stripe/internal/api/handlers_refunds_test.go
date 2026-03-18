package api_test

import (
	"testing"
)

func TestFullRefundSetsChargeRefunded(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a confirmed PI to get a charge.
	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=5000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	chargeID := resp.JSONMap()["latest_charge"].(string)

	// Full refund (no amount param = full refund).
	resp = stripePost(tc, "/v1/refunds?charge="+chargeID, nil)
	resp.AssertStatus(200)
	ref := resp.JSONMap()
	if ref["amount"].(float64) != 5000 {
		t.Errorf("expected refund amount=5000, got %v", ref["amount"])
	}
	if ref["status"] != "succeeded" {
		t.Errorf("expected refund status=succeeded, got %v", ref["status"])
	}

	// Verify charge is marked as fully refunded.
	resp = stripeGet(tc, "/v1/charges/"+chargeID)
	resp.AssertStatus(200)
	ch := resp.JSONMap()
	if ch["refunded"] != true {
		t.Errorf("expected charge refunded=true, got %v", ch["refunded"])
	}
	if ch["amount_refunded"].(float64) != 5000 {
		t.Errorf("expected amount_refunded=5000, got %v", ch["amount_refunded"])
	}
}

func TestPartialRefundUpdatesAmountRefunded(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=10000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	chargeID := resp.JSONMap()["latest_charge"].(string)

	// Partial refund of 3000.
	resp = stripePost(tc, "/v1/refunds?charge="+chargeID+"&amount=3000", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["amount"].(float64) != 3000 {
		t.Errorf("expected refund amount=3000, got %v", resp.JSONMap()["amount"])
	}

	// Charge should NOT be fully refunded yet.
	resp = stripeGet(tc, "/v1/charges/"+chargeID)
	resp.AssertStatus(200)
	ch := resp.JSONMap()
	if ch["refunded"] != false {
		t.Errorf("expected charge refunded=false after partial refund, got %v", ch["refunded"])
	}
	if ch["amount_refunded"].(float64) != 3000 {
		t.Errorf("expected amount_refunded=3000, got %v", ch["amount_refunded"])
	}
}

func TestExcessRefundReturns400(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=2000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	chargeID := resp.JSONMap()["latest_charge"].(string)

	// Try to refund more than the charge amount.
	resp = stripePost(tc, "/v1/refunds?charge="+chargeID+"&amount=3000", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("amount_too_large")
}

func TestRefundByPaymentIntent(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=4000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)
	chargeID := resp.JSONMap()["latest_charge"].(string)

	// Refund by payment_intent instead of charge.
	resp = stripePost(tc, "/v1/refunds?payment_intent="+piID, nil)
	resp.AssertStatus(200)
	ref := resp.JSONMap()
	if ref["charge"] != chargeID {
		t.Errorf("expected refund.charge=%s, got %v", chargeID, ref["charge"])
	}
	if ref["payment_intent"] != piID {
		t.Errorf("expected refund.payment_intent=%s, got %v", piID, ref["payment_intent"])
	}
	if ref["amount"].(float64) != 4000 {
		t.Errorf("expected refund amount=4000, got %v", ref["amount"])
	}
}

func TestDoublePartialRefundExceedingTotalFails(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=5000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	chargeID := resp.JSONMap()["latest_charge"].(string)

	// First partial refund: 3000.
	resp = stripePost(tc, "/v1/refunds?charge="+chargeID+"&amount=3000", nil)
	resp.AssertStatus(200)

	// Second partial refund: 3000 — should fail (total would be 6000 > 5000).
	resp = stripePost(tc, "/v1/refunds?charge="+chargeID+"&amount=3000", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("amount_too_large")
}

func TestRefundBalanceDebited(t *testing.T) {
	_, tc := setupStripe(t)

	// Create confirmed PI.
	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=8000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	chargeID := resp.JSONMap()["latest_charge"].(string)

	// Balance after charge.
	resp = stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	balAfterCharge := getAvailableBalance(t, resp.JSONMap(), "usd")

	// Refund.
	resp = stripePost(tc, "/v1/refunds?charge="+chargeID, nil)
	resp.AssertStatus(200)

	// Balance after refund.
	resp = stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	balAfterRefund := getAvailableBalance(t, resp.JSONMap(), "usd")

	if balAfterCharge-balAfterRefund != 8000 {
		t.Errorf("expected balance decrease of 8000, got %v (after charge=%v, after refund=%v)", balAfterCharge-balAfterRefund, balAfterCharge, balAfterRefund)
	}
}

func TestRefundRequiresChargeOrPaymentIntent(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/refunds", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("Must provide charge or payment_intent")
}

func TestSecondFullRefundDefaultsToRemaining(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=6000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	chargeID := resp.JSONMap()["latest_charge"].(string)

	// Partial refund: 4000.
	resp = stripePost(tc, "/v1/refunds?charge="+chargeID+"&amount=4000", nil)
	resp.AssertStatus(200)

	// Full refund of remaining (no amount param).
	resp = stripePost(tc, "/v1/refunds?charge="+chargeID, nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["amount"].(float64) != 2000 {
		t.Errorf("expected remaining refund=2000, got %v", resp.JSONMap()["amount"])
	}

	// Charge should now be fully refunded.
	resp = stripeGet(tc, "/v1/charges/"+chargeID)
	resp.AssertStatus(200)
	if resp.JSONMap()["refunded"] != true {
		t.Error("expected charge refunded=true after full refund")
	}
}
