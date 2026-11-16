package api

import (
	"math"

	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// roundHalfUp rounds to the nearest minor unit, a .5 away from zero, as Stripe
// documents for tax ("rounds tax amounts half up to the smallest currency unit").
func roundHalfUp(x float64) int64 {
	if x < 0 {
		return -int64(math.Floor(-x + 0.5))
	}
	return int64(math.Floor(x + 0.5))
}

// allocate rounds each raw share half up, then moves single units so the shares
// add up to total: a missing unit goes to the share that lost most to rounding,
// an extra one comes off the share that gained most, the first share on a tie.
// Which line Stripe gives such a unit is not settled; this rule is declared in
// divergences.json.
func allocate(total int64, raw []float64) []int64 {
	out := make([]int64, len(raw))
	var sum int64
	for i, r := range raw {
		out[i] = roundHalfUp(r)
		sum += out[i]
	}
	for sum != total && len(out) > 0 {
		pick, step := 0, int64(1)
		if sum > total {
			step = -1
		}
		best := math.Inf(-1)
		for i, r := range raw {
			gap := (r - float64(out[i])) * float64(step) // what rounding took away, in the direction we move
			if gap > best+1e-9 {
				best, pick = gap, i
			}
		}
		out[pick] += step
		sum += step
	}
	return out
}

// spread splits amount across lines in proportion to their amounts, with the
// same rounding rule as allocate.
func spread(amount int64, lines []int64) []int64 {
	var base int64
	for _, l := range lines {
		base += l
	}
	raw := make([]float64, len(lines))
	for i, l := range lines {
		if base != 0 {
			raw[i] = float64(amount) * float64(l) / float64(base)
		}
	}
	return allocate(amount, raw)
}

// taxedLines is the result of taxing an invoice's lines at its tax rates.
type taxedLines struct {
	totalTaxes   []store.InvoiceTax
	lineTaxes    [][]store.InvoiceTax
	exclusiveTax int64 // added on top of the total
	inclusiveTax int64 // carved out of the prices
}

// taxAtRates taxes lines (amounts after discounts) at each rate. Amounts at one
// rate are summed and rounded once, then allocated back to the lines. An
// exclusive rate adds tax on top; an inclusive rate carves it out of the price.
func taxAtRates(lines []int64, rates []store.TaxRate, reason string) taxedLines {
	out := taxedLines{lineTaxes: make([][]store.InvoiceTax, len(lines))}
	if len(lines) == 0 {
		return out
	}
	for _, tr := range rates {
		p := tr.Percentage / 100
		raw := make([]float64, len(lines))
		var gross float64
		for i, l := range lines {
			gross += float64(l)
			if tr.Inclusive {
				raw[i] = float64(l) - float64(l)/(1+p)
			} else {
				raw[i] = float64(l) * p
			}
		}
		var total int64
		if tr.Inclusive {
			total = roundHalfUp(gross) - roundHalfUp(gross/(1+p))
		} else {
			total = roundHalfUp(gross * p)
		}
		perLine := allocate(total, raw)
		behavior := "exclusive"
		if tr.Inclusive {
			behavior = "inclusive"
		}
		var taxable int64
		for i, l := range lines {
			lineTaxable := l
			if tr.Inclusive {
				lineTaxable = l - perLine[i]
			}
			taxable += lineTaxable
			out.lineTaxes[i] = append(out.lineTaxes[i], invoiceTax(perLine[i], lineTaxable, behavior, tr.ID, reason))
		}
		out.totalTaxes = append(out.totalTaxes, invoiceTax(total, taxable, behavior, tr.ID, reason))
		if tr.Inclusive {
			out.inclusiveTax += total
		} else {
			out.exclusiveTax += total
		}
	}
	return out
}

func invoiceTax(amount, taxable int64, behavior, rate, reason string) store.InvoiceTax {
	return store.InvoiceTax{
		Amount:           amount,
		TaxBehavior:      behavior,
		TaxRateDetails:   store.TaxRateDetails{TaxRate: rate},
		TaxabilityReason: reason,
		TaxableAmount:    taxable,
		Type:             "tax_rate_details",
	}
}

// applyManualTaxes taxes an invoice at manually set tax rates (default_tax_rates)
// and sets its tax fields and totals. Stripe reports manual rates as
// taxability_reason not_available.
func applyManualTaxes(inv *store.Invoice, discount int64, rates []store.TaxRate) {
	var amounts []int64
	if inv.Lines != nil {
		for _, l := range inv.Lines.Data {
			amounts = append(amounts, l.Amount)
		}
	}
	shares := spread(discount, amounts)
	taxableLines := make([]int64, len(amounts))
	for i, a := range amounts {
		taxableLines[i] = a - shares[i]
	}
	taxed := taxAtRates(taxableLines, rates, "not_available")
	if inv.Lines != nil {
		for i := range inv.Lines.Data {
			inv.Lines.Data[i].Taxes = taxed.lineTaxes[i]
		}
	}
	inv.TotalTaxes = taxed.totalTaxes
	excluding := inv.Subtotal - discount - taxed.inclusiveTax
	inv.TotalExcludingTax = &excluding
	inv.Total = inv.Subtotal - discount + taxed.exclusiveTax
}
