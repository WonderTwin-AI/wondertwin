package api_test

import (
	"testing"
)

func TestFinalizeInvoiceSetsHostedURL(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=HostedURL", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	invID := resp.JSONMap()["id"].(string)

	// Before finalize, no hosted URL.
	if resp.JSONMap()["hosted_invoice_url"] != nil && resp.JSONMap()["hosted_invoice_url"] != "" {
		t.Error("expected no hosted_invoice_url on draft invoice")
	}

	resp = stripePost(tc, "/v1/invoices/"+invID+"/finalize", nil)
	resp.AssertStatus(200)

	url, ok := resp.JSONMap()["hosted_invoice_url"].(string)
	if !ok || url == "" {
		t.Fatal("expected hosted_invoice_url to be set after finalize")
	}
	if resp.JSONMap()["status"] != "open" {
		t.Errorf("expected status=open, got %v", resp.JSONMap()["status"])
	}
}

func TestPayInvoiceCreatesPaymentIntent(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=PayPI", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/invoiceitems?customer="+custID+"&amount=5000&currency=usd", nil).AssertStatus(200)

	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	invID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/invoices/"+invID+"/finalize", nil).AssertStatus(200)

	resp = stripePost(tc, "/v1/invoices/"+invID+"/pay", nil)
	resp.AssertStatus(200)
	inv := resp.JSONMap()

	if inv["status"] != "paid" {
		t.Errorf("expected status=paid, got %v", inv["status"])
	}

	// Should have a payment_intent.
	piID, ok := inv["payment_intent"].(string)
	if !ok || piID == "" {
		t.Fatal("expected payment_intent to be set after pay")
	}

	// Verify PI exists and is succeeded.
	resp = stripeGet(tc, "/v1/payment_intents/"+piID)
	resp.AssertStatus(200)
	pi := resp.JSONMap()
	if pi["status"] != "succeeded" {
		t.Errorf("expected PI status=succeeded, got %v", pi["status"])
	}
	if pi["amount"].(float64) != 5000 {
		t.Errorf("expected PI amount=5000, got %v", pi["amount"])
	}

	// Verify charge exists.
	chargeID := pi["latest_charge"].(string)
	resp = stripeGet(tc, "/v1/charges/"+chargeID)
	resp.AssertStatus(200)
	if resp.JSONMap()["amount"].(float64) != 5000 {
		t.Errorf("expected charge amount=5000, got %v", resp.JSONMap()["amount"])
	}
}

func TestPayInvoiceCreditsBalance(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=PayBal", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/invoiceitems?customer="+custID+"&amount=3000&currency=usd", nil).AssertStatus(200)

	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	invID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/invoices/"+invID+"/finalize", nil).AssertStatus(200)

	// Check balance before.
	resp = stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	before := getAvailableBalance(t, resp.JSONMap(), "usd")

	stripePost(tc, "/v1/invoices/"+invID+"/pay", nil).AssertStatus(200)

	// Check balance after.
	resp = stripeGet(tc, "/v1/balance")
	resp.AssertStatus(200)
	after := getAvailableBalance(t, resp.JSONMap(), "usd")

	if after-before != 3000 {
		t.Errorf("expected balance increase of 3000, got %v", after-before)
	}
}

func TestPayZeroInvoiceNoPICreated(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=ZeroInv", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// No items — zero total.
	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	invID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/invoices/"+invID+"/finalize", nil).AssertStatus(200)
	resp = stripePost(tc, "/v1/invoices/"+invID+"/pay", nil)
	resp.AssertStatus(200)

	// No payment_intent for zero-amount invoice.
	pi := resp.JSONMap()["payment_intent"]
	if pi != nil && pi != "" {
		t.Errorf("expected no payment_intent for zero invoice, got %v", pi)
	}
}

func TestSendInvoice(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=SendInv", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	invID := resp.JSONMap()["id"].(string)

	// Send a draft invoice — should finalize and send.
	resp = stripePost(tc, "/v1/invoices/"+invID+"/send", nil)
	resp.AssertStatus(200)
	inv := resp.JSONMap()

	if inv["status"] != "open" {
		t.Errorf("expected status=open after send, got %v", inv["status"])
	}
	url, ok := inv["hosted_invoice_url"].(string)
	if !ok || url == "" {
		t.Error("expected hosted_invoice_url to be set after send")
	}
}

