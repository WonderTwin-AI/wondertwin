package api_test

import (
	"testing"
)

func TestCard4242Succeeds(t *testing.T) {
	_, tc := setupStripe(t)

	// Create PM with 4242 card.
	resp := stripePost(tc, "/v1/payment_methods?type=card&card[number]=4242424242424242&card[exp_month]=12&card[exp_year]=2030", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	// Create and confirm PI — should succeed.
	resp = stripePost(tc, "/v1/payment_intents?amount=1000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "succeeded" {
		t.Errorf("expected status=succeeded, got %v", resp.JSONMap()["status"])
	}
}

func TestCardDeclined(t *testing.T) {
	_, tc := setupStripe(t)

	// Create PM with generic decline card.
	resp := stripePost(tc, "/v1/payment_methods?type=card&card[number]=4000000000000002", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	// Create and confirm PI — should fail with 402.
	resp = stripePost(tc, "/v1/payment_intents?amount=1000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(402)
	resp.AssertBodyContains("card_declined")
}

func TestCardInsufficientFunds(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_methods?type=card&card[number]=4000000000009995", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=1000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(402)
	resp.AssertBodyContains("insufficient_funds")
}

func TestUnknownCardSucceeds(t *testing.T) {
	_, tc := setupStripe(t)

	// Create PM with unknown card number.
	resp := stripePost(tc, "/v1/payment_methods?type=card&card[number]=4111111111111111", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=1000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "succeeded" {
		t.Errorf("expected status=succeeded for unknown card, got %v", resp.JSONMap()["status"])
	}
}

func TestCardDeclineOnConfirm(t *testing.T) {
	_, tc := setupStripe(t)

	// Create PM with decline card.
	resp := stripePost(tc, "/v1/payment_methods?type=card&card[number]=4000000000000002", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	// Create PI without confirming.
	resp = stripePost(tc, "/v1/payment_intents?amount=2000&currency=usd&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)

	// Confirm — should fail.
	resp = stripePost(tc, "/v1/payment_intents/"+piID+"/confirm", nil)
	resp.AssertStatus(402)
	resp.AssertBodyContains("card_declined")
}

func TestCardDeclineOnDirectCharge(t *testing.T) {
	_, tc := setupStripe(t)

	// Create PM with decline card.
	resp := stripePost(tc, "/v1/payment_methods?type=card&card[number]=4000000000009995", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	// Create charge with source = the PM — should fail.
	resp = stripePost(tc, "/v1/charges?amount=500&currency=usd&source="+pmID, nil)
	resp.AssertStatus(402)
	resp.AssertBodyContains("insufficient_funds")
}
