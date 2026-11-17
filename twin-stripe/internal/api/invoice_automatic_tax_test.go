package api_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

// invoiceTaxSeed is the tax seed the 2026-11-15 captures were taken under:
// Stripe Tax active, head office and one registration in CH at 8.1%.
const invoiceTaxSeed = `{
  "tax_settings": {"object": "tax.settings", "status": "active", "head_office": {"address": {"country": "CH"}},
    "defaults": {"provider": "stripe", "tax_behavior": "inferred_by_currency", "tax_code": "txcd_10000000"}},
  "tax_registrations": {"taxreg_seed_ch": {"id": "taxreg_seed_ch", "object": "tax.registration", "country": "CH",
    "country_options": {"ch": {"type": "standard"}}, "status": "active", "active_from": 1794700000,
    "standard_rate": {"percentage": 8.1, "tax_type": "vat", "display_name": "VAT"}}}
}`

const autoTax = "automatic_tax[enabled]=true"

func seedTax(t *testing.T, tc *testutil.TwinClient) {
	t.Helper()
	var seed map[string]any
	if err := json.Unmarshal([]byte(invoiceTaxSeed), &seed); err != nil {
		t.Fatal(err)
	}
	tc.Post("/admin/state", seed).AssertStatus(200)
}

// taxCustomer creates a customer, in country when it is not empty.
func taxCustomer(tc *testutil.TwinClient, country string) string {
	path := "/v1/customers?name=Tax"
	if country != "" {
		path += "&address[country]=" + country
	}
	resp := stripePost(tc, path, nil)
	resp.AssertStatus(200)
	return resp.JSONMap()["id"].(string)
}

// autoInvoice adds pending items of the given amounts and creates a draft that
// pulls them in, with extra query parameters.
func autoInvoice(tc *testutil.TwinClient, cus, extra string, amounts ...int) map[string]any {
	for _, a := range amounts {
		stripePost(tc, fmt.Sprintf("/v1/invoiceitems?customer=%s&amount=%d&currency=usd", cus, a), nil).AssertStatus(200)
	}
	resp := stripePost(tc, "/v1/invoices?customer="+cus+"&"+extra, nil)
	resp.AssertStatus(200)
	return resp.JSONMap()
}

func num(m map[string]any, k string) float64 { f, _ := m[k].(float64); return f }

func totalTaxes(m map[string]any) []map[string]any {
	var out []map[string]any
	list, _ := m["total_taxes"].([]any)
	for _, x := range list {
		out = append(out, x.(map[string]any))
	}
	return out
}

func autoStatus(m map[string]any) any {
	at, _ := m["automatic_tax"].(map[string]any)
	return at["status"]
}

// The tax-off refusal as Stripe answered it on 2026-11-15 (quirk error-006,
// captures t11 and t14): 400 invalid_request_error, this message, no code and no
// param. Stripe also sends an account-specific request_log_url, which the app
// emulator omits on every error.
func TestInvoiceAutomaticTaxRefusedWhenTaxIsNotSetUp(t *testing.T) {
	_, tc := setupStripe(t)
	cus := taxCustomer(tc, "CH")
	want := map[string]any{"error": map[string]any{
		"type":    "invalid_request_error",
		"message": "You must have a valid head office address to enable automatic tax calculation in test mode. Visit https://dashboard.stripe.com/test/settings/tax to update it.",
	}}

	resp := stripePost(tc, "/v1/invoices?customer="+cus+"&"+autoTax, nil)
	resp.AssertStatus(400)
	if got := resp.JSONMap(); !reflect.DeepEqual(got, want) {
		t.Errorf("create: %v, want %v", got, want)
	}

	plain := stripePost(tc, "/v1/invoices?customer="+cus, nil)
	plain.AssertStatus(200)
	inv := plain.JSONMap()
	wantOff := map[string]any{"enabled": false, "disabled_reason": nil, "liability": nil, "provider": nil, "status": nil}
	if !reflect.DeepEqual(inv["automatic_tax"], wantOff) {
		t.Errorf("plain invoice automatic_tax = %v, want %v", inv["automatic_tax"], wantOff)
	}

	resp = stripePost(tc, "/v1/invoices/"+inv["id"].(string)+"?"+autoTax, nil)
	resp.AssertStatus(400)
	if got := resp.JSONMap(); !reflect.DeepEqual(got, want) {
		t.Errorf("update: %v, want %v", got, want)
	}
	if got := stripeGet(tc, "/v1/invoices/"+inv["id"].(string)).JSONMap()["automatic_tax"]; !reflect.DeepEqual(got, wantOff) {
		t.Errorf("after the refused update automatic_tax = %v, want it unchanged", got)
	}
}

