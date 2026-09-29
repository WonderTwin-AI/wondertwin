package api_test

import (
	"net/url"
	"testing"
)

// --- Documented test payment method tests ---

func TestDecline_TestPaymentMethodOnConfirm(t *testing.T) {
	_, tc := setupStripe(t)
	pi := stripeForm(t, tc, "POST", "/v1/payment_intents",
		url.Values{"amount": {"1099"}, "currency": {"usd"}, "payment_method_types[0]": {"card"}}, nil).assertStatus(200).json()
	id := pi["id"].(string)

	e := errorOf(t, stripeForm(t, tc, "POST", "/v1/payment_intents/"+id+"/confirm",
		url.Values{"payment_method": {"pm_card_visa_chargeDeclined"}}, nil).assertStatus(402))
	if e["type"] != "card_error" || e["code"] != "card_declined" || e["decline_code"] != "generic_decline" {
		t.Fatalf("expected card_error card_declined generic_decline, got %v", e)
	}
	if e["charge"] == nil || e["payment_intent"] == nil {
		t.Fatalf("expected the error to carry the charge and the intent, got %v", e)
	}

	got := stripeForm(t, tc, "GET", "/v1/payment_intents/"+id, nil, nil).assertStatus(200).json()
	lpe, _ := got["last_payment_error"].(map[string]any)
	if got["status"] != "requires_payment_method" || lpe["code"] != "card_declined" {
		t.Fatalf("expected requires_payment_method with last_payment_error, got %v", got)
	}
	ch := stripeForm(t, tc, "GET", "/v1/charges/"+e["charge"].(string), nil, nil).assertStatus(200).json()
	if ch["status"] != "failed" || ch["failure_code"] != "card_declined" {
		t.Fatalf("expected a failed charge, got %v", ch)
	}
}

func TestDecline_CodesPerTestMethod(t *testing.T) {
	_, tc := setupStripe(t)
	cases := map[string][2]string{
		"pm_card_visa_chargeDeclinedInsufficientFunds": {"card_declined", "insufficient_funds"},
		"pm_card_chargeDeclinedExpiredCard":            {"expired_card", ""},
		"pm_card_chargeDeclinedIncorrectCvc":           {"incorrect_cvc", ""},
		"pm_card_chargeDeclinedProcessingError":        {"processing_error", ""},
	}
	for pm, want := range cases {
		e := errorOf(t, stripeForm(t, tc, "POST", "/v1/payment_intents",
			url.Values{"amount": {"500"}, "currency": {"usd"}, "confirm": {"true"}, "payment_method": {pm}}, nil).assertStatus(402))
		dc, _ := e["decline_code"].(string)
		if e["code"] != want[0] || dc != want[1] {
			t.Errorf("%s: expected code=%s decline_code=%q, got %v", pm, want[0], want[1], e)
		}
	}
}

func TestDecline_VisaTestMethodSucceedsAndAttaches(t *testing.T) {
	_, tc := setupStripe(t)
	pi := stripeForm(t, tc, "POST", "/v1/payment_intents",
		url.Values{"amount": {"2000"}, "currency": {"usd"}, "confirm": {"true"}, "payment_method": {"pm_card_visa"}}, nil).assertStatus(200).json()
	if pi["status"] != "succeeded" {
		t.Fatalf("expected succeeded, got %v", pi["status"])
	}
	if pmID, _ := pi["payment_method"].(string); pmID == "pm_card_visa" || pmID == "" {
		t.Fatalf("expected a new PaymentMethod ID in place of the test ID, got %q", pmID)
	}

	cus := stripeForm(t, tc, "POST", "/v1/customers", url.Values{"email": {"a@example.com"}}, nil).assertStatus(200).json()
	pm := stripeForm(t, tc, "POST", "/v1/payment_methods/pm_card_visa/attach",
		url.Values{"customer": {cus["id"].(string)}}, nil).assertStatus(200).json()
	if pm["customer"] != cus["id"] {
		t.Fatalf("expected the new PaymentMethod attached to the customer, got %v", pm)
	}
	e := errorOf(t, stripeForm(t, tc, "POST", "/v1/payment_methods/pm_card_visa_chargeDeclined/attach",
		url.Values{"customer": {cus["id"].(string)}}, nil).assertStatus(402))
	if e["code"] != "card_declined" {
		t.Fatalf("expected an issuer-decline card to be refused on attach, got %v", e)
	}
}
