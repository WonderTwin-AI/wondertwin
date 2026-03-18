package api_test

import (
	"testing"
)

func TestCreateAndGetCreditNote(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a credit note (invoice and amount are required but we use query-param approach)
	resp := stripePost(tc, "/v1/credit_notes?invoice=in_test123&amount=500&currency=usd", nil)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected credit note id")
	}
	if m["object"] != "credit_note" {
		t.Errorf("expected object=credit_note, got %v", m["object"])
	}
	if m["status"] != "issued" {
		t.Errorf("expected status=issued, got %v", m["status"])
	}
	if m["invoice"] != "in_test123" {
		t.Errorf("expected invoice=in_test123, got %v", m["invoice"])
	}
	if m["amount"].(float64) != 500 {
		t.Errorf("expected amount=500, got %v", m["amount"])
	}
	if m["type"] != "post_payment" {
		t.Errorf("expected type=post_payment, got %v", m["type"])
	}
	number, _ := m["number"].(string)
	if number == "" {
		t.Error("expected a credit note number")
	}

	// Get it back
	resp = stripeGet(tc, "/v1/credit_notes/"+id)
	resp.AssertStatus(200)
	if resp.JSONMap()["id"] != id {
		t.Errorf("expected id=%s, got %v", id, resp.JSONMap()["id"])
	}
}

func TestVoidCreditNote(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/credit_notes?invoice=in_test123&amount=500", nil)
	resp.AssertStatus(200)
	id := resp.JSONMap()["id"].(string)

	// Void it
	resp = stripePost(tc, "/v1/credit_notes/"+id+"/void", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "void" {
		t.Errorf("expected status=void, got %v", resp.JSONMap()["status"])
	}

	// Voiding again should fail
	resp = stripePost(tc, "/v1/credit_notes/"+id+"/void", nil)
	resp.AssertStatus(400)
}

func TestPreviewCreditNote(t *testing.T) {
	_, tc := setupStripe(t)

	// Preview should not persist
	resp := stripeGet(tc, "/v1/credit_notes/preview?invoice=in_test123&amount=1000&currency=usd")
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["object"] != "credit_note" {
		t.Errorf("expected object=credit_note, got %v", m["object"])
	}
	// Should NOT have an ID (not persisted)
	if m["id"] != nil && m["id"] != "" {
		t.Errorf("expected no id for preview, got %v", m["id"])
	}

	// List should be empty (nothing persisted)
	resp = stripeGet(tc, "/v1/credit_notes")
	resp.AssertStatus(200)
	data := resp.JSONMap()["data"].([]any)
	if len(data) != 0 {
		t.Errorf("expected 0 credit notes after preview, got %d", len(data))
	}
}

func TestCreateAndGetPromotionCode(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a coupon first
	resp := stripePost(tc, "/v1/coupons?duration=once&percent_off=25", nil)
	resp.AssertStatus(200)
	couponID := resp.JSONMap()["id"].(string)

	// Create promotion code referencing the coupon via query param
	resp = stripePost(tc, "/v1/promotion_codes?coupon="+couponID, nil)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected promotion code id")
	}
	if m["object"] != "promotion_code" {
		t.Errorf("expected object=promotion_code, got %v", m["object"])
	}
	if m["active"] != true {
		t.Errorf("expected active=true, got %v", m["active"])
	}
	code, _ := m["code"].(string)
	if code == "" {
		t.Error("expected an auto-generated code")
	}
	if m["coupon"] != couponID {
		t.Errorf("expected coupon=%s, got %v", couponID, m["coupon"])
	}

	// Get it back
	resp = stripeGet(tc, "/v1/promotion_codes/"+id)
	resp.AssertStatus(200)
	if resp.JSONMap()["id"] != id {
		t.Errorf("expected id=%s, got %v", id, resp.JSONMap()["id"])
	}
}

func TestCreateAndGetSubscriptionItem(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a product
	resp := stripePost(tc, "/v1/products?name=TestProd", nil)
	resp.AssertStatus(200)
	prodID := resp.JSONMap()["id"].(string)

	// Create a recurring price
	resp = stripePost(tc, "/v1/prices?unit_amount=2000&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	priceID := resp.JSONMap()["id"].(string)

	// Create a customer
	resp = stripePost(tc, "/v1/customers?name=TestCustomer", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create a subscription with the price
	resp = stripePost(tc, "/v1/subscriptions?customer="+custID+"&items[0][price]="+priceID, nil)
	resp.AssertStatus(200)
	subID := resp.JSONMap()["id"].(string)

	// Now create a standalone subscription item via SubItems store
	// We need another price for a second item
	resp = stripePost(tc, "/v1/prices?unit_amount=500&currency=usd&product="+prodID+"&recurring[interval]=month", nil)
	resp.AssertStatus(200)
	price2ID := resp.JSONMap()["id"].(string)

	resp = stripePost(tc, "/v1/subscription_items?subscription="+subID+"&price="+price2ID, nil)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	siID, ok := m["id"].(string)
	if !ok || siID == "" {
		t.Fatal("expected subscription item id")
	}
	if m["object"] != "subscription_item" {
		t.Errorf("expected object=subscription_item, got %v", m["object"])
	}
	if m["quantity"].(float64) != 1 {
		t.Errorf("expected quantity=1, got %v", m["quantity"])
	}

	// Get it back
	resp = stripeGet(tc, "/v1/subscription_items/"+siID)
	resp.AssertStatus(200)
	if resp.JSONMap()["id"] != siID {
		t.Errorf("expected id=%s, got %v", siID, resp.JSONMap()["id"])
	}
}

func TestQuoteLifecycle(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a quote
	resp := stripePost(tc, "/v1/quotes?customer=cus_test123&currency=usd", nil)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected quote id")
	}
	if m["object"] != "quote" {
		t.Errorf("expected object=quote, got %v", m["object"])
	}
	if m["status"] != "draft" {
		t.Errorf("expected status=draft, got %v", m["status"])
	}

	// Finalize it
	resp = stripePost(tc, "/v1/quotes/"+id+"/finalize", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "open" {
		t.Errorf("expected status=open after finalize, got %v", resp.JSONMap()["status"])
	}
	if resp.JSONMap()["expires_at"] == nil || resp.JSONMap()["expires_at"].(float64) == 0 {
		t.Error("expected expires_at to be set after finalize")
	}

	// Accept it
	resp = stripePost(tc, "/v1/quotes/"+id+"/accept", nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "accepted" {
		t.Errorf("expected status=accepted, got %v", resp.JSONMap()["status"])
	}

	// Cannot accept again
	resp = stripePost(tc, "/v1/quotes/"+id+"/accept", nil)
	resp.AssertStatus(400)
}

func TestCreateBillingPortalSession(t *testing.T) {
	_, tc := setupStripe(t)

	// Create a customer first
	resp := stripePost(tc, "/v1/customers?name=PortalUser", nil)
	resp.AssertStatus(200)
	custID := resp.JSONMap()["id"].(string)

	// Create billing portal session
	resp = stripePost(tc, "/v1/billing_portal/sessions?customer="+custID+"&return_url=https://example.com/return", nil)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected billing portal session id")
	}
	if m["object"] != "billing_portal.session" {
		t.Errorf("expected object=billing_portal.session, got %v", m["object"])
	}
	if m["customer"] != custID {
		t.Errorf("expected customer=%s, got %v", custID, m["customer"])
	}
	url, _ := m["url"].(string)
	if url == "" {
		t.Error("expected a billing portal URL")
	}
}
