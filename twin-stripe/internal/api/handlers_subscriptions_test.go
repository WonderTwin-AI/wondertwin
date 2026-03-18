package api_test

import (
	"testing"
)

func TestSubscriptionRequiresCustomer(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/subscriptions", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("parameter_missing")
}

func TestSubscriptionItemsPopulatedFromPrice(t *testing.T) {
	_, tc := setupStripe(t)

	// Setup.
	resp := stripePost(tc, "/v1/products?name=ItemTest", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=1500&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=Test", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create subscription with 1 item.
	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID, nil)
	resp.AssertStatus(200)
	sub := resp.JSONMap()

	items, ok := sub["items"].(map[string]any)
	if !ok {
		t.Fatal("expected items object")
	}
	data, ok := items["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("expected 1 item, got %d", len(data))
	}

	item := data[0].(map[string]any)
	if item["object"] != "subscription_item" {
		t.Errorf("expected object=subscription_item, got %v", item["object"])
	}
	if item["quantity"].(float64) != 1 {
		t.Errorf("expected quantity=1, got %v", item["quantity"])
	}

	// Check that the embedded price has the right data.
	price, ok := item["price"].(map[string]any)
	if !ok {
		t.Fatal("expected price object in item")
	}
	if price["id"] != priceID {
		t.Errorf("expected price.id=%s, got %v", priceID, price["id"])
	}
	if price["unit_amount"].(float64) != 1500 {
		t.Errorf("expected price.unit_amount=1500, got %v", price["unit_amount"])
	}
}

func TestSubscriptionMultipleItems(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=Multi", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=1000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	price1 := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=2000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	price2 := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=Test", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+price1+"&items[1][price]="+price2+"&items[1][quantity]=3", nil)
	resp.AssertStatus(200)
	sub := resp.JSONMap()

	items := sub["items"].(map[string]any)
	data := items["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(data))
	}

	// Verify invoice total = 1000*1 + 2000*3 = 7000.
	invoiceID := sub["latest_invoice"].(string)
	resp = stripeGet(tc, "/v1/invoices/"+invoiceID)
	resp.AssertStatus(200)
	if resp.JSONMap()["total"].(float64) != 7000 {
		t.Errorf("expected invoice total=7000, got %v", resp.JSONMap()["total"])
	}
}

func TestSubscriptionTrialSetsStatus(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=TrialProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=5000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=TrialCust", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create subscription with 14-day trial.
	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID+"&trial_period_days=14", nil)
	resp.AssertStatus(200)
	sub := resp.JSONMap()

	if sub["status"] != "trialing" {
		t.Fatalf("expected status=trialing, got %v", sub["status"])
	}
	if sub["trial_start"] == nil || sub["trial_start"].(float64) == 0 {
		t.Error("expected trial_start to be set")
	}
	if sub["trial_end"] == nil || sub["trial_end"].(float64) == 0 {
		t.Error("expected trial_end to be set")
	}
	// trial_end should be after trial_start.
	if sub["trial_end"].(float64) <= sub["trial_start"].(float64) {
		t.Error("expected trial_end > trial_start")
	}
}

func TestSubscriptionLatestInvoiceSet(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=InvProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=2000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=Test", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID, nil)
	resp.AssertStatus(200)
	sub := resp.JSONMap()

	invoiceID, ok := sub["latest_invoice"].(string)
	if !ok || invoiceID == "" {
		t.Fatal("expected latest_invoice to be set")
	}

	// Verify the invoice references the subscription.
	resp = stripeGet(tc, "/v1/invoices/"+invoiceID)
	resp.AssertStatus(200)
	if resp.JSONMap()["subscription"] != sub["id"] {
		t.Errorf("expected invoice.subscription=%v, got %v", sub["id"], resp.JSONMap()["subscription"])
	}
}

func TestCancelSubscriptionSetsStatusAndCanceledAt(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=CancelProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=1000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=Test", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID, nil)
	resp.AssertStatus(200)
	subID := resp.JSONMap()["id"].(string)

	// Cancel.
	resp = tc.DoWithHeaders("DELETE", "/v1/subscriptions/"+subID, nil, map[string]string{
		"Authorization": "Bearer sk_test_sim_123",
	})
	resp.AssertStatus(200)
	sub := resp.JSONMap()

	if sub["status"] != "canceled" {
		t.Fatalf("expected status=canceled, got %v", sub["status"])
	}
	if sub["canceled_at"] == nil || sub["canceled_at"].(float64) == 0 {
		t.Error("expected canceled_at to be set")
	}
}