// Quirk lifecycle-005: the draft and the finalized invoice carry the same tax.
func TestInvoiceAutomaticTaxStandardRated(t *testing.T) {
	_, tc := setupStripe(t)
	seedTax(t, tc)
	inv := autoInvoice(tc, taxCustomer(tc, "CH"), autoTax, 46800)
	wantAT := map[string]any{"enabled": true, "disabled_reason": nil, "liability": map[string]any{"type": "self"}, "provider": "stripe", "status": "complete"}
	if !reflect.DeepEqual(inv["automatic_tax"], wantAT) {
		t.Errorf("automatic_tax = %v, want %v", inv["automatic_tax"], wantAT)
	}
	fin := stripePost(tc, "/v1/invoices/"+inv["id"].(string)+"/finalize", nil)
	fin.AssertStatus(200)
	for name, m := range map[string]map[string]any{"draft": inv, "finalized": fin.JSONMap()} {
		tt := totalTaxes(m)
		if len(tt) != 1 || num(tt[0], "amount") != 3791 || num(tt[0], "taxable_amount") != 46800 || tt[0]["taxability_reason"] != "standard_rated" {
			t.Errorf("%s total_taxes = %v", name, tt)
		}
		if num(m, "total") != 50591 || num(m, "total_excluding_tax") != 46800 || num(m, "amount_due") != 50591 {
			t.Errorf("%s totals: total %v excluding %v due %v", name, m["total"], m["total_excluding_tax"], m["amount_due"])
		}
	}
	rate := totalTaxes(inv)[0]["tax_rate_details"].(map[string]any)["tax_rate"].(string)
	if got := stripeGet(tc, "/v1/tax_rates/"+rate).JSONMap(); got["jurisdiction"] != "Switzerland" || got["active"] != false {
		t.Errorf("GET /v1/tax_rates/%s = %v", rate, got)
	}
}

func TestInvoiceAutomaticTaxAfterDiscount(t *testing.T) {
	_, tc := setupStripe(t)
	seedTax(t, tc)
	coupon := stripePost(tc, "/v1/coupons?percent_off=30&duration=forever", nil).JSONMap()["id"].(string)
	inv := autoInvoice(tc, taxCustomer(tc, "CH"), autoTax+"&discounts[0][coupon]="+coupon, 46800)
	tt := totalTaxes(inv)
	if len(tt) != 1 || num(tt[0], "amount") != 2654 || num(tt[0], "taxable_amount") != 32760 {
		t.Errorf("total_taxes = %v, want 2654 on 32760", tt)
	}
	if num(inv, "total") != 35414 || num(inv, "total_excluding_tax") != 32760 {
		t.Errorf("total %v excluding %v, want 35414 and 32760", inv["total"], inv["total_excluding_tax"])
	}
}

// Quirk crud-014: an unregistered country is taxed zero, not_collecting.
func TestInvoiceAutomaticTaxUnregisteredCountry(t *testing.T) {
	_, tc := setupStripe(t)
	seedTax(t, tc)
	inv := autoInvoice(tc, taxCustomer(tc, "US"), autoTax, 46800)
	tt := totalTaxes(inv)
	if len(tt) != 1 || num(tt[0], "amount") != 0 || num(tt[0], "taxable_amount") != 0 || tt[0]["taxability_reason"] != "not_collecting" {
		t.Errorf("total_taxes = %v, want one zero not_collecting entry", tt)
	}
	if num(inv, "total") != 46800 || autoStatus(inv) != "complete" {
		t.Errorf("total %v status %v", inv["total"], autoStatus(inv))
	}
}

