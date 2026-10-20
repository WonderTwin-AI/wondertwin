package api

import (
	"fmt"
	"math"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// taxRates holds the rate of a TaxType as a fraction. Xero reads it from the
// tenant's tax rates, which this build does not model (TaxRates is outside
// xero-bp1), so every type is 0% until one is listed here. A TaxAmount the client
// sends is always kept. This is a guess, not Xero's table.
var taxRates = map[string]float64{}

// totals are the computed amounts of an invoice (crud-004).
type totals struct {
	SubTotal, TotalTax, Total, TotalDiscount float64
}

func round(f float64, dp int) float64 {
	p := math.Pow10(dp)
	return math.Round(f*p) / p
}

func ptr(f float64) *float64 { return &f }

// computeLines fills in each line's amounts and returns the invoice totals.
// LineAmountTypes is Exclusive (tax added), Inclusive (tax inside the line
// amount) or NoTax. unitdp is how many decimals a UnitAmount keeps (2 by
// default, 4 with unitdp=4). Quantity defaults to 1, a line with only a
// LineAmount takes it as its UnitAmount, and a LineAmount that disagrees with
// Quantity x UnitAmount less any discount is refused. The wording of the
// refusals is a guess.
func computeLines(lineAmountTypes string, lines []store.LineItem, unitdp int) ([]store.LineItem, totals, []string) {
	var t totals
	var msgs []string
	switch lineAmountTypes {
	case "", "Exclusive", "Inclusive", "NoTax":
	default:
		msgs = append(msgs, "LineAmountTypes must be Exclusive, Inclusive or NoTax.")
	}
	out := make([]store.LineItem, len(lines))
	for i, l := range lines {
		qty := 1.0
		if l.Quantity != nil {
			qty = *l.Quantity
		}
		gross := 0.0
		switch {
		case l.UnitAmount != nil:
			l.UnitAmount = ptr(round(*l.UnitAmount, unitdp))
			gross = qty * *l.UnitAmount
		case l.LineAmount != nil && l.DiscountRate != nil:
			msgs = append(msgs, fmt.Sprintf("Line %d: a UnitAmount is required with a DiscountRate.", i+1))
		case l.LineAmount != nil:
			gross = *l.LineAmount
			if qty != 0 {
				l.UnitAmount = ptr(round(gross/qty, unitdp))
			}
		}
		net := gross
		if l.DiscountRate != nil {
			net = gross * (100 - *l.DiscountRate) / 100
		} else if l.DiscountAmount != nil {
			net = gross - *l.DiscountAmount
		}
		net = round(net, 2)
		if l.LineAmount != nil && l.UnitAmount != nil && math.Abs(*l.LineAmount-net) > 0.005 {
			msgs = append(msgs, fmt.Sprintf("Line %d: the LineAmount does not equal Quantity x UnitAmount less any discount.", i+1))
		}
		l.Quantity, l.LineAmount = ptr(qty), ptr(net)
		t.TotalDiscount += round(gross, 2) - net

		tax := 0.0
		switch {
		case lineAmountTypes == "NoTax":
		case l.TaxAmount != nil:
			tax = *l.TaxAmount
		case lineAmountTypes == "Inclusive":
			tax = round(net-net/(1+taxRates[l.TaxType]), 2)
		default:
			tax = round(net*taxRates[l.TaxType], 2)
		}
		l.TaxAmount = ptr(tax)
		t.TotalTax += tax
		t.SubTotal += net
		if l.Tracking == nil {
			l.Tracking = []store.LineItemTracking{}
		}
		out[i] = l
	}
	t.SubTotal, t.TotalTax, t.TotalDiscount = round(t.SubTotal, 2), round(t.TotalTax, 2), round(t.TotalDiscount, 2)
	t.Total = t.SubTotal + t.TotalTax
	if lineAmountTypes == "Inclusive" {
		t.Total, t.SubTotal = t.SubTotal, round(t.SubTotal-t.TotalTax, 2)
	}
	t.Total = round(t.Total, 2)
	return out, t, msgs
}
