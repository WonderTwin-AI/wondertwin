package api_test

import (
	"reflect"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

// yearlyPrice creates a 46800 USD yearly price with taxBehavior, unless empty.
func yearlyPrice(tc *testutil.TwinClient, taxBehavior string) string {
	prod := stripePost(tc, "/v1/products?name=Tax", nil).JSONMap()["id"].(string)
	path := "/v1/prices?product=" + prod + "&currency=usd&unit_amount=46800&recurring[interval]=year"
	if taxBehavior != "" {
		path += "&tax_behavior=" + taxBehavior
	}
	resp := stripePost(tc, path, nil)
	resp.AssertStatus(200)
	return resp.JSONMap()["id"].(string)
}

func autoSubscription(tc *testutil.TwinClient, cus, price string) *testutil.Response {
	return stripePost(tc, "/v1/subscriptions?customer="+cus+"&items[0][price]="+price+"&"+autoTax, nil)
}

func latestInvoice(tc *testutil.TwinClient, sub map[string]any) map[string]any {
	resp := stripeGet(tc, "/v1/invoices/"+sub["latest_invoice"].(string))
	resp.AssertStatus(200)
	return resp.JSONMap()
}

var subscriptionTaxOn = map[string]any{"enabled": true, "disabled_reason": nil, "liability": map[string]any{"type": "self"}}

// The tax-off refusal as Stripe answered it on 2026-11-15 (quirk error-006,
// captures t08, t10, t19): it comes before the location check.
func TestSubscriptionAutomaticTaxRefusedWhenTaxIsNotSetUp(t *testing.T) {
	_, tc := setupStripe(t)
	price := yearlyPrice(tc, "exclusive")
	want := map[string]any{"error": map[string]any{
		"type":    "invalid_request_error",
		"message": "You must have a valid head office address to enable automatic tax calculation in test mode. Visit https://dashboard.stripe.com/test/settings/tax to update it.",
	}}
	for _, country := range []string{"CH", ""} {
		resp := autoSubscription(tc, taxCustomer(tc, country), price)
		resp.AssertStatus(400)
		if got := resp.JSONMap(); !reflect.DeepEqual(got, want) {
			t.Errorf("create, customer country %q: %v, want %v", country, got, want)
		}
	}

	plain := stripePost(tc, "/v1/subscriptions?customer="+taxCustomer(tc, "CH")+"&items[0][price]="+price, nil)
	plain.AssertStatus(200)
	sub := plain.JSONMap()
	off := map[string]any{"enabled": false, "disabled_reason": nil, "liability": nil}
	if !reflect.DeepEqual(sub["automatic_tax"], off) {
		t.Errorf("plain subscription automatic_tax = %v, want %v", sub["automatic_tax"], off)
	}
	resp := stripePost(tc, "/v1/subscriptions/"+sub["id"].(string)+"?"+autoTax, nil)
	resp.AssertStatus(400)
	if got := resp.JSONMap(); !reflect.DeepEqual(got, want) {
		t.Errorf("update: %v, want %v", got, want)
	}
}

// Quirk crud-016: the subscription's automatic_tax has no status or provider;
// its first invoice is taxed and paid.
func TestSubscriptionAutomaticTaxStandardRated(t *testing.T) {
	for _, c := range []struct {
		name, behavior   string
		tax, excl, total float64
		taxBehavior      string
	}{
		{"exclusive", "exclusive", 3791, 46800, 50591, "exclusive"},
		{"inclusive", "inclusive", 3507, 43293, 46800, "inclusive"},
		{"unspecified, USD default exclusive", "", 3791, 46800, 50591, "exclusive"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, tc := setupStripe(t)
			seedTax(t, tc)
			resp := autoSubscription(tc, taxCustomer(tc, "CH"), yearlyPrice(tc, c.behavior))
			resp.AssertStatus(200)
			sub := resp.JSONMap()
			if !reflect.DeepEqual(sub["automatic_tax"], subscriptionTaxOn) {
				t.Errorf("subscription automatic_tax = %v, want %v", sub["automatic_tax"], subscriptionTaxOn)
			}
			inv := latestInvoice(tc, sub)
			tt := totalTaxes(inv)
			if len(tt) != 1 || num(tt[0], "amount") != c.tax || tt[0]["tax_behavior"] != c.taxBehavior {
				t.Errorf("total_taxes = %v, want %v %s", tt, c.tax, c.taxBehavior)
			}
			if num(inv, "total") != c.total || num(inv, "total_excluding_tax") != c.excl || num(inv, "amount_paid") != c.total ||
				num(inv, "amount_remaining") != 0 || inv["status"] != "paid" || autoStatus(inv) != "complete" {
				t.Errorf("invoice: status %v total %v excluding %v paid %v remaining %v tax status %v", inv["status"],
					inv["total"], inv["total_excluding_tax"], inv["amount_paid"], inv["amount_remaining"], autoStatus(inv))
			}
		})
	}
}

