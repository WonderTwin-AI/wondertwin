package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// stripeTaxSeed is the tax seed the 2026-11-15 captures were taken under:
// Stripe Tax active, head office and one registration in CH at 8.1%.
const stripeTaxSeed = `{
  "tax_settings": {"object": "tax.settings", "status": "active", "head_office": {"address": {"country": "CH"}},
    "defaults": {"provider": "stripe", "tax_behavior": "inferred_by_currency", "tax_code": "txcd_10000000"}},
  "tax_registrations": {"taxreg_seed_ch": {"id": "taxreg_seed_ch", "object": "tax.registration", "country": "CH",
    "country_options": {"ch": {"type": "standard"}}, "status": "active", "active_from": 1794700000,
    "standard_rate": {"percentage": 8.1, "tax_type": "vat", "display_name": "VAT"}}}
}`

func seededTaxHandler(t *testing.T, seed string) *Handler {
	t.Helper()
	s := store.New()
	if err := s.LoadState([]byte(seed)); err != nil {
		t.Fatal(err)
	}
	return NewHandler(s, nil, nil)
}

func lineAmounts(taxed taxedLines) []int64 {
	var out []int64
	for _, lt := range taxed.lineTaxes {
		var sum int64
		for _, t := range lt {
			sum += t.Amount
		}
		out = append(out, sum)
	}
	return out
}

// The captured figures (quirks lifecycle-005, -006, -008, -009): CH 8.1% after
// discounts, rounded half up, same-rate amounts summed then allocated.
func TestStandardRatedTaxReplaysCapturedFigures(t *testing.T) {
	for _, c := range []struct {
		name          string
		lines         []taxLine
		total         int64
		taxable       int64
		lineTax       []int64 // sorted; which line takes a cent is lifecycle-007
		behavior      string
		exclusive     int64
		inclusive     int64
		reason        string
		totalEntries  int
		totalCurrency string
	}{
		{"exclusive 46800", []taxLine{{46800, "exclusive"}}, 3791, 46800, []int64{3791}, "exclusive", 3791, 0, "standard_rated", 1, "usd"},
		{"30% off, 32760", []taxLine{{32760, "exclusive"}}, 2654, 32760, []int64{2654}, "exclusive", 2654, 0, "standard_rated", 1, "usd"},
		{"inclusive 46800", []taxLine{{46800, "inclusive"}}, 3507, 43293, []int64{3507}, "inclusive", 0, 3507, "standard_rated", 1, "usd"},
		{"three lines of 1006", []taxLine{{1006, "exclusive"}, {1006, "exclusive"}, {1006, "exclusive"}}, 244, 3018, []int64{81, 81, 82}, "exclusive", 244, 0, "standard_rated", 1, "usd"},
		{"tie on 500", []taxLine{{500, "exclusive"}}, 41, 500, []int64{41}, "exclusive", 41, 0, "standard_rated", 1, "usd"},
		{"tie on 1500", []taxLine{{1500, "exclusive"}}, 122, 1500, []int64{122}, "exclusive", 122, 0, "standard_rated", 1, "usd"},
		{"proration credit keeps its sign", []taxLine{{-46800, "exclusive"}}, -3791, -46800, []int64{-3791}, "exclusive", -3791, 0, "standard_rated", 1, "usd"},
		{"net-zero proration", []taxLine{{-46800, "exclusive"}, {46800, "exclusive"}}, 0, 0, []int64{-3791, 3791}, "exclusive", 0, 0, "standard_rated", 1, "usd"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := seededTaxHandler(t, stripeTaxSeed)
			taxed := h.automaticTaxes("CH", c.totalCurrency, c.lines)
			if len(taxed.totalTaxes) != c.totalEntries {
				t.Fatalf("total_taxes = %+v, want %d entry", taxed.totalTaxes, c.totalEntries)
			}
			tt := taxed.totalTaxes[0]
			if tt.Amount != c.total || tt.TaxableAmount != c.taxable || tt.TaxBehavior != c.behavior || tt.TaxabilityReason != c.reason {
				t.Errorf("total_taxes[0] = %+v, want amount %d taxable %d %s %s", tt, c.total, c.taxable, c.behavior, c.reason)
			}
			got := lineAmounts(taxed)
			sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
			if len(got) != len(c.lineTax) {
				t.Fatalf("line taxes %v, want %v", got, c.lineTax)
			}
			for i := range got {
				if got[i] != c.lineTax[i] {
					t.Fatalf("line taxes %v, want %v", got, c.lineTax)
				}
			}
			if taxed.exclusiveTax != c.exclusive || taxed.inclusiveTax != c.inclusive {
				t.Errorf("exclusive %d inclusive %d, want %d and %d", taxed.exclusiveTax, taxed.inclusiveTax, c.exclusive, c.inclusive)
			}
		})
	}
}

