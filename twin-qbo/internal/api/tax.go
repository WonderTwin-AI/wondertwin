package api

import "github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"

// resolveTaxRate looks up a tax code and returns its effective rate.
// Returns 0 if the tax code is not found or not taxable.
func (h *Handler) resolveTaxRate(taxCodeRef *store.Ref) float64 {
	if taxCodeRef == nil {
		return 0
	}
	tc, ok := h.store.TaxCodes.Get(taxCodeRef.Value)
	if !ok || !tc.Taxable {
		return 0
	}
	// Look for a tax rate with matching name or use the first active rate.
	rates := h.store.TaxRates.Filter(func(_ string, tr store.TaxRate) bool {
		return tr.Active
	})
	if len(rates) > 0 {
		return rates[0].RateValue
	}
	return 0
}

// computeTaxForLines calculates tax for line items and populates TxnTaxDetail.
func (h *Handler) computeTaxForLines(lines []store.Line, existing *store.TxnTaxDetail) *store.TxnTaxDetail {
	// If tax detail is already explicitly provided, use it.
	if existing != nil && existing.TotalTax > 0 {
		return existing
	}

	var totalTax float64
	var taxLines []store.TaxLine

	for _, line := range lines {
		var taxCodeRef *store.Ref
		if line.SalesItemLineDetail != nil && line.SalesItemLineDetail.TaxCodeRef != nil {
			taxCodeRef = line.SalesItemLineDetail.TaxCodeRef
		} else if line.AccountBasedExpenseLineDetail != nil && line.AccountBasedExpenseLineDetail.TaxCodeRef != nil {
			taxCodeRef = line.AccountBasedExpenseLineDetail.TaxCodeRef
		}
		if taxCodeRef == nil {
			continue
		}

		rate := h.resolveTaxRate(taxCodeRef)
		if rate <= 0 {
			continue
		}

		taxAmt := line.Amount * rate / 100.0
		totalTax += taxAmt
		taxLines = append(taxLines, store.TaxLine{
			Amount:     taxAmt,
			DetailType: "TaxLineDetail",
			TaxLineDetail: &store.TaxLineDetail{
				TaxRateRef:       taxCodeRef,
				PercentBased:     true,
				TaxPercent:       rate,
				NetAmountTaxable: line.Amount,
			},
		})
	}

	if totalTax == 0 {
		return existing
	}

	return &store.TxnTaxDetail{
		TxnTaxCodeRef: nil,
		TotalTax:      totalTax,
		TaxLine:       taxLines,
	}
}
