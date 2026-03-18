package api_test

import (
	"testing"
)

func TestPercentOffCouponOnInvoice(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a 25% off coupon.
	resp := stripePost(tc, "/v1/coupons?duration=once&percent_off=25", nil)
	resp.AssertStatus(200)
	couponID := resp.JSONMap()["id"].(string)

	// Create customer and invoice item.
	resp = stripePost(tc, "/v1/customers?name=DiscountCust", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/invoiceitems?customer="+custID+"&amount=10000&currency=usd", nil).AssertStatus(200)

	// Create invoice with coupon discount.
	resp = stripePost(tc, "/v1/invoices?customer="+custID+"&discounts[0][coupon]="+couponID, nil)
	resp.AssertStatus(200)
	inv := resp.JSONMap()

	// 10000 - 25% = 7500
	if inv["subtotal"].(float64) != 10000 {
		t.Errorf("expected subtotal=10000, got %v", inv["subtotal"])
	}
	if inv["total"].(float64) != 7500 {
		t.Errorf("expected total=7500, got %v", inv["total"])
	}
	if inv["amount_due"].(float64) != 7500 {
		t.Errorf("expected amount_due=7500, got %v", inv["amount_due"])
	}

	// Verify discount is attached.
	disc, ok := inv["discount"].(map[string]any)
	if !ok {
		t.Fatal("expected discount object on invoice")
	}
	coup, ok := disc["coupon"].(map[string]any)
	if !ok {
		t.Fatal("expected coupon in discount")
	}
	if coup["percent_off"].(float64) != 25 {
		t.Errorf("expected coupon percent_off=25, got %v", coup["percent_off"])
	}
}

func TestAmountOffCouponOnInvoice(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a $20 off coupon.
	resp := stripePost(tc, "/v1/coupons?duration=once&amount_off=2000&currency=usd", nil)
	resp.AssertStatus(200)
	couponID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=AmtOff", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/invoiceitems?customer="+custID+"&amount=5000&currency=usd", nil).AssertStatus(200)

	resp = stripePost(tc, "/v1/invoices?customer="+custID+"&discounts[0][coupon]="+couponID, nil)
	resp.AssertStatus(200)
	inv := resp.JSONMap()

	// 5000 - 2000 = 3000
	if inv["total"].(float64) != 3000 {
		t.Errorf("expected total=3000, got %v", inv["total"])
	}
}

func TestSubscriptionWithCoupon(t *testing.T) {
	_, tc := setupStripe(t)

	// Create 50% off coupon.
	resp := stripePost(tc, "/v1/coupons?duration=once&percent_off=50", nil)
	resp.AssertStatus(200)
	couponID := resp.JSONMap()["id"].(string)

	// Create product + price.
	resp = stripePost(tc, "/v1/products?name=DiscountProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=4000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=SubDiscount", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create subscription with coupon.
	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID+"&coupon="+couponID, nil)
	resp.AssertStatus(200)
	sub := resp.JSONMap()

	// Verify discount is on subscription.
	disc, ok := sub["discount"].(map[string]any)
	if !ok {
		t.Fatal("expected discount on subscription")
	}
	if disc["subscription"] != sub["id"] {
		t.Errorf("expected discount.subscription=%v, got %v", sub["id"], disc["subscription"])
	}

	// Verify invoice total is discounted: 4000 - 50% = 2000.
	invoiceID := sub["latest_invoice"].(string)
	resp = stripeGet(tc, "/v1/invoices/"+invoiceID)
	resp.AssertStatus(200)
	inv := resp.JSONMap()
	if inv["subtotal"].(float64) != 4000 {
		t.Errorf("expected subtotal=4000, got %v", inv["subtotal"])
	}
	if inv["total"].(float64) != 2000 {
		t.Errorf("expected total=2000 after 50%% discount, got %v", inv["total"])
	}
}

func TestCouponTimesRedeemedIncremented(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/coupons?duration=once&percent_off=10", nil)
	resp.AssertStatus(200)
	couponID := resp.JSONMap()["id"].(string)

	// Check initial times_redeemed.
	resp = stripeGet(tc, "/v1/coupons/"+couponID)
	resp.AssertStatus(200)
	if resp.JSONMap()["times_redeemed"].(float64) != 0 {
		t.Errorf("expected initial times_redeemed=0, got %v", resp.JSONMap()["times_redeemed"])
	}

	// Apply it to an invoice.
	resp = stripePost(tc, "/v1/customers?name=Redeem", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/invoiceitems?customer="+custID+"&amount=1000&currency=usd", nil).AssertStatus(200)
	stripePost(tc, "/v1/invoices?customer="+custID+"&discounts[0][coupon]="+couponID, nil).AssertStatus(200)

	// Check times_redeemed incremented.
	resp = stripeGet(tc, "/v1/coupons/"+couponID)
	resp.AssertStatus(200)
	if resp.JSONMap()["times_redeemed"].(float64) != 1 {
		t.Errorf("expected times_redeemed=1, got %v", resp.JSONMap()["times_redeemed"])
	}
}

func TestExclusiveTaxRateOnInvoice(t *testing.T) {
	_, tc := setupStripe(t)

	// Create an exclusive 10% tax rate.
	resp := stripePost(tc, "/v1/tax_rates?display_name=VAT&percentage=10&inclusive=false", nil)
	resp.AssertStatus(200)
	taxRateID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=TaxCust", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/invoiceitems?customer="+custID+"&amount=10000&currency=usd", nil).AssertStatus(200)

	// Create invoice with exclusive tax rate.
	resp = stripePost(tc, "/v1/invoices?customer="+custID+"&default_tax_rates[0]="+taxRateID, nil)
	resp.AssertStatus(200)
	inv := resp.JSONMap()

	// 10000 + 10% exclusive = 11000
	if inv["subtotal"].(float64) != 10000 {
		t.Errorf("expected subtotal=10000, got %v", inv["subtotal"])
	}
	if inv["total"].(float64) != 11000 {
		t.Errorf("expected total=11000 (with 10%% exclusive tax), got %v", inv["total"])
	}

	// Verify tax amounts.
	taxAmounts, ok := inv["total_tax_amounts"].([]any)
	if !ok || len(taxAmounts) != 1 {
		t.Fatalf("expected 1 tax amount, got %v", taxAmounts)
	}
	ta := taxAmounts[0].(map[string]any)
	if ta["amount"].(float64) != 1000 {
		t.Errorf("expected tax amount=1000, got %v", ta["amount"])
	}
	if ta["inclusive"] != false {
		t.Errorf("expected inclusive=false, got %v", ta["inclusive"])
	}
}