// Quirk crud-017: an unspecified price takes the account default; with
// inferred_by_currency, USD and CAD are exclusive and other currencies inclusive.
func TestStandardRatedTaxBehaviorDefaults(t *testing.T) {
	inferred := seededTaxHandler(t, stripeTaxSeed)
	for _, c := range []struct {
		currency, price, want string
	}{
		{"usd", "unspecified", "exclusive"},
		{"cad", "", "exclusive"},
		{"chf", "unspecified", "inclusive"},
		{"eur", "exclusive", "exclusive"},
		{"usd", "inclusive", "inclusive"},
	} {
		taxed := inferred.automaticTaxes("CH", c.currency, []taxLine{{46800, c.price}})
		if got := taxed.totalTaxes[0].TaxBehavior; got != c.want {
			t.Errorf("%s price %q: tax_behavior %s, want %s", c.currency, c.price, got, c.want)
		}
	}

	var seed map[string]any
	_ = json.Unmarshal([]byte(stripeTaxSeed), &seed)
	seed["tax_settings"].(map[string]any)["defaults"].(map[string]any)["tax_behavior"] = "inclusive"
	b, _ := json.Marshal(seed)
	h := seededTaxHandler(t, string(b))
	if got := h.automaticTaxes("CH", "usd", []taxLine{{46800, "unspecified"}}).totalTaxes[0].TaxBehavior; got != "inclusive" {
		t.Errorf("account default inclusive, usd unspecified price: %s, want inclusive", got)
	}
}

// Mixed behaviours on one invoice tax each group at its own rate object.
func TestStandardRatedTaxMixedBehaviors(t *testing.T) {
	h := seededTaxHandler(t, stripeTaxSeed)
	taxed := h.automaticTaxes("CH", "usd", []taxLine{{46800, "exclusive"}, {46800, "inclusive"}})
	if len(taxed.totalTaxes) != 2 || taxed.exclusiveTax != 3791 || taxed.inclusiveTax != 3507 {
		t.Fatalf("totals %+v exclusive %d inclusive %d", taxed.totalTaxes, taxed.exclusiveTax, taxed.inclusiveTax)
	}
	if taxed.lineTaxes[0][0].TaxBehavior != "exclusive" || taxed.lineTaxes[1][0].TaxBehavior != "inclusive" {
		t.Errorf("line behaviours %+v", taxed.lineTaxes)
	}
	if taxed.totalTaxes[0].TaxRateDetails.TaxRate == taxed.totalTaxes[1].TaxRateDetails.TaxRate {
		t.Error("exclusive and inclusive share a rate object; Stripe's rate carries inclusive")
	}
}

