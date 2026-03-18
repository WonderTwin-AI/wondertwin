package api_test

import (
	"testing"
)

// --- Charge Update ---

func TestUpdateChargeDescription(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/charges?amount=1000&currency=usd&description=original", nil)
	resp.AssertStatus(200)
	id := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/charges/"+id+"?description=updated", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["description"] != "updated" {
		t.Errorf("expected description=updated, got %v", resp.JSONMap()["description"])
	}

	// Verify persistence.
	resp = stripeGet(tc, "/v1/charges/"+id)
	resp.AssertStatus(200)
	if resp.JSONMap()["description"] != "updated" {
		t.Error("description should persist on GET")
	}
}

func TestUpdateChargeMetadata(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/charges?amount=500&currency=usd", nil)
	resp.AssertStatus(200)
	id := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/charges/"+id+"?metadata[order_id]=ord_123", nil)
	resp.AssertStatus(200)
	meta, ok := resp.JSONMap()["metadata"].(map[string]any)
	if !ok || meta["order_id"] != "ord_123" {
		t.Errorf("expected metadata.order_id=ord_123, got %v", meta)
	}
}

func TestUpdateChargeNotFound(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/charges/ch_nonexistent?description=test", nil)
	resp.AssertStatus(404)
}

// --- Invoice Update ---

func TestUpdateDraftInvoice(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=UpdInv", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	invID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/invoices/"+invID+"?description=Monthly+charge&collection_method=send_invoice", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["description"] != "Monthly charge" {
		t.Errorf("expected description='Monthly charge', got %v", resp.JSONMap()["description"])
	}
	if resp.JSONMap()["collection_method"] != "send_invoice" {
		t.Errorf("expected collection_method=send_invoice, got %v", resp.JSONMap()["collection_method"])
	}
}

func TestUpdateFinalizedInvoiceFails(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=FinalizedUpd", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	invID := resp.JSONMap()["id"].(string)

	// Finalize it.
	stripePost(tc, "/v1/invoices/"+invID+"/finalize", nil).AssertStatus(200)

	// Update should fail — no longer draft.
	resp = stripePost(tc, "/v1/invoices/"+invID+"?description=nope", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("invoice_not_editable")
}

func TestUpdateInvoiceMetadata(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?name=MetaInv", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/invoices?customer="+custID, nil)
	resp.AssertStatus(200)
	invID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/invoices/"+invID+"?metadata[ref]=abc123", nil)
	resp.AssertStatus(200)
	meta, ok := resp.JSONMap()["metadata"].(map[string]any)
	if !ok || meta["ref"] != "abc123" {
		t.Errorf("expected metadata.ref=abc123, got %v", meta)
	}
}

// --- Payment Intent Update ---

func TestUpdatePaymentIntentAmount(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_intents?amount=1000&currency=usd", nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents/"+piID+"?amount=2000", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["amount"].(float64) != 2000 {
		t.Errorf("expected amount=2000, got %v", resp.JSONMap()["amount"])
	}
}

func TestUpdatePaymentIntentDescription(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_intents?amount=1000&currency=usd", nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents/"+piID+"?description=Order+42", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["description"] != "Order 42" {
		t.Errorf("expected description='Order 42', got %v", resp.JSONMap()["description"])
	}
}

func TestUpdatePaymentIntentPaymentMethod(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_intents?amount=1000&currency=usd", nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)
	if resp.JSONMap()["status"] != "requires_payment_method" {
		t.Fatalf("expected requires_payment_method, got %v", resp.JSONMap()["status"])
	}

	resp = stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	// Updating payment_method should transition to requires_confirmation.
	resp = stripePost(tc, "/v1/payment_intents/"+piID+"?payment_method="+pmID, nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["payment_method"] != pmID {
		t.Errorf("expected payment_method=%s, got %v", pmID, resp.JSONMap()["payment_method"])
	}
	if resp.JSONMap()["status"] != "requires_confirmation" {
		t.Errorf("expected status=requires_confirmation after setting PM, got %v", resp.JSONMap()["status"])
	}
}

func TestUpdateSucceededPaymentIntentFails(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_methods?type=card", nil)
	resp.AssertStatus(200)
	pmID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents?amount=1000&currency=usd&confirm=true&payment_method="+pmID, nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents/"+piID+"?description=nope", nil)
	resp.AssertStatus(400)
	resp.AssertBodyContains("payment_intent_unexpected_state")
}

func TestUpdatePaymentIntentMetadata(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_intents?amount=1000&currency=usd", nil)
	resp.AssertStatus(200)
	piID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/payment_intents/"+piID+"?metadata[session]=xyz", nil)
	resp.AssertStatus(200)
	meta, ok := resp.JSONMap()["metadata"].(map[string]any)
	if !ok || meta["session"] != "xyz" {
		t.Errorf("expected metadata.session=xyz, got %v", meta)
	}
}
