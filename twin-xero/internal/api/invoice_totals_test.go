package api

import (
	"math"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func line(qty, unit, amount, discount *float64) store.LineItem {
	return store.LineItem{Quantity: qty, UnitAmount: unit, LineAmount: amount, DiscountRate: discount}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.0001 }

func TestInvoiceTotalsComputedFromTheLines(t *testing.T) {
	cases := []struct {
		name  string
		types string
		lines []store.LineItem
		want  totals
	}{
		{"quantity times unit", "Exclusive", []store.LineItem{line(ptr(3), ptr(10), nil, nil)}, totals{SubTotal: 30, Total: 30}},
		{"default LineAmountTypes is Exclusive", "", []store.LineItem{line(ptr(2), ptr(5.5), nil, nil)}, totals{SubTotal: 11, Total: 11}},
		{"a fee line with only a LineAmount", "Exclusive", []store.LineItem{line(nil, nil, ptr(25.5), nil)}, totals{SubTotal: 25.5, Total: 25.5}},
		{"a negative proration line", "Exclusive", []store.LineItem{line(ptr(1), ptr(100), nil, nil), line(nil, nil, ptr(-40), nil)}, totals{SubTotal: 60, Total: 60}},
		{"quantity with only a LineAmount", "Exclusive", []store.LineItem{line(ptr(4), nil, ptr(10), nil)}, totals{SubTotal: 10, Total: 10}},
		{"a discount rate", "Exclusive", []store.LineItem{line(ptr(2), ptr(50), nil, ptr(10))}, totals{SubTotal: 90, Total: 90, TotalDiscount: 10}},
		{"no lines", "Exclusive", nil, totals{}},
		{"NoTax ignores a TaxAmount", "NoTax", []store.LineItem{{LineAmount: ptr(40), TaxAmount: ptr(4)}}, totals{SubTotal: 40, Total: 40}},
		{"Exclusive adds a given TaxAmount", "Exclusive", []store.LineItem{{LineAmount: ptr(40), TaxAmount: ptr(4)}}, totals{SubTotal: 40, TotalTax: 4, Total: 44}},
		{"Inclusive keeps the tax inside", "Inclusive", []store.LineItem{{LineAmount: ptr(44), TaxAmount: ptr(4)}}, totals{SubTotal: 40, TotalTax: 4, Total: 44}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines, got, msgs := computeLines(tc.types, tc.lines, 2)
			if len(msgs) != 0 || !near(got.SubTotal, tc.want.SubTotal) || !near(got.TotalTax, tc.want.TotalTax) ||
				!near(got.Total, tc.want.Total) || !near(got.TotalDiscount, tc.want.TotalDiscount) {
				t.Fatalf("got %+v %v, want %+v", got, msgs, tc.want)
			}
			for _, l := range lines {
				if l.Quantity == nil || l.LineAmount == nil || l.TaxAmount == nil || l.Tracking == nil {
					t.Fatalf("a computed line must carry Quantity, LineAmount, TaxAmount and Tracking: %+v", l)
				}
			}
		})
	}
}

func TestInvoiceTotalsDeriveTheMissingAmounts(t *testing.T) {
	lines, _, _ := computeLines("Exclusive", []store.LineItem{line(ptr(4), nil, ptr(10), nil), line(nil, nil, ptr(7), nil)}, 2)
	if *lines[0].UnitAmount != 2.5 || *lines[0].Quantity != 4 {
		t.Fatalf("line 1 = %+v, want unit 2.5 for quantity 4", lines[0])
	}
	if *lines[1].UnitAmount != 7 || *lines[1].Quantity != 1 {
		t.Fatalf("line 2 = %+v, want quantity 1 and unit 7", lines[1])
	}
}

func TestInvoiceTotalsUnitAmountKeepsUnitdpDecimals(t *testing.T) {
	for dp, want := range map[int]float64{2: 0.12, 4: 0.1235} {
		lines, _, _ := computeLines("Exclusive", []store.LineItem{line(ptr(1), ptr(0.12345), nil, nil)}, dp)
		if *lines[0].UnitAmount != want {
			t.Errorf("unitdp %d: UnitAmount = %v, want %v", dp, *lines[0].UnitAmount, want)
		}
	}
}

func TestInvoiceTotalsRefuseWhatDoesNotAddUp(t *testing.T) {
	cases := map[string]struct {
		types string
		line  store.LineItem
		want  string
	}{
		"a LineAmount that disagrees":        {"Exclusive", line(ptr(2), ptr(50), ptr(90), nil), "does not equal Quantity x UnitAmount"},
		"a discount with no UnitAmount":      {"Exclusive", line(nil, nil, ptr(90), ptr(10)), "UnitAmount is required with a DiscountRate"},
		"an unknown LineAmountTypes":         {"Tax Free", line(ptr(1), ptr(1), nil, nil), "LineAmountTypes must be"},
		"a LineAmount that agrees is silent": {"Exclusive", line(ptr(2), ptr(50), ptr(100), nil), ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, msgs := computeLines(tc.types, []store.LineItem{tc.line}, 2)
			joined := strings.Join(msgs, "; ")
			if (tc.want == "" && joined != "") || !strings.Contains(joined, tc.want) {
				t.Fatalf("messages = %q, want one containing %q", joined, tc.want)
			}
		})
	}
}
