package api_test

import (
	"testing"
)

func TestCreateAndGetCheckoutSession(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePostWithParams(tc, "/v1/checkout/sessions", map[string]string{
		"success_url": "https://example.com/success",
		"cancel_url":  "https://example.com/cancel",
		"mode":        "payment",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected checkout session id")
	}
	if m["object"] != "checkout.session" {
		t.Errorf("expected object=checkout.session, got %v", m["object"])
	}
	if m["status"] != "open" {
		t.Errorf("expected status=open, got %v", m["status"])
	}
	if m["mode"] != "payment" {
		t.Errorf("expected mode=payment, got %v", m["mode"])
	}
	if m["payment_status"] != "unpaid" {
		t.Errorf("expected payment_status=unpaid, got %v", m["payment_status"])
	}
	if m["success_url"] != "https://example.com/success" {
		t.Errorf("expected success_url, got %v", m["success_url"])
	}
	u, _ := m["url"].(string)
	if u == "" {
		t.Error("expected a checkout URL")
	}
	if m["expires_at"] == nil || m["expires_at"].(float64) == 0 {
		t.Error("expected expires_at to be set")
	}

	// Get it back
	resp = stripeGet(tc, "/v1/checkout/sessions/"+id)
	resp.AssertStatus(200)
	m = resp.JSONMap()
	if m["id"] != id {
		t.Errorf("expected id=%s, got %v", id, m["id"])
	}
}

func TestListCheckoutSessions(t *testing.T) {
	_, tc := setupStripe(t)

	stripePostWithParams(tc, "/v1/checkout/sessions", map[string]string{
		"mode": "payment",
	}).AssertStatus(200)
	stripePostWithParams(tc, "/v1/checkout/sessions", map[string]string{
		"mode": "setup",
	}).AssertStatus(200)

	resp := stripeGet(tc, "/v1/checkout/sessions")
	resp.AssertStatus(200)

	m := resp.JSONMap()
	data, ok := m["data"].([]any)
	if !ok || len(data) < 2 {
		t.Fatalf("expected at least 2 checkout sessions, got %v", m["data"])
	}
}

func TestExpireCheckoutSession(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePostWithParams(tc, "/v1/checkout/sessions", map[string]string{
		"mode": "payment",
	})
	resp.AssertStatus(200)
	id := resp.JSONMap()["id"].(string)

	// Expire it
	resp = stripePost(tc, "/v1/checkout/sessions/"+id+"/expire", nil)
	resp.AssertStatus(200)

	m := resp.JSONMap()
	if m["status"] != "expired" {
		t.Errorf("expected status=expired, got %v", m["status"])
	}

	// Verify it persisted
	resp = stripeGet(tc, "/v1/checkout/sessions/"+id)
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "expired" {
		t.Error("expected status to remain expired after get")
	}

	// Expiring again should fail
	resp = stripePost(tc, "/v1/checkout/sessions/"+id+"/expire", nil)
	resp.AssertStatus(400)
}

func TestCreateAndGetPaymentLink(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePostWithParams(tc, "/v1/payment_links", map[string]string{
		"currency": "usd",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected payment link id")
	}
	if m["object"] != "payment_link" {
		t.Errorf("expected object=payment_link, got %v", m["object"])
	}
	if m["active"] != true {
		t.Errorf("expected active=true, got %v", m["active"])
	}
	u, _ := m["url"].(string)
	if u == "" {
		t.Error("expected a payment link URL")
	}

	// Get it back
	resp = stripeGet(tc, "/v1/payment_links/"+id)
	resp.AssertStatus(200)
	m = resp.JSONMap()
	if m["id"] != id {
		t.Errorf("expected id=%s, got %v", id, m["id"])
	}
}

func TestListPaymentLinks(t *testing.T) {
	_, tc := setupStripe(t)

	stripePostWithParams(tc, "/v1/payment_links", map[string]string{
		"currency": "usd",
	}).AssertStatus(200)
	stripePostWithParams(tc, "/v1/payment_links", map[string]string{
		"currency": "eur",
	}).AssertStatus(200)

	resp := stripeGet(tc, "/v1/payment_links")
	resp.AssertStatus(200)

	m := resp.JSONMap()
	data, ok := m["data"].([]any)
	if !ok || len(data) < 2 {
		t.Fatalf("expected at least 2 payment links, got %v", m["data"])
	}
}

func TestUpdatePaymentLink(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePostWithParams(tc, "/v1/payment_links", map[string]string{
		"currency": "usd",
	})
	resp.AssertStatus(200)
	id := resp.JSONMap()["id"].(string)

	// Deactivate it
	resp = stripePostWithParams(tc, "/v1/payment_links/"+id, map[string]string{
		"active": "false",
	})
	resp.AssertStatus(200)
	if resp.JSONMap()["active"] != false {
		t.Errorf("expected active=false after update, got %v", resp.JSONMap()["active"])
	}

	// Verify persistence
	resp = stripeGet(tc, "/v1/payment_links/"+id)
	resp.AssertStatus(200)
	if resp.JSONMap()["active"] != false {
		t.Error("expected active to remain false after get")
	}
}
