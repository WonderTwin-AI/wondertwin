package api_test

import (
	"fmt"
	"testing"
)

// taxRateSpec is a manual tax rate to create for a test invoice.
type taxRateSpec struct {
	percentage string
	inclusive  bool
}

// manualInvoice creates a customer, pending items of the given amounts and an
// invoice taxed at one manual rate, and returns the invoice as JSON.
func manualInvoice(t *testing.T, percentage string, inclusive bool, amounts ...int) map[string]any {
	t.Helper()
	return manualInvoiceRates(t, []taxRateSpec{{percentage, inclusive}}, amounts...)
}

// manualInvoiceRates is manualInvoice with any number of manual rates. The
// first rate's ID is returned under "_rate".
func manualInvoiceRates(t *testing.T, rates []taxRateSpec, amounts ...int) map[string]any {
	t.Helper()
	_, tc := setupStripe(t)
	var ids []string
	for _, r := range rates {
		resp := stripePost(tc, fmt.Sprintf("/v1/tax_rates?display_name=VAT&percentage=%s&inclusive=%t&country=CH", r.percentage, r.inclusive), nil)
		resp.AssertStatus(200)
		ids = append(ids, resp.JSONMap()["id"].(string))
	}
	resp := stripePost(tc, "/v1/customers?name=Tax", nil)
	resp.AssertStatus(200)
	cus := resp.JSONMap()["id"].(string)
	for _, a := range amounts {
		stripePost(tc, fmt.Sprintf("/v1/invoiceitems?customer=%s&amount=%d&currency=usd", cus, a), nil).AssertStatus(200)
	}
	path := "/v1/invoices?customer=" + cus
	for i, id := range ids {
		path += fmt.Sprintf("&default_tax_rates[%d]=%s", i, id)
	}
	resp = stripePost(tc, path, nil)
	resp.AssertStatus(200)
	inv := resp.JSONMap()
	resp = stripePost(tc, "/v1/invoices/"+inv["id"].(string)+"/finalize", nil)
	resp.AssertStatus(200)
	out := resp.JSONMap()
	out["_rate"] = ids[0]
	return out
}

func lineTaxAmounts(t *testing.T, inv map[string]any) []float64 {
	t.Helper()
	var out []float64
	for _, l := range inv["lines"].(map[string]any)["data"].([]any) {
		taxes := l.(map[string]any)["taxes"].([]any)
		if len(taxes) != 1 {
			t.Fatalf("expected one tax per line, got %v", taxes)
		}
		out = append(out, taxes[0].(map[string]any)["amount"].(float64))
	}
	return out
}

func TestInvoiceTaxShape(t *testing.T) {
	inv := manualInvoice(t, "8.1", false, 46800)
	if _, ok := inv["total_tax_amounts"]; ok {
		t.Errorf("total_tax_amounts was removed in basil; got %v", inv["total_tax_amounts"])
	}
	taxes := inv["total_taxes"].([]any)
	if len(taxes) != 1 {
		t.Fatalf("expected one total_taxes entry, got %v", taxes)
	}
	tax := taxes[0].(map[string]any)
	want := map[string]any{"amount": 3791.0, "taxable_amount": 46800.0, "tax_behavior": "exclusive",
		"taxability_reason": "not_available", "type": "tax_rate_details"}
	for k, v := range want {
		if tax[k] != v {
			t.Errorf("total_taxes[0].%s = %v, want %v", k, tax[k], v)
		}
	}
	if got := tax["tax_rate_details"].(map[string]any)["tax_rate"]; got != inv["_rate"] {
		t.Errorf("tax_rate_details.tax_rate = %v, want %v", got, inv["_rate"])
	}
	if got := lineTaxAmounts(t, inv); len(got) != 1 || got[0] != 3791 {
		t.Errorf("line taxes = %v, want [3791]", got)
	}
	for k, v := range map[string]float64{"subtotal": 46800, "total_excluding_tax": 46800, "total": 50591, "amount_due": 50591} {
		if inv[k] != v {
			t.Errorf("%s = %v, want %v", k, inv[k], v)
		}
	}
	at := inv["automatic_tax"].(map[string]any)
	if at["enabled"] != false || at["status"] != nil || at["provider"] != nil {
		t.Errorf("automatic_tax = %v, want disabled with null status and provider", at)
	}
}