func TestSendAlreadyOpenInvoice(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=SendOpen", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	invID := resp.JSONMap()["id"].(string)

	// Finalize first.
	stripePost(tc, "/v1/invoices/"+invID+"/finalize", nil).AssertStatus(200)

	// Send an already-open invoice — should succeed.
	resp = stripePost(tc, "/v1/invoices/"+invID+"/send", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "open" {
		t.Errorf("expected status=open, got %v", resp.JSONMap()["status"])
	}
}

// --- Payout Cancellation ---

func TestCancelPendingPayout(t *testing.T) {
	_, tc := setupStripe(t)

	// Create an account and fund it.
	resp := tc.DoWithHeaders("POST", "/v1/accounts", nil, map[string]string{
		"Authorization": "Bearer sk_test_sim_123",
	})
	resp.AssertStatus(200)
	acctID := resp.JSONMap()["id"].(string)

	tc.Post("/admin/accounts/"+acctID+"/fund", map[string]any{"amount": 10000, "currency": "usd"})

	// Create a payout.
	resp = tc.DoWithHeaders("POST", "/v1/payouts?amount=5000&currency=usd", nil, map[string]string{
		"Authorization":  "Bearer sk_test_sim_123",
		"Stripe-Account": acctID,
	})
	resp.AssertStatus(200)
	payoutID := resp.JSONMap()["id"].(string)
	if resp.JSONMap()["status"] != "pending" {
		t.Fatalf("expected status=pending, got %v", resp.JSONMap()["status"])
	}

	// Cancel it.
	resp = stripePost(tc, "/v1/payouts/"+payoutID+"/cancel", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "canceled" {
		t.Errorf("expected status=canceled, got %v", resp.JSONMap()["status"])
	}
}

func TestCancelNonPendingPayoutFails(t *testing.T) {
	_, tc := setupStripe(t)

	resp := tc.DoWithHeaders("POST", "/v1/accounts", nil, map[string]string{
		"Authorization": "Bearer sk_test_sim_123",
	})
	resp.AssertStatus(200)
	acctID := resp.JSONMap()["id"].(string)

	tc.Post("/admin/accounts/"+acctID+"/fund", map[string]any{"amount": 10000, "currency": "usd"})

	resp = tc.DoWithHeaders("POST", "/v1/payouts?amount=2000&currency=usd", nil, map[string]string{
		"Authorization":  "Bearer sk_test_sim_123",
		"Stripe-Account": acctID,
	})
	resp.AssertStatus(200)
	payoutID := resp.JSONMap()["id"].(string)

	// Advance time so payout moves to paid.
	tc.Post("/admin/time/advance", map[string]any{"duration": "5h"})

	// Force a GET to trigger state advance.
	stripeGet(tc, "/v1/payouts/"+payoutID)

	// Cancel should fail.
	resp = stripePost(tc, "/v1/payouts/"+payoutID+"/cancel", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("payout_not_cancelable")
}

// --- Product/Price Cascade ---

func TestDeleteProductDeactivatesPrices(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/products?name=CascProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=1000&currency=usd&product="+prodID, nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	// Price should be active.
	resp = stripeGet(tc, "/v1/prices/"+priceID)
	resp.AssertStatus(200)
	if resp.JSONMap()["active"] != true {
		t.Fatal("expected price active=true before product delete")
	}

	// Delete product.
	tc.DoWithHeaders("DELETE", "/v1/products/"+prodID, nil, map[string]string{
		"Authorization": "Bearer sk_test_sim_123",
	}).AssertStatus(200)

	// Price should now be inactive.
	resp = stripeGet(tc, "/v1/prices/"+priceID)
	resp.AssertStatus(200)
	if resp.JSONMap()["active"] != false {
		t.Errorf("expected price active=false after product delete, got %v", resp.JSONMap()["active"])
	}
}

func TestSendPaidInvoiceFails(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=SendPaid", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	invID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/invoices/"+invID+"/finalize", nil).AssertStatus(200)
	stripePost(tc, "/v1/invoices/"+invID+"/pay", nil).AssertStatus(200)

	// Sending a paid invoice should fail.
	resp = stripePost(tc, "/v1/invoices/"+invID+"/send", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("invoice_not_sendable")
}