func TestSubscriptionCancelAtPeriodEnd(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=CAPE", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=1000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=Test", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID, nil)
	resp.AssertStatus(200)
	subID := resp.JSONMap()["id"].(string)

	// Update to cancel_at_period_end=true.
	resp = stripePost(tc, "/v1/subscriptions/"+subID+"?cancel_at_period_end=true", nil)
	resp.AssertStatus(200)
	sub := resp.JSONMap()
	if sub["cancel_at_period_end"] != true {
		t.Fatalf("expected cancel_at_period_end=true, got %v", sub["cancel_at_period_end"])
	}
	// Should still be active.
	if sub["status"] != "active" {
		t.Errorf("expected status=active (not immediately canceled), got %v", sub["status"])
	}

	// Verify persistence via GET.
	resp = stripeGet(tc, "/v1/subscriptions/"+subID)
	resp.AssertStatus(200)
	if resp.JSONMap()["cancel_at_period_end"] != true {
		t.Error("cancel_at_period_end should persist on GET")
	}

	// Un-cancel.
	resp = stripePost(tc, "/v1/subscriptions/"+subID+"?cancel_at_period_end=false", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["cancel_at_period_end"] != false {
		t.Errorf("expected cancel_at_period_end=false after un-cancel, got %v", resp.JSONMap()["cancel_at_period_end"])
	}
}

func TestSubscriptionCurrentPeriodSetFromPrice(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=PeriodTest", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	// Create a yearly recurring price.
	resp = stripePost(tc, "/v1/prices?unit_amount=12000&currency=usd&product="+prodID+"&recurring[interval]=year", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=Test", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID, nil)
	resp.AssertStatus(200)
	sub := resp.JSONMap()

	start := sub["current_period_start"].(float64)
	end := sub["current_period_end"].(float64)

	// For a yearly interval, the period should be approximately 365 days.
	diffDays := (end - start) / 86400
	if diffDays < 364 || diffDays > 366 {
		t.Errorf("expected ~365 day period for yearly subscription, got %.0f days", diffDays)
	}
}

func TestSubscriptionSendInvoiceMode(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=SendInv", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=2000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=Test", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create with collection_method=send_invoice.
	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID+"&collection_method=send_invoice", nil)
	resp.AssertStatus(200)
	sub := resp.JSONMap()
	invoiceID := sub["latest_invoice"].(string)

	// Verify invoice is open (not auto-paid).
	resp = stripeGet(tc, "/v1/invoices/"+invoiceID)
	resp.AssertStatus(200)
	inv := resp.JSONMap()
	if inv["status"] != "open" {
		t.Errorf("expected invoice status=open for send_invoice mode, got %v", inv["status"])
	}
	if inv["paid"] != false {
		t.Errorf("expected paid=false for send_invoice mode, got %v", inv["paid"])
	}
	if inv["amount_remaining"].(float64) != 2000 {
		t.Errorf("expected amount_remaining=2000, got %v", inv["amount_remaining"])
	}
}

func TestSubscriptionInvalidPriceReturnsError(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=Test", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]=price_nonexistent", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("resource_missing")
}

func TestSubscriptionListFilterByCustomer(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=ListTest", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=1000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=Cust1", nil)
	resp.AssertStatus(200)
	cust1 := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=Cust2", nil)
	resp.AssertStatus(200)
	cust2 := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/subscriptions?customer="+cust1+"&items[0][price]="+priceID, nil).AssertStatus(200)
	stripePost(tc, "/v1/subscriptions?customer="+cust2+"&items[0][price]="+priceID, nil).AssertStatus(200)

	resp = stripeGet(tc, "/v1/subscriptions?customer="+cust1)
	resp.AssertStatus(200)
	data := resp.JSONMap()["data"].([]any)
	if len(data) != 1 {
		t.Errorf("expected 1 subscription for cust1, got %d", len(data))
	}
}
