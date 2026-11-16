package api

import (
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// Stripe Tax, as the tax pattern (wondertwin-docs
// architecture/app-emulator-tax-emulation.md) scopes it: the customer's country
// selects a seeded registration, whose standard rate applies to each line after
// discounts. Sourcing rules beyond the country, taxability and non-standard
// rates are out of scope.

// taxLine is one line to tax: its amount after discounts (negative for a
// proration credit) and its price's tax_behavior.
type taxLine struct {
	Amount      int64
	TaxBehavior string
}

// customerTaxCountry is the country Stripe Tax locates a customer by: the
// shipping address, then the billing address. Empty when neither has one.
func customerTaxCountry(c store.Customer) string {
	if c.Shipping != nil && c.Shipping.Address != nil && c.Shipping.Address.Country != nil && *c.Shipping.Address.Country != "" {
		return strings.ToUpper(*c.Shipping.Address.Country)
	}
	if c.Address != nil && c.Address.Country != nil {
		return strings.ToUpper(*c.Address.Country)
	}
	return ""
}

// lineBehavior resolves a price's tax_behavior: its own when set, else the
// account default, where inferred_by_currency is exclusive for USD and CAD and
// inclusive otherwise.
func lineBehavior(priceBehavior, accountDefault, currency string) string {
	switch priceBehavior {
	case "inclusive", "exclusive":
		return priceBehavior
	}
	switch accountDefault {
	case "inclusive", "exclusive":
		return accountDefault
	}
	switch strings.ToLower(currency) {
	case "usd", "cad":
		return "exclusive"
	}
	return "inclusive"
}

// registrationFor is the active registration with a standard rate for country.
func (h *Handler) registrationFor(country string) (store.TaxRegistration, bool) {
	if country == "" {
		return store.TaxRegistration{}, false
	}
	for _, reg := range h.store.TaxRegistrations.List() {
		if strings.EqualFold(reg.Country, country) && reg.Status == "active" && reg.StandardRate != nil {
			return reg, true
		}
	}
	return store.TaxRegistration{}, false
}

// automaticTaxes taxes lines for a customer in country, with the currency
// deciding inferred_by_currency. A registered country is taxed at its standard
// rate, standard_rated; any other country gets a zero entry per line and one
// in total, not_collecting, as Stripe answers when it is not registered there.
func (h *Handler) automaticTaxes(country, currency string, lines []taxLine) taxedLines {
	accountDefault := ""
	if ts, ok := h.store.TaxSettings(); ok && ts.Defaults.TaxBehavior != nil {
		accountDefault = *ts.Defaults.TaxBehavior
	}
	reg, registered := h.registrationFor(country)
	if !registered {
		rate := h.stripeTaxRate(country, 0, false, "", "")
		out := taxedLines{lineTaxes: make([][]store.InvoiceTax, len(lines))}
		for i := range lines {
			out.lineTaxes[i] = []store.InvoiceTax{invoiceTax(0, 0, "exclusive", rate.ID, "not_collecting")}
		}
		out.totalTaxes = []store.InvoiceTax{invoiceTax(0, 0, "exclusive", rate.ID, "not_collecting")}
		return out
	}

	// Lines are taxed in two groups, by behaviour, each at its own rate object.
	out := taxedLines{lineTaxes: make([][]store.InvoiceTax, len(lines))}
	sr := reg.StandardRate
	for _, behavior := range []string{"exclusive", "inclusive"} {
		var idx []int
		var amounts []int64
		for i, l := range lines {
			if lineBehavior(l.TaxBehavior, accountDefault, currency) == behavior {
				idx = append(idx, i)
				amounts = append(amounts, l.Amount)
			}
		}
		if len(idx) == 0 {
			continue
		}
		rate := h.stripeTaxRate(reg.Country, sr.Percentage, behavior == "inclusive", sr.TaxType, sr.DisplayName)
		taxed := taxAtRates(amounts, []store.TaxRate{rate}, "standard_rated")
		for j, i := range idx {
			out.lineTaxes[i] = taxed.lineTaxes[j]
		}
		out.totalTaxes = append(out.totalTaxes, taxed.totalTaxes...)
		out.exclusiveTax += taxed.exclusiveTax
		out.inclusiveTax += taxed.inclusiveTax
	}
	return out
}

// stripeTaxRate is the rate object Stripe Tax references for one country, rate
// and behaviour, created once and reused, with the fields Stripe gave its own
// (inactive, country level, percentage). The zero rate behind not_collecting
// was only seen as throwaway preview IDs; its fields are assumed.
func (h *Handler) stripeTaxRate(country string, pct float64, inclusive bool, taxType, displayName string) store.TaxRate {
	country = strings.ToUpper(country)
	if displayName == "" {
		displayName = "Tax"
	}
	return h.store.StripeTaxRate(func(tr store.TaxRate) bool {
		return tr.Country == country && tr.Percentage == pct && tr.Inclusive == inclusive
	}, func(id string) store.TaxRate {
		p := pct
		return store.TaxRate{
			ID:                  id,
			Object:              "tax_rate",
			DisplayName:         displayName,
			Inclusive:           inclusive,
			Percentage:          pct,
			EffectivePercentage: &p,
			Country:             country,
			Jurisdiction:        countryName(country),
			JurisdictionLevel:   "country",
			RateType:            "percentage",
			TaxType:             taxType,
			Created:             h.store.Now(),
		}
	})
}

// countryName is the jurisdiction Stripe names a country rate after (CH is
// Switzerland). Countries not listed fall back to their code.
func countryName(code string) string {
	if n, ok := countryNames[code]; ok {
		return n
	}
	return code
}

var countryNames = map[string]string{
	"AT": "Austria", "AU": "Australia", "BE": "Belgium", "BG": "Bulgaria", "CA": "Canada",
	"CH": "Switzerland", "CY": "Cyprus", "CZ": "Czech Republic", "DE": "Germany", "DK": "Denmark",
	"EE": "Estonia", "ES": "Spain", "FI": "Finland", "FR": "France", "GB": "United Kingdom",
	"GR": "Greece", "HR": "Croatia", "HU": "Hungary", "IE": "Ireland", "IN": "India",
	"IS": "Iceland", "IT": "Italy", "JP": "Japan", "LT": "Lithuania", "LU": "Luxembourg",
	"LV": "Latvia", "MT": "Malta", "MX": "Mexico", "NL": "Netherlands", "NO": "Norway",
	"NZ": "New Zealand", "PL": "Poland", "PT": "Portugal", "RO": "Romania", "SE": "Sweden",
	"SG": "Singapore", "SI": "Slovenia", "SK": "Slovakia", "US": "United States", "ZA": "South Africa",
}
