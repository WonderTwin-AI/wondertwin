package api_test

import (
	"testing"
)

func TestInvoiceDraftToFinalizeToPayTransitions(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a customer.
	resp := stripePost(tc, "/v1/customers?name=InvoiceTest", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create a draft invoice.
	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	inv := resp.JSONMap()
	invID := inv["id"].(string)
	if inv["status"] != "draft" {
		t.Fatalf("expected status=draft, got %v", inv["status"])
	}

	// Finalize it → open.
	resp = stripePost(tc, "/v1/invoices/"+invID+"/finalize", nil)
	resp.AssertStatus(200)
	inv = resp.JSONMap()
	if inv["status"] != "open" {
		t.Fatalf("expected status=open after finalize, got %v", inv["status"])
	}

	// Pay it → paid.
	resp = stripePost(tc, "/v1/invoices/"+invID+"/pay", nil)
	resp.AssertStatus(200)
	inv = resp.JSONMap()
	if inv["status"] != "paid" {
		t.Fatalf("expected status=paid after pay, got %v", inv["status"])
	}
	if inv["paid"] != true {
		t.Errorf("expected paid=true, got %v", inv["paid"])
	}
	if inv["amount_remaining"].(float64) != 0 {
		t.Errorf("expected amount_remaining=0, got %v", inv["amount_remaining"])
	}
}

func TestFinalizeNonDraftFails(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=Test", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create and finalize an invoice.
	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	invID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/invoices/"+invID+"/finalize", nil)
	resp.AssertStatus(200)

	// Try to finalize again — should fail since it's now open, not draft.
	resp = stripePost(tc, "/v1/invoices/"+invID+"/finalize", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("invoice_not_draft")
}

func TestPayNonOpenFails(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=Test", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create a draft invoice — paying it directly should fail.
	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	invID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/invoices/"+invID+"/pay", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("invoice_not_open")
}

func TestVoidPaidFails(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=Test", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create, finalize, pay.
	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	invID := resp.JSONMap()["id"].(string)

	stripePost(tc, "/v1/invoices/"+invID+"/finalize", nil).AssertStatus(200)
	stripePost(tc, "/v1/invoices/"+invID+"/pay", nil).AssertStatus(200)

	// Void should fail since it's paid.
	resp = stripePost(tc, "/v1/invoices/"+invID+"/void", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("invoice_not_voidable")
}

func TestVoidOpenInvoice(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=Test", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	invID := resp.JSONMap()["id"].(string)

	// Finalize then void.
	stripePost(tc, "/v1/invoices/"+invID+"/finalize", nil).AssertStatus(200)
	resp = stripePost(tc, "/v1/invoices/"+invID+"/void", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "void" {
		t.Errorf("expected status=void, got %v", resp.JSONMap()["status"])
	}
}

func TestVoidDraftInvoice(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=Test", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	invID := resp.JSONMap()["id"].(string)

	// Void a draft invoice — this should also work since the handler allows draft or open.
	resp = stripePost(tc, "/v1/invoices/"+invID+"/void", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "void" {
		t.Errorf("expected status=void, got %v", resp.JSONMap()["status"])
	}
}

func TestPendingInvoiceItemsAttachedOnCreate(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=Test", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create pending invoice items (no invoice specified).
	stripePost(tc, "/v1/invoiceitems?customer="+custID+"&amount=1000&currency=usd&description=Item+One", nil).AssertStatus(200)
	stripePost(tc, "/v1/invoiceitems?customer="+custID+"&amount=2500&currency=usd&description=Item+Two", nil).AssertStatus(200)

	// Create an invoice — pending items should be attached.
	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	inv := resp.JSONMap()

	if inv["total"].(float64) != 3500 {
		t.Errorf("expected total=3500, got %v", inv["total"])
	}
	if inv["subtotal"].(float64) != 3500 {
		t.Errorf("expected subtotal=3500, got %v", inv["subtotal"])
	}
	if inv["amount_due"].(float64) != 3500 {
		t.Errorf("expected amount_due=3500, got %v", inv["amount_due"])
	}

	// Check lines.
	lines, ok := inv["lines"].(map[string]any)
	if !ok {
		t.Fatal("expected lines object")
	}
	lineData, ok := lines["data"].([]any)
	if !ok || len(lineData) != 2 {
		t.Fatalf("expected 2 line items, got %v", len(lineData))
	}
}

func TestInvoiceNoItemsZeroTotal(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=Test", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create invoice with no pending items.
	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	inv := resp.JSONMap()
	if inv["total"].(float64) != 0 {
		t.Errorf("expected total=0 with no items, got %v", inv["total"])
	}
}

func TestSubscriptionCreationGeneratesInvoice(t *testing.T) {
	_, tc := setupStripe(t)

	// Create product and price.
	resp := stripePost(tc, "/v1/products?name=SubProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/prices?unit_amount=3000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	// Create customer.
	resp = stripePost(tc, "/v1/customers?name=SubCust", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create subscription.
	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID, nil)
	resp.AssertStatus(200)
	sub := resp.JSONMap()
	invoiceID, ok := sub["latest_invoice"].(string)
	if !ok || invoiceID == "" {
		t.Fatal("expected latest_invoice to be set on subscription")
	}

	// Verify invoice exists and is paid.
	resp = stripeGet(tc, "/v1/invoices/"+invoiceID)
	resp.AssertStatus(200)
	inv := resp.JSONMap()
	if inv["status"] != "paid" {
		t.Errorf("expected invoice status=paid, got %v", inv["status"])
	}
	if inv["subscription"] != sub["id"] {
		t.Errorf("expected invoice.subscription=%v, got %v", sub["id"], inv["subscription"])
	}
	if inv["total"].(float64) != 3000 {
		t.Errorf("expected invoice total=3000, got %v", inv["total"])
	}
	if inv["paid"] != true {
		t.Errorf("expected invoice paid=true, got %v", inv["paid"])
	}
}

func TestInvoicePaySetsAmountFields(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=Test", nil)
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

	if inv["amount_paid"].(float64) != 5000 {
		t.Errorf("expected amount_paid=5000, got %v", inv["amount_paid"])
	}
	if inv["amount_remaining"].(float64) != 0 {
		t.Errorf("expected amount_remaining=0, got %v", inv["amount_remaining"])
	}
	if inv["amount_due"].(float64) != 0 {
		t.Errorf("expected amount_due=0, got %v", inv["amount_due"])
	}
}

func TestInvoiceRequiresCustomer(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/invoices", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("parameter_missing")
}

func TestInvoiceListFilterByCustomer(t *testing.T) {
	_, tc := setupStripe(t)

	// Create two customers.
	resp := stripePost(tc, "/v1/customers?name=Cust1", nil)
	resp.AssertStatus(200)
	cust1 := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/customers?name=Cust2", nil)
	resp.AssertStatus(200)
	cust2 := resp.JSONMap()["id"].(string)

	// Create invoices for each.
	stripePost(tc, "/v1/invoices?customer="+cust1, nil).AssertStatus(200)
	stripePost(tc, "/v1/invoices?customer="+cust2, nil).AssertStatus(200)
	stripePost(tc, "/v1/invoices?customer="+cust1, nil).AssertStatus(200)

	// Filter by cust1.
	resp = stripeGet(tc, "/v1/invoices?customer="+cust1)
	resp.AssertStatus(200)
	data := resp.JSONMap()["data"].([]any)
	if len(data) != 2 {
		t.Errorf("expected 2 invoices for cust1, got %d", len(data))
	}
	for _, d := range data {
		inv := d.(map[string]any)
		if inv["customer"] != cust1 {
			t.Errorf("expected customer=%s, got %v", cust1, inv["customer"])
		}
	}
}
