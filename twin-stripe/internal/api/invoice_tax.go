package api

import (
	"math/big"
	"strconv"

	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// roundHalfUp rounds to the nearest minor unit, a .5 away from zero, as Stripe
// documents for tax ("rounds tax amounts half up to the smallest currency unit").
// It works on exact rationals so a decimal .5 is never lost to binary floats.
func roundHalfUp(x *big.Rat) int64 {
	abs := new(big.Rat).Abs(x)
	abs.Add(abs, big.NewRat(1, 2))
	n := new(big.Int).Quo(abs.Num(), abs.Denom()).Int64()
	if x.Sign() < 0 {
		return -n
	}
	return n
}

// percentRat is a tax rate's percentage as an exact fraction (8.1 is 81/1000).
func percentRat(pct float64) *big.Rat {
	r, _ := new(big.Rat).SetString(strconv.FormatFloat(pct, 'f', -1, 64))
	return r.Quo(r, big.NewRat(100, 1))
}

func ratInt(n int64) *big.Rat { return new(big.Rat).SetInt64(n) }

// allocate rounds each raw share half up, then moves single units so the shares
// add up to total: a missing unit goes to the share that lost most to rounding,
// an extra one comes off the share that gained most, the first share on a tie.
// Which line Stripe gives such a unit is not settled; this rule is declared in
// divergences.json.
func allocate(total int64, raw []*big.Rat) []int64 {
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
		var best *big.Rat
		for i, r := range raw {
			gap := new(big.Rat).Sub(r, ratInt(out[i])) // what rounding took away, in the direction we move
			if step < 0 {
				gap.Neg(gap)
			}
			if best == nil || gap.Cmp(best) > 0 {
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
	raw := make([]*big.Rat, len(lines))
	for i, l := range lines {
		raw[i] = new(big.Rat)
		if base != 0 {
			raw[i].SetFrac64(amount*l, base)
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

// taxAtRates taxes lines (amounts after discounts) at each rate. All inclusive
// rates are backed out of one shared base per line, price/(1+sum of inclusive
// percentages), and every rate, inclusive or exclusive, is applied to that base.
// Amounts at one rate are summed and rounded once, then allocated back to the
// lines.
func taxAtRates(lines []int64, rates []store.TaxRate, reason string) taxedLines {
	out := taxedLines{lineTaxes: make([][]store.InvoiceTax, len(lines))}
	if len(lines) == 0 {
		return out
	}
	divisor := big.NewRat(1, 1)
	for _, tr := range rates {
		if tr.Inclusive {
			divisor.Add(divisor, percentRat(tr.Percentage))
		}
	}
	bases := make([]*big.Rat, len(lines))
	for i, l := range lines {
		bases[i] = new(big.Rat).Quo(ratInt(l), divisor)
	}

	totals := make([]int64, len(rates))
	perLine := make([][]int64, len(rates))
	taxableLines := append([]int64(nil), lines...)
	for r, tr := range rates {
		p := percentRat(tr.Percentage)
		raw := make([]*big.Rat, len(lines))
		sum := new(big.Rat)
		for i := range lines {
			raw[i] = new(big.Rat).Mul(bases[i], p)
			sum.Add(sum, raw[i])
		}
		totals[r] = roundHalfUp(sum)
		perLine[r] = allocate(totals[r], raw)
		if tr.Inclusive {
			for i := range lines {
				taxableLines[i] -= perLine[r][i]
			}
		}
	}

	var taxable int64
	for _, t := range taxableLines {
		taxable += t
	}
	for r, tr := range rates {
		behavior := "exclusive"
		if tr.Inclusive {
			behavior = "inclusive"
			out.inclusiveTax += totals[r]
		} else {
			out.exclusiveTax += totals[r]
		}
		for i := range lines {
			out.lineTaxes[i] = append(out.lineTaxes[i], invoiceTax(perLine[r][i], taxableLines[i], behavior, tr.ID, reason))
		}
		out.totalTaxes = append(out.totalTaxes, invoiceTax(totals[r], taxable, behavior, tr.ID, reason))
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