// Quirk error-008: a draft for a customer with no location is created untaxed,
// requires_location_inputs; finalizing is refused and the invoice stays a
// draft until the customer has an address.
func TestInvoiceAutomaticTaxFinalizeWithoutLocation(t *testing.T) {
	_, tc := setupStripe(t)
	seedTax(t, tc)
	cus := taxCustomer(tc, "")
	inv := autoInvoice(tc, cus, autoTax, 46800)
	id := inv["id"].(string)
	if autoStatus(inv) != "requires_location_inputs" || len(totalTaxes(inv)) != 0 || num(inv, "total") != 46800 {
		t.Errorf("draft: status %v total_taxes %v total %v", autoStatus(inv), inv["total_taxes"], inv["total"])
	}

	resp := stripePost(tc, "/v1/invoices/"+id+"/finalize", nil)
	resp.AssertStatus(400)
	want := map[string]any{"error": map[string]any{
		"type":    "invalid_request_error",
		"code":    "customer_tax_location_invalid",
		"doc_url": "https://stripe.com/docs/error-codes/customer-tax-location-invalid",
		"message": "When `automatic_tax[enabled]=true`, enough customer location information must be provided to accurately determine tax rates for the customer.",
	}}
	if got := resp.JSONMap(); !reflect.DeepEqual(got, want) {
		t.Errorf("finalize: %v, want %v", got, want)
	}
	if got := stripeGet(tc, "/v1/invoices/"+id).JSONMap()["status"]; got != "draft" {
		t.Errorf("after the refusal status = %v, want draft", got)
	}
	stripePost(tc, "/v1/invoices/"+id+"/send", nil).AssertStatus(400)

	stripePost(tc, "/v1/customers/"+cus+"?address[country]=CH", nil).AssertStatus(200)
	fin := stripePost(tc, "/v1/invoices/"+id+"/finalize", nil)
	fin.AssertStatus(200)
	if m := fin.JSONMap(); m["status"] != "open" || num(m, "total") != 50591 || autoStatus(m) != "complete" {
		t.Errorf("finalized after the address: status %v total %v tax status %v", m["status"], m["total"], autoStatus(m))
	}
}

// A draft with no lines is complete and untaxed even without a location (t37, t51).
func TestInvoiceAutomaticTaxEmptyDraft(t *testing.T) {
	_, tc := setupStripe(t)
	seedTax(t, tc)
	inv := stripePost(tc, "/v1/invoices?customer="+taxCustomer(tc, "")+"&"+autoTax, nil).JSONMap()
	if autoStatus(inv) != "complete" || len(totalTaxes(inv)) != 0 || num(inv, "total") != 0 {
		t.Errorf("empty draft: status %v total_taxes %v total %v", autoStatus(inv), inv["total_taxes"], inv["total"])
	}
}

// Turning automatic tax on and off on a draft recomputes its totals.
func TestInvoiceAutomaticTaxToggledOnUpdate(t *testing.T) {
	_, tc := setupStripe(t)
	seedTax(t, tc)
	inv := autoInvoice(tc, taxCustomer(tc, "CH"), "", 46800)
	id := inv["id"].(string)
	if num(inv, "total") != 46800 {
		t.Fatalf("untaxed draft total %v", inv["total"])
	}
	on := stripePost(tc, "/v1/invoices/"+id+"?"+autoTax, nil)
	on.AssertStatus(200)
	if m := on.JSONMap(); num(m, "total") != 50591 || autoStatus(m) != "complete" {
		t.Errorf("enabled: total %v status %v", m["total"], autoStatus(m))
	}
	off := stripePost(tc, "/v1/invoices/"+id+"?automatic_tax[enabled]=false", nil)
	off.AssertStatus(200)
	if m := off.JSONMap(); num(m, "total") != 46800 || len(totalTaxes(m)) != 0 || m["automatic_tax"].(map[string]any)["enabled"] != false {
		t.Errorf("disabled: total %v total_taxes %v automatic_tax %v", m["total"], m["total_taxes"], m["automatic_tax"])
	}
}
