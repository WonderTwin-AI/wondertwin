package api_test

import (
	"reflect"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

func autoCheckout(tc *testutil.TwinClient, cus, price, extra string) *testutil.Response {
	path := "/v1/checkout/sessions?mode=subscription&success_url=https://example.com/ok&line_items[0][price]=" + price + "&line_items[0][quantity]=1"
	if cus != "" {
		path += "&customer=" + cus
	}
	return stripePost(tc, path+"&"+extra, nil)
}

// The tax-off refusal as Stripe answered Checkout on 2026-11-15 (quirk
// error-006, capture t18): Checkout's message has two spaces before "Visit".
func TestCheckoutAutomaticTaxRefusedWhenTaxIsNotSetUp(t *testing.T) {
	_, tc := setupStripe(t)
	resp := autoCheckout(tc, taxCustomer(tc, "CH"), yearlyPrice(tc, "exclusive"), autoTax)
	resp.AssertStatus(400)
	want := map[string]any{"error": map[string]any{
		"type":    "invalid_request_error",
		"message": "You must have a valid head office address to enable automatic tax calculation in test mode.  Visit https://dashboard.stripe.com/test/settings/tax to update it.",
	}}
	if got := resp.JSONMap(); !reflect.DeepEqual(got, want) {
		t.Errorf("create: %v, want %v", got, want)
	}
}

// Quirk crud-016 and capture t46: amount_total includes the tax,
// total_details.amount_tax is the tax, and the expanded breakdown embeds the
// full tax_rate object.
func TestCheckoutAutomaticTaxTotalsAndBreakdown(t *testing.T) {
	_, tc := setupStripe(t)
	seedTax(t, tc)
	resp := autoCheckout(tc, taxCustomer(tc, "CH"), yearlyPrice(tc, "exclusive"), autoTax+"&expand[]=total_details.breakdown")
	resp.AssertStatus(200)
	cs := resp.JSONMap()
	if num(cs, "amount_subtotal") != 46800 || num(cs, "amount_total") != 50591 {
		t.Errorf("amount_subtotal %v amount_total %v, want 46800 and 50591", cs["amount_subtotal"], cs["amount_total"])
	}
	wantAT := map[string]any{"address_collection_precision": nil, "enabled": true, "liability": map[string]any{"type": "self"}, "provider": "stripe", "status": "complete"}
	if !reflect.DeepEqual(cs["automatic_tax"], wantAT) {
		t.Errorf("automatic_tax = %v, want %v", cs["automatic_tax"], wantAT)
	}
	td := cs["total_details"].(map[string]any)
	if num(td, "amount_tax") != 3791 || num(td, "amount_discount") != 0 || num(td, "amount_shipping") != 0 {
		t.Errorf("total_details = %v", td)
	}
	taxes := td["breakdown"].(map[string]any)["taxes"].([]any)
	if len(taxes) != 1 {
		t.Fatalf("breakdown taxes = %v", taxes)
	}
	tax := taxes[0].(map[string]any)
	if num(tax, "amount") != 3791 || num(tax, "taxable_amount") != 46800 || tax["taxability_reason"] != "standard_rated" {
		t.Errorf("breakdown tax = %v", tax)
	}
	rate := tax["rate"].(map[string]any)
	for k, v := range map[string]any{"object": "tax_rate", "country": "CH", "jurisdiction": "Switzerland", "percentage": 8.1, "active": false, "inclusive": false} {
		if rate[k] != v {
			t.Errorf("breakdown rate %s = %v, want %v", k, rate[k], v)
		}
	}

	// Without the expand the breakdown carries no taxes (the response schema
	// fills an unexpanded breakdown with empty lists).
	plain := autoCheckout(tc, taxCustomer(tc, "CH"), yearlyPrice(tc, "exclusive"), autoTax).JSONMap()
	if b, _ := plain["total_details"].(map[string]any)["breakdown"].(map[string]any); b != nil && len(b["taxes"].([]any)) != 0 {
		t.Errorf("total_details.breakdown taxes rendered without expand: %v", b)
	}
	if _, ok := plain["tax_breakdown"]; ok {
		t.Error("internal tax_breakdown rendered")
	}
}

// An inclusive price keeps amount_total; the tax inside it is still amount_tax.
func TestCheckoutAutomaticTaxInclusivePrice(t *testing.T) {
	_, tc := setupStripe(t)
	seedTax(t, tc)
	cs := autoCheckout(tc, taxCustomer(tc, "CH"), yearlyPrice(tc, "inclusive"), autoTax).JSONMap()
	if num(cs, "amount_total") != 46800 || num(cs["total_details"].(map[string]any), "amount_tax") != 3507 {
		t.Errorf("amount_total %v total_details %v, want 46800 with 3507 tax", cs["amount_total"], cs["total_details"])
	}
}

// With no customer location Checkout waits for the address it collects.
func TestCheckoutAutomaticTaxWithoutLocation(t *testing.T) {
	_, tc := setupStripe(t)
	seedTax(t, tc)
	price := yearlyPrice(tc, "exclusive")
	for _, cus := range []string{"", taxCustomer(tc, "")} {
		resp := autoCheckout(tc, cus, price, autoTax)
		resp.AssertStatus(200)
		cs := resp.JSONMap()
		if autoStatus(cs) != "requires_location_inputs" || num(cs, "amount_total") != 46800 || num(cs["total_details"].(map[string]any), "amount_tax") != 0 {
			t.Errorf("customer %q: status %v amount_total %v total_details %v", cus, autoStatus(cs), cs["amount_total"], cs["total_details"])
		}
	}
}

// A session without automatic tax renders as it did before.
func TestCheckoutAutomaticTaxOffUnchanged(t *testing.T) {
	_, tc := setupStripe(t)
	seedTax(t, tc)
	cs := autoCheckout(tc, taxCustomer(tc, "CH"), yearlyPrice(tc, "exclusive"), "").JSONMap()
	if num(cs, "amount_total") != 46800 {
		t.Errorf("amount_total %v, want 46800", cs["amount_total"])
	}
	if at, _ := cs["automatic_tax"].(map[string]any); at != nil && at["enabled"] == true {
		t.Errorf("automatic_tax = %v, want off", at)
	}
}
