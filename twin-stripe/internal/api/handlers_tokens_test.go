package api_test

import (
	"testing"
)

func TestCreateAndGetToken(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePostWithParams(tc, "/v1/tokens", map[string]string{
		"card[number]":    "4242424242424242",
		"card[exp_month]": "12",
		"card[exp_year]":  "2030",
		"card[cvc]":       "123",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected token id")
	}
	if m["object"] != "token" {
		t.Errorf("expected object=token, got %v", m["object"])
	}
	if m["type"] != "card" {
		t.Errorf("expected type=card, got %v", m["type"])
	}
	if m["used"] != false {
		t.Errorf("expected used=false, got %v", m["used"])
	}

	card, ok := m["card"].(map[string]any)
	if !ok {
		t.Fatal("expected card details")
	}
	if card["last4"] != "4242" {
		t.Errorf("expected last4=4242, got %v", card["last4"])
	}
	if card["brand"] != "visa" {
		t.Errorf("expected brand=visa, got %v", card["brand"])
	}

	// GET token
	resp = stripeGet(tc, "/v1/tokens/"+id)
	resp.AssertStatus(200)
	m = resp.JSONMap()
	if m["id"] != id {
		t.Errorf("expected id=%s, got %v", id, m["id"])
	}
}

func TestTokenCardBrandDetection(t *testing.T) {
	_, tc := setupStripe(t)

	tests := []struct {
		number string
		brand  string
	}{
		{"4242424242424242", "visa"},
		{"5555555555554444", "mastercard"},
		{"378282246310005", "amex"},
		{"6011111111111117", "unknown"},
	}

	for _, tt := range tests {
		resp := stripePostWithParams(tc, "/v1/tokens", map[string]string{
			"card[number]":    tt.number,
			"card[exp_month]": "12",
			"card[exp_year]":  "2030",
		})
		resp.AssertStatus(200)

		m := resp.JSONMap()
		card, ok := m["card"].(map[string]any)
		if !ok {
			t.Fatalf("expected card details for number=%s", tt.number)
		}
		if card["brand"] != tt.brand {
			t.Errorf("number=%s: expected brand=%s, got %v", tt.number, tt.brand, card["brand"])
		}
	}
}

func TestCreateGetUpdateSource(t *testing.T) {
	_, tc := setupStripe(t)

	// Create source
	resp := stripePostWithParams(tc, "/v1/sources", map[string]string{
		"type":           "card",
		"currency":       "usd",
		"metadata[key1]": "value1",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected source id")
	}
	if m["object"] != "source" {
		t.Errorf("expected object=source, got %v", m["object"])
	}
	if m["type"] != "card" {
		t.Errorf("expected type=card, got %v", m["type"])
	}
	if m["status"] != "chargeable" {
		t.Errorf("expected status=chargeable, got %v", m["status"])
	}
	if m["flow"] != "none" {
		t.Errorf("expected flow=none, got %v", m["flow"])
	}

	// GET source
	resp = stripeGet(tc, "/v1/sources/"+id)
	resp.AssertStatus(200)
	m = resp.JSONMap()
	if m["id"] != id {
		t.Errorf("expected id=%s, got %v", id, m["id"])
	}

	// Update source metadata
	resp = stripePostWithParams(tc, "/v1/sources/"+id, map[string]string{
		"metadata[key2]": "value2",
	})
	resp.AssertStatus(200)
	m = resp.JSONMap()
	md, ok := m["metadata"].(map[string]any)
	if !ok {
		t.Fatal("expected metadata")
	}
	if md["key2"] != "value2" {
		t.Errorf("expected metadata key2=value2, got %v", md["key2"])
	}
}

func TestSourceRequiresType(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePostWithParams(tc, "/v1/sources", map[string]string{
		"currency": "usd",
	})
	resp.AssertStatus(400)
	resp.AssertBodyContains("parameter_missing")
}

func TestGetMandate404(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripeGet(tc, "/v1/mandates/mandate_nonexistent")
	resp.AssertStatus(404)
	resp.AssertBodyContains("resource_missing")
}

func TestGetConfirmationToken404(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripeGet(tc, "/v1/confirmation_tokens/ctoken_nonexistent")
	resp.AssertStatus(404)
	resp.AssertBodyContains("resource_missing")
}
