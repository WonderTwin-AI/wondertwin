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