func TestInvoiceTaxRounding(t *testing.T) {
	cases := []struct {
		name      string
		pct       string
		inclusive bool
		amounts   []int
		total     float64
		lines     []float64
		sum       float64 // invoice total
	}{
		// 8.1% of 500 is 40.5: half up gives 41, half-even would give 40.
		{"half cent rounds up", "8.1", false, []int{500}, 41, []float64{41}, 541},
		// Rounded once on the sum (244.458), then one cent to the line rounding lost most on.
		{"rounded on the sum", "8.1", false, []int{1006, 1006, 1006}, 244, []float64{82, 81, 81}, 3262},
		// Carved out of an inclusive price: 46800 - 46800/1.081 = 3506.75.
		{"inclusive carve out", "8.1", true, []int{46800}, 3507, []float64{3507}, 46800},
		// Exact decimal ties that binary floats land just below.
		{"7.25 percent tie", "7.25", false, []int{200}, 15, []float64{15}, 215},
		{"7.45 percent below half", "7.45", false, []int{100}, 7, []float64{7}, 107},
		{"7.45 percent tie", "7.45", false, []int{1000}, 75, []float64{75}, 1075},
		{"1.15 percent below half", "1.15", false, []int{300}, 3, []float64{3}, 303},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inv := manualInvoice(t, c.pct, c.inclusive, c.amounts...)
			tax := inv["total_taxes"].([]any)[0].(map[string]any)
			if tax["amount"] != c.total {
				t.Errorf("total tax = %v, want %v", tax["amount"], c.total)
			}
			got := lineTaxAmounts(t, inv)
			var sum float64
			for _, g := range got {
				sum += g
			}
			if sum != c.total || len(got) != len(c.lines) {
				t.Errorf("line taxes %v do not add up to %v", got, c.total)
			}
			if inv["total"] != c.sum {
				t.Errorf("invoice total = %v, want %v", inv["total"], c.sum)
			}
		})
	}
}

func TestInvoiceTaxSharedInclusiveBase(t *testing.T) {
	// Two 10% inclusive rates on 12000 back out of one base: 12000/1.2 = 10000.
	inv := manualInvoiceRates(t, []taxRateSpec{{"10", true}, {"10", true}}, 12000)
	taxes := inv["total_taxes"].([]any)
	if len(taxes) != 2 {
		t.Fatalf("expected two total_taxes entries, got %v", taxes)
	}
	for i, tx := range taxes {
		tax := tx.(map[string]any)
		if tax["amount"] != 1000.0 || tax["taxable_amount"] != 10000.0 || tax["tax_behavior"] != "inclusive" {
			t.Errorf("total_taxes[%d] = %v, want amount 1000 on taxable 10000, inclusive", i, tax)
		}
	}
	lineTaxes := inv["lines"].(map[string]any)["data"].([]any)[0].(map[string]any)["taxes"].([]any)
	for i, tx := range lineTaxes {
		tax := tx.(map[string]any)
		if tax["amount"] != 1000.0 || tax["taxable_amount"] != 10000.0 {
			t.Errorf("lines[0].taxes[%d] = %v, want amount 1000 on taxable 10000", i, tax)
		}
	}
	for k, v := range map[string]float64{"subtotal": 12000, "total_excluding_tax": 10000, "total": 12000} {
		if inv[k] != v {
			t.Errorf("%s = %v, want %v", k, inv[k], v)
		}
	}
}

func TestInvoiceTaxExclusiveOnInclusiveBase(t *testing.T) {
	// A 10% exclusive rate beside a 20% inclusive one taxes the 10000 base, not 12000.
	inv := manualInvoiceRates(t, []taxRateSpec{{"20", true}, {"10", false}}, 12000)
	taxes := inv["total_taxes"].([]any)
	if len(taxes) != 2 {
		t.Fatalf("expected two total_taxes entries, got %v", taxes)
	}
	want := []float64{2000, 1000}
	for i, tx := range taxes {
		tax := tx.(map[string]any)
		if tax["amount"] != want[i] || tax["taxable_amount"] != 10000.0 {
			t.Errorf("total_taxes[%d] = %v, want amount %v on taxable 10000", i, tax, want[i])
		}
	}
	for k, v := range map[string]float64{"total_excluding_tax": 10000, "total": 13000} {
		if inv[k] != v {
			t.Errorf("%s = %v, want %v", k, inv[k], v)
		}
	}
}