func TestInclusiveTaxRateOnInvoice(t *testing.T) {
	_, tc := setupStripe(t)

	// Create an inclusive 20% tax rate.
	resp := stripePost(tc, "/v1/tax_rates?display_name=GST&percentage=20&inclusive=true", nil)
	resp.AssertStatus(200)
	taxRateID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=InclTax", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/invoiceitems?customer="+custID+"&amount=12000&currency=usd", nil).AssertStatus(200)

	resp = stripePost(tc, "/v1/invoices?customer="+custID+"&default_tax_rates[0]="+taxRateID, nil)
	resp.AssertStatus(200)
	inv := resp.JSONMap()

	// Inclusive tax: total should stay the same as subtotal.
	if inv["total"].(float64) != 12000 {
		t.Errorf("expected total=12000 (inclusive tax doesn't add to total), got %v", inv["total"])
	}

	// But tax amount should still be computed.
	taxAmounts, ok := inv["total_tax_amounts"].([]any)
	if !ok || len(taxAmounts) != 1 {
		t.Fatalf("expected 1 tax amount, got %v", taxAmounts)
	}
	ta := taxAmounts[0].(map[string]any)
	if ta["amount"].(float64) != 2400 {
		t.Errorf("expected tax amount=2400 (20%% of 12000), got %v", ta["amount"])
	}
	if ta["inclusive"] != true {
		t.Errorf("expected inclusive=true, got %v", ta["inclusive"])
	}
}

func TestDiscountAndTaxTogether(t *testing.T) {
	_, tc := setupStripe(t)

	// 20% off coupon.
	resp := stripePost(tc, "/v1/coupons?duration=once&percent_off=20", nil)
	resp.AssertStatus(200)
	couponID := resp.JSONMap()["id"].(string)

	// 10% exclusive tax.
	resp = stripePost(tc, "/v1/tax_rates?display_name=Tax&percentage=10&inclusive=false", nil)
	resp.AssertStatus(200)
	taxRateID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=BothCust", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/invoiceitems?customer="+custID+"&amount=10000&currency=usd", nil).AssertStatus(200)

	resp = stripePost(tc, "/v1/invoices?customer="+custID+"&discounts[0][coupon]="+couponID+"&default_tax_rates[0]="+taxRateID, nil)
	resp.AssertStatus(200)
	inv := resp.JSONMap()

	// subtotal=10000, discount=2000 (20%), taxable=8000, tax=800 (10%), total=8800
	if inv["subtotal"].(float64) != 10000 {
		t.Errorf("expected subtotal=10000, got %v", inv["subtotal"])
	}
	if inv["total"].(float64) != 8800 {
		t.Errorf("expected total=8800 (10000 - 2000 + 800), got %v", inv["total"])
	}
}

func TestInvalidCouponRejected(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=BadCoup", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/invoiceitems?customer="+custID+"&amount=1000&currency=usd", nil).AssertStatus(200)

	resp = stripePost(tc, "/v1/invoices?customer="+custID+"&discounts[0][coupon]=coup_nonexistent", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("resource_missing")
}

func TestSubscriptionWithTaxRate(t *testing.T) {
	_, tc := setupStripe(t)

	// Create 8% exclusive tax.
	resp := stripePost(tc, "/v1/tax_rates?display_name=Sales+Tax&percentage=8&inclusive=false", nil)
	resp.AssertStatus(200)
	taxRateID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/products?name=TaxProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=5000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=TaxSub", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID+"&default_tax_rates[0]="+taxRateID, nil)
	resp.AssertStatus(200)
	sub := resp.JSONMap()

	// Verify invoice total includes tax: 5000 + 8% = 5400.
	invoiceID := sub["latest_invoice"].(string)
	resp = stripeGet(tc, "/v1/invoices/"+invoiceID)
	resp.AssertStatus(200)
	inv := resp.JSONMap()
	if inv["total"].(float64) != 5400 {
		t.Errorf("expected total=5400, got %v", inv["total"])
	}
}