// Quirk error-007: a customer with no location is refused at create, with
// nothing created.
func TestSubscriptionAutomaticTaxCreateWithoutLocation(t *testing.T) {
	_, tc := setupStripe(t)
	seedTax(t, tc)
	cus := taxCustomer(tc, "")
	resp := autoSubscription(tc, cus, yearlyPrice(tc, "exclusive"))
	resp.AssertStatus(400)
	want := map[string]any{"error": map[string]any{
		"type":    "invalid_request_error",
		"code":    "customer_tax_location_invalid",
		"doc_url": "https://stripe.com/docs/error-codes/customer-tax-location-invalid",
		"message": "The customer's location isn't recognized. Set a valid customer address in order to automatically calculate tax.",
	}}
	if got := resp.JSONMap(); !reflect.DeepEqual(got, want) {
		t.Errorf("create: %v, want %v", got, want)
	}
	if data := stripeGet(tc, "/v1/subscriptions?customer="+cus).JSONMap()["data"].([]any); len(data) != 0 {
		t.Errorf("a refused create left %d subscriptions", len(data))
	}
	if data := stripeGet(tc, "/v1/invoices?customer="+cus).JSONMap()["data"].([]any); len(data) != 0 {
		t.Errorf("a refused create left %d invoices", len(data))
	}
}

// Quirk lifecycle-008: doubling the quantity at once credits the unused year
// and charges two, each line taxed with its sign; the rate total is summed
// before rounding (capture t44).
func TestProrationTaxKeepsTheSign(t *testing.T) {
	_, tc := setupStripe(t)
	seedTax(t, tc)
	resp := autoSubscription(tc, taxCustomer(tc, "CH"), yearlyPrice(tc, "exclusive"))
	resp.AssertStatus(200)
	sub := resp.JSONMap()
	item := sub["items"].(map[string]any)["data"].([]any)[0].(map[string]any)["id"].(string)

	upd := stripePost(tc, "/v1/subscriptions/"+sub["id"].(string)+"?items[0][id]="+item+"&items[0][quantity]=2&proration_behavior=always_invoice", nil)
	upd.AssertStatus(200)
	updated := upd.JSONMap()
	if q := updated["items"].(map[string]any)["data"].([]any)[0].(map[string]any)["quantity"]; q != float64(2) {
		t.Errorf("item quantity %v, want 2 with the price kept", q)
	}
	inv := latestInvoice(tc, updated)
	if inv["id"] == sub["latest_invoice"] {
		t.Fatal("no proration invoice")
	}
	var got [][2]float64
	for _, l := range inv["lines"].(map[string]any)["data"].([]any) {
		line := l.(map[string]any)
		taxes, _ := line["taxes"].([]any)
		if len(taxes) != 1 {
			t.Fatalf("line %v taxes %v", line["amount"], line["taxes"])
		}
		got = append(got, [2]float64{num(line, "amount"), num(taxes[0].(map[string]any), "amount")})
	}
	if want := [][2]float64{{-46800, -3791}, {93600, 7582}}; !reflect.DeepEqual(got, want) {
		t.Errorf("lines (amount, tax) = %v, want %v", got, want)
	}
	tt := totalTaxes(inv)
	if len(tt) != 1 || num(tt[0], "amount") != 3791 || num(tt[0], "taxable_amount") != 46800 {
		t.Errorf("total_taxes = %v, want 3791 on 46800", tt)
	}
	if num(inv, "total") != 50591 || num(inv, "amount_due") != 50591 || autoStatus(inv) != "complete" {
		t.Errorf("total %v due %v status %v", inv["total"], inv["amount_due"], autoStatus(inv))
	}
}

// Without automatic tax a proration invoice stays untaxed, as before.
func TestProrationTaxOffLeavesProrationUntaxed(t *testing.T) {
	_, tc := setupStripe(t)
	resp := stripePost(tc, "/v1/subscriptions?customer="+taxCustomer(tc, "CH")+"&items[0][price]="+yearlyPrice(tc, "exclusive"), nil)
	sub := resp.JSONMap()
	item := sub["items"].(map[string]any)["data"].([]any)[0].(map[string]any)["id"].(string)
	updated := stripePost(tc, "/v1/subscriptions/"+sub["id"].(string)+"?items[0][id]="+item+"&items[0][quantity]=2", nil).JSONMap()
	inv := latestInvoice(tc, updated)
	if len(totalTaxes(inv)) != 0 || num(inv, "total") != 46800 {
		t.Errorf("total_taxes %v total %v, want untaxed 46800", inv["total_taxes"], inv["total"])
	}
}
