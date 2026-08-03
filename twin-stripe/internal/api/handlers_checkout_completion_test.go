package api_test

import (
	"testing"
)

func TestCompletePaymentCheckoutSession(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a checkout session in payment mode with an explicit amount.
	resp := stripePostWithParams(tc, "/v1/checkout/sessions", map[string]string{
		"mode":         "payment",
		"amount_total": "5000",
		"currency":     "usd",
	})
	resp.AssertStatus(200)
	csID := resp.JSONMap()["id"].(string)
	if resp.JSONMap()["status"] != "open" {
		t.Fatalf("expected status=open, got %v", resp.JSONMap()["status"])
	}

	// Complete it via admin endpoint.
	resp = tc.Post("/admin/checkout/sessions/"+csID+"/complete", nil)
	resp.AssertStatus(200)
	cs := resp.JSONMap()

	if cs["status"] != "complete" {
		t.Errorf("expected status=complete, got %v", cs["status"])
	}
	if cs["payment_status"] != "paid" {
		t.Errorf("expected payment_status=paid, got %v", cs["payment_status"])
	}

	// Should have a payment_intent.
	piID, ok := cs["payment_intent"].(string)
	if !ok || piID == "" {
		t.Fatal("expected payment_intent to be set after completion")
	}

	// Verify the PI exists and is succeeded.
	resp = stripeGet(tc, "/v1/payment_intents/"+piID)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "succeeded" {
		t.Errorf("expected PI status=succeeded, got %v", resp.JSONMap()["status"])
	}

	// Verify a charge was created.
	chargeID := resp.JSONMap()["latest_charge"].(string)
	resp = stripeGet(tc, "/v1/charges/"+chargeID)
	resp.AssertStatus(200)
	if resp.JSONMap()["amount"].(float64) != 5000 {
		t.Errorf("expected charge amount=5000, got %v", resp.JSONMap()["amount"])
	}
}

func TestCompleteSubscriptionCheckoutSession(t *testing.T) {
	_, tc := setupStripe(t)

	// Create product + price.
	resp := stripePost(tc, "/v1/products?name=CheckoutProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=3000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	// Create customer.
	resp = stripePost(tc, "/v1/customers?name=CheckoutCust", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create session with mode=subscription and line items.
	resp = stripePostWithParams(tc, "/v1/checkout/sessions", map[string]string{
		"mode":                    "subscription",
		"customer":                custID,
		"line_items[0][price]":    priceID,
		"line_items[0][quantity]": "1",
	})
	resp.AssertStatus(200)
	csID := resp.JSONMap()["id"].(string)

	// Complete.
	resp = tc.Post("/admin/checkout/sessions/"+csID+"/complete", nil)
	resp.AssertStatus(200)
	cs := resp.JSONMap()

	if cs["status"] != "complete" {
		t.Errorf("expected status=complete, got %v", cs["status"])
	}

	// Should have a subscription.
	subID, ok := cs["subscription"].(string)
	if !ok || subID == "" {
		t.Fatal("expected subscription to be set after completion")
	}

	// Verify subscription exists.
	resp = stripeGet(tc, "/v1/subscriptions/"+subID)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "active" {
		t.Errorf("expected sub status=active, got %v", resp.JSONMap()["status"])
	}
	if resp.JSONMap()["customer"] != custID {
		t.Errorf("expected sub customer=%s, got %v", custID, resp.JSONMap()["customer"])
	}
}

func TestDoubleCompleteFails(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePostWithParams(tc, "/v1/checkout/sessions", map[string]string{
		"mode":         "payment",
		"amount_total": "1000",
		"currency":     "usd",
	})
	resp.AssertStatus(200)
	csID := resp.JSONMap()["id"].(string)

	// First complete succeeds.
	tc.Post("/admin/checkout/sessions/"+csID+"/complete", nil).AssertStatus(200)

	// Second complete fails.
	resp = tc.Post("/admin/checkout/sessions/"+csID+"/complete", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("can not be completed")
}

func TestExpireThenCompleteFails(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePostWithParams(tc, "/v1/checkout/sessions", map[string]string{
		"mode": "payment",
	})
	resp.AssertStatus(200)
	csID := resp.JSONMap()["id"].(string)

	// Expire it.
	stripePost(tc, "/v1/checkout/sessions/"+csID+"/expire", nil).AssertStatus(200)

	// Complete should fail.
	resp = tc.Post("/admin/checkout/sessions/"+csID+"/complete", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("can not be completed")
}

func TestCheckoutSessionLineItems(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=LIProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=2500&currency=usd&product="+prodID, nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	// Create session with line items.
	resp = stripePostWithParams(tc, "/v1/checkout/sessions", map[string]string{
		"mode":                    "payment",
		"line_items[0][price]":    priceID,
		"line_items[0][quantity]": "2",
	})
	resp.AssertStatus(200)
	cs := resp.JSONMap()

	// Amount should be auto-computed: 2500 * 2 = 5000.
	if cs["amount_total"].(float64) != 5000 {
		t.Errorf("expected amount_total=5000, got %v", cs["amount_total"])
	}
}
