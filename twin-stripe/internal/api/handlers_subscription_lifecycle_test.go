package api_test

import (
	"testing"
)

func TestSubscriptionAdvanceTrialToActive(t *testing.T) {
	_, tc := setupStripe(t)

	// Create product + price.
	resp := stripePost(tc, "/v1/products?name=TrialAdv", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=2000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=TrialAdvCust", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create subscription with trial — trial_end will be in the future.
	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID+"&trial_period_days=1", nil)
	resp.AssertStatus(200)
	sub := resp.JSONMap()
	subID := sub["id"].(string)
	if sub["status"] != "trialing" {
		t.Fatalf("expected trialing, got %v", sub["status"])
	}

	// Advance time past the trial end.
	tc.Post("/admin/time/advance", map[string]any{"duration": "48h"})

	// Advance subscriptions.
	resp = tc.Post("/admin/subscriptions/advance", nil)
	resp.AssertStatus(200)

	// Verify trial→active.
	resp = stripeGet(tc, "/v1/subscriptions/"+subID)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "active" {
		t.Errorf("expected status=active after advance, got %v", resp.JSONMap()["status"])
	}
}

func TestSubscriptionAdvanceCancelAtPeriodEnd(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=CAPE", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=1000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=CAPECust", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID, nil)
	resp.AssertStatus(200)
	subID := resp.JSONMap()["id"].(string)

	// Set cancel_at_period_end.
	stripePost(tc, "/v1/subscriptions/"+subID+"?cancel_at_period_end=true", nil).AssertStatus(200)

	// Advance time past period end.
	tc.Post("/admin/time/advance", map[string]any{"duration": "840h"})

	resp = tc.Post("/admin/subscriptions/advance", nil)
	resp.AssertStatus(200)

	resp = stripeGet(tc, "/v1/subscriptions/"+subID)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "canceled" {
		t.Errorf("expected status=canceled after advance with cancel_at_period_end, got %v", resp.JSONMap()["status"])
	}
	if resp.JSONMap()["canceled_at"] == nil || resp.JSONMap()["canceled_at"].(float64) == 0 {
		t.Error("expected canceled_at to be set")
	}
}

func TestSubscriptionRenewalCreatesInvoice(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=RenewProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=3000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=RenewCust", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID, nil)
	resp.AssertStatus(200)
	sub := resp.JSONMap()
	subID := sub["id"].(string)
	firstInvoice := sub["latest_invoice"].(string)
	firstPeriodEnd := sub["current_period_end"].(float64)

	// Advance past period end.
	tc.Post("/admin/time/advance", map[string]any{"duration": "840h"})

	resp = tc.Post("/admin/subscriptions/advance", nil)
	resp.AssertStatus(200)

	// Verify subscription renewed.
	resp = stripeGet(tc, "/v1/subscriptions/"+subID)
	resp.AssertStatus(200)
	sub = resp.JSONMap()
	if sub["status"] != "active" {
		t.Errorf("expected status=active after renewal, got %v", sub["status"])
	}

	// Period should have advanced.
	newPeriodStart := sub["current_period_start"].(float64)
	if newPeriodStart != firstPeriodEnd {
		t.Errorf("expected new period_start=%v, got %v", firstPeriodEnd, newPeriodStart)
	}

	// New invoice should be created.
	newInvoice := sub["latest_invoice"].(string)
	if newInvoice == firstInvoice {
		t.Error("expected a new invoice after renewal, got same invoice ID")
	}

	// Verify new invoice exists.
	resp = stripeGet(tc, "/v1/invoices/"+newInvoice)
	resp.AssertStatus(200)
	if resp.JSONMap()["total"].(float64) != 3000 {
		t.Errorf("expected renewal invoice total=3000, got %v", resp.JSONMap()["total"])
	}
}

func TestSubscriptionAdvanceNoPrematureTransition(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=NoPremature", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=1000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=NoPremCust", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID+"&trial_period_days=14", nil)
	resp.AssertStatus(200)
	subID := resp.JSONMap()["id"].(string)

	// Advance only 5 days — should NOT transition.
	tc.Post("/admin/time/advance", map[string]any{"duration": "120h"})
	tc.Post("/admin/subscriptions/advance", nil).AssertStatus(200)

	resp = stripeGet(tc, "/v1/subscriptions/"+subID)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "trialing" {
		t.Errorf("expected status=trialing (not enough time passed), got %v", resp.JSONMap()["status"])
	}
}
