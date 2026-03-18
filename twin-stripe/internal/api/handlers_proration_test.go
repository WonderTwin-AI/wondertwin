package api_test

import (
	"testing"
)

func TestProrationOnPriceUpgrade(t *testing.T) {
	_, tc := setupStripe(t)

	// Create product + two prices.
	resp := stripePost(tc, "/v1/products?name=ProProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=1000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	cheapPrice := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=3000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	expensivePrice := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=ProCust", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create subscription on cheap price.
	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+cheapPrice, nil)
	resp.AssertStatus(200)
	subID := resp.JSONMap()["id"].(string)
	firstInvoice := resp.JSONMap()["latest_invoice"].(string)

	// Advance time ~halfway through the period (15 days).
	tc.Post("/admin/time/advance", map[string]any{"duration": "360h"})

	// Upgrade to expensive price.
	resp = stripePost(tc, "/v1/subscriptions/"+subID+"?items[0][price]="+expensivePrice, nil)
	resp.AssertStatus(200)

	// Should have a new proration invoice.
	newInvoice := resp.JSONMap()["latest_invoice"].(string)
	if newInvoice == firstInvoice {
		t.Fatal("expected a new proration invoice after upgrade")
	}

	// Verify proration invoice exists and has correct structure.
	resp = stripeGet(tc, "/v1/invoices/"+newInvoice)
	resp.AssertStatus(200)
	inv := resp.JSONMap()

	if inv["description"] != "Proration" {
		t.Errorf("expected description=Proration, got %v", inv["description"])
	}

	// Net should be positive (upgrading costs more).
	total := inv["total"].(float64)
	if total <= 0 {
		t.Errorf("expected positive proration total for upgrade, got %v", total)
	}

	// Check line items: should have credit + charge.
	lines := inv["lines"].(map[string]any)
	lineData := lines["data"].([]any)
	if len(lineData) != 2 {
		t.Fatalf("expected 2 proration line items (credit + charge), got %d", len(lineData))
	}

	// One should be negative (credit), one positive (charge).
	var hasCredit, hasCharge bool
	for _, l := range lineData {
		line := l.(map[string]any)
		amt := line["amount"].(float64)
		if amt < 0 {
			hasCredit = true
		}
		if amt > 0 {
			hasCharge = true
		}
	}
	if !hasCredit || !hasCharge {
		t.Error("expected one negative (credit) and one positive (charge) line item")
	}

	// Items should be updated.
	items := resp.JSONMap()
	_ = items // verified by checking the subscription directly
	resp = stripeGet(tc, "/v1/subscriptions/"+subID)
	resp.AssertStatus(200)
	subItems := resp.JSONMap()["items"].(map[string]any)["data"].([]any)
	if len(subItems) != 1 {
		t.Fatalf("expected 1 item, got %d", len(subItems))
	}
	price := subItems[0].(map[string]any)["price"].(map[string]any)
	if price["id"] != expensivePrice {
		t.Errorf("expected price=%s, got %v", expensivePrice, price["id"])
	}
}

func TestProrationOnPriceDowngrade(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=DownProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=5000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	expensivePrice := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=1000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	cheapPrice := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=DownCust", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+expensivePrice, nil)
	resp.AssertStatus(200)
	subID := resp.JSONMap()["id"].(string)

	tc.Post("/admin/time/advance", map[string]any{"duration": "360h"})

	resp = stripePost(tc, "/v1/subscriptions/"+subID+"?items[0][price]="+cheapPrice, nil)
	resp.AssertStatus(200)

	invID := resp.JSONMap()["latest_invoice"].(string)
	resp = stripeGet(tc, "/v1/invoices/"+invID)
	resp.AssertStatus(200)

	// Net should be negative (downgrade = credit exceeds charge).
	total := resp.JSONMap()["total"].(float64)
	if total >= 0 {
		t.Errorf("expected negative proration total for downgrade, got %v", total)
	}
}

func TestProrationBehaviorNone(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=NoPro", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=1000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	price1 := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=2000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	price2 := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=NoPro", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+price1, nil)
	resp.AssertStatus(200)
	subID := resp.JSONMap()["id"].(string)
	firstInvoice := resp.JSONMap()["latest_invoice"].(string)

	tc.Post("/admin/time/advance", map[string]any{"duration": "360h"})

	// Update with proration_behavior=none — no invoice should be created.
	resp = stripePost(tc, "/v1/subscriptions/"+subID+"?items[0][price]="+price2+"&proration_behavior=none", nil)
	resp.AssertStatus(200)

	// latest_invoice should be unchanged.
	if resp.JSONMap()["latest_invoice"] != firstInvoice {
		t.Errorf("expected latest_invoice unchanged with proration_behavior=none, got %v", resp.JSONMap()["latest_invoice"])
	}

	// Items should still be updated.
	subItems := resp.JSONMap()["items"].(map[string]any)["data"].([]any)
	price := subItems[0].(map[string]any)["price"].(map[string]any)
	if price["id"] != price2 {
		t.Errorf("expected price=%s, got %v", price2, price["id"])
	}
}

func TestQuantityChangeProration(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=QtyProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=2000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=QtyCust", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Start with quantity=1.
	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID+"&items[0][quantity]=1", nil)
	resp.AssertStatus(200)
	subID := resp.JSONMap()["id"].(string)
	firstInvoice := resp.JSONMap()["latest_invoice"].(string)

	tc.Post("/admin/time/advance", map[string]any{"duration": "360h"})

	// Increase to quantity=3.
	resp = stripePost(tc, "/v1/subscriptions/"+subID+"?items[0][price]="+priceID+"&items[0][quantity]=3", nil)
	resp.AssertStatus(200)

	newInvoice := resp.JSONMap()["latest_invoice"].(string)
	if newInvoice == firstInvoice {
		t.Fatal("expected a new proration invoice after quantity increase")
	}

	resp = stripeGet(tc, "/v1/invoices/"+newInvoice)
	resp.AssertStatus(200)
	// Net should be positive (more seats).
	if resp.JSONMap()["total"].(float64) <= 0 {
		t.Errorf("expected positive total for quantity increase, got %v", resp.JSONMap()["total"])
	}
}