// Quirk crud-014: a country with no registration, or no location, gets one zero
// entry per line and in total, not_collecting, exclusive.
func TestStandardRatedTaxNotCollecting(t *testing.T) {
	h := seededTaxHandler(t, stripeTaxSeed)
	for _, country := range []string{"US", "DE", ""} {
		taxed := h.automaticTaxes(country, "usd", []taxLine{{46800, "exclusive"}, {1000, "unspecified"}})
		entries := append([]store.InvoiceTax{}, taxed.totalTaxes...)
		for _, lt := range taxed.lineTaxes {
			entries = append(entries, lt...)
		}
		if len(taxed.totalTaxes) != 1 || len(entries) != 3 {
			t.Fatalf("%q: total %+v lines %+v", country, taxed.totalTaxes, taxed.lineTaxes)
		}
		for _, e := range entries {
			if e.Amount != 0 || e.TaxableAmount != 0 || e.TaxabilityReason != "not_collecting" || e.TaxBehavior != "exclusive" || e.TaxRateDetails.TaxRate == "" {
				t.Errorf("%q: entry %+v", country, e)
			}
		}
		if taxed.exclusiveTax != 0 || taxed.inclusiveTax != 0 {
			t.Errorf("%q: tax %d/%d, want none", country, taxed.exclusiveTax, taxed.inclusiveTax)
		}
	}
}

// The shipping address locates the customer before the billing address.
func TestStandardRatedTaxCustomerCountry(t *testing.T) {
	ch, us, lower := "CH", "US", "ch"
	for _, c := range []struct {
		name string
		cus  store.Customer
		want string
	}{
		{"none", store.Customer{}, ""},
		{"address", store.Customer{Address: &store.Address{Country: &ch}}, "CH"},
		{"lower case", store.Customer{Address: &store.Address{Country: &lower}}, "CH"},
		{"shipping first", store.Customer{Address: &store.Address{Country: &us}, Shipping: &store.Shipping{Address: &store.Address{Country: &ch}}}, "CH"},
		{"shipping without country", store.Customer{Address: &store.Address{Country: &us}, Shipping: &store.Shipping{Address: &store.Address{}}}, "US"},
	} {
		if got := customerTaxCountry(c.cus); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func serveTaxRate(h *Handler, id string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/v1/tax_rates/"+id, nil)
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", id)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
	w := httptest.NewRecorder()
	h.GetTaxRate(w, r)
	return w
}

// Quirk crud-015: Stripe Tax creates one rate per country, rate and behaviour,
// reuses it, serves it by ID and leaves it out of GET /v1/tax_rates.
func TestStripeTaxRatesReusedRetrievableNotListed(t *testing.T) {
	h := seededTaxHandler(t, stripeTaxSeed)
	first := h.automaticTaxes("CH", "usd", []taxLine{{46800, "exclusive"}}).totalTaxes[0].TaxRateDetails.TaxRate
	again := h.automaticTaxes("CH", "usd", []taxLine{{1006, "exclusive"}}).totalTaxes[0].TaxRateDetails.TaxRate
	if first == "" || first != again {
		t.Fatalf("rate ids %q and %q, want one reused rate", first, again)
	}

	w := serveTaxRate(h, first)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /v1/tax_rates/%s: %d %s", first, w.Code, w.Body)
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	want := map[string]any{
		"object": "tax_rate", "country": "CH", "jurisdiction": "Switzerland", "jurisdiction_level": "country",
		"tax_type": "vat", "display_name": "VAT", "percentage": 8.1, "effective_percentage": 8.1,
		"rate_type": "percentage", "inclusive": false, "active": false,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}

	lw := httptest.NewRecorder()
	h.ListTaxRates(lw, httptest.NewRequest(http.MethodGet, "/v1/tax_rates?limit=100", nil))
	var list struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.Unmarshal(lw.Body.Bytes(), &list)
	for _, tr := range list.Data {
		if tr["id"] == first {
			t.Errorf("GET /v1/tax_rates lists the Stripe-created rate %s", first)
		}
	}

	// The rates survive a state round trip and stay out of the list.
	b, _ := json.Marshal(h.store.Snapshot())
	s2 := store.New()
	if err := s2.LoadState(b); err != nil {
		t.Fatal(err)
	}
	h2 := NewHandler(s2, nil, nil)
	if w := serveTaxRate(h2, first); w.Code != http.StatusOK {
		t.Errorf("after state load, GET /v1/tax_rates/%s: %d", first, w.Code)
	}
	if s2.TaxRates.Count() != 0 {
		t.Errorf("state load put %d rates in the listed store", s2.TaxRates.Count())
	}
}
