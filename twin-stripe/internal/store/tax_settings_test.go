package store

import (
	"encoding/json"
	"testing"
)

// taxSeed is the seed contract: tax settings and a CH registration in Stripe's
// shapes, plus the registration's declared standard rate.
const taxSeed = `{
  "tax_settings": {"object": "tax.settings", "status": "active", "head_office": {"address": {"country": "CH"}},
    "defaults": {"provider": "stripe", "tax_behavior": "inferred_by_currency", "tax_code": "txcd_10000000"}},
  "tax_registrations": {"taxreg_seed_ch": {"id": "taxreg_seed_ch", "object": "tax.registration", "country": "CH",
    "country_options": {"ch": {"type": "standard"}}, "status": "active", "active_from": 1794700000,
    "standard_rate": {"percentage": 8.1, "tax_type": "vat", "display_name": "VAT"}}}
}`

func TestTaxSettingsStateLoadsTheSeed(t *testing.T) {
	s := New()
	if s.TaxActive() {
		t.Fatal("a new store has tax active; it must be off until seeded")
	}
	if err := s.LoadState([]byte(taxSeed)); err != nil {
		t.Fatal(err)
	}
	if !s.TaxActive() {
		t.Fatal("seeded tax settings are not active")
	}
	reg, ok := s.TaxRegistrations.Get("taxreg_seed_ch")
	if !ok || reg.Country != "CH" || reg.StandardRate == nil || reg.StandardRate.Percentage != 8.1 || reg.StandardRate.TaxType != "vat" {
		t.Fatalf("registration not loaded as seeded: %+v", reg)
	}
}

func TestTaxSettingsStateRoundTripsAndResets(t *testing.T) {
	s := New()
	if err := s.LoadState([]byte(taxSeed)); err != nil {
		t.Fatal(err)
	}
	exported, err := json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(exported, &out); err != nil {
		t.Fatal(err)
	}
	if out["tax_settings"].(map[string]any)["status"] != "active" {
		t.Errorf("exported tax_settings = %v", out["tax_settings"])
	}
	if _, ok := out["tax_registrations"].(map[string]any)["taxreg_seed_ch"]; !ok {
		t.Errorf("exported tax_registrations = %v", out["tax_registrations"])
	}

	again := New()
	if err := again.LoadState(exported); err != nil || !again.TaxActive() {
		t.Fatalf("exported state does not load back as active (err %v)", err)
	}

	s.Reset()
	if s.TaxActive() || s.TaxRegistrations.Count() != 0 {
		t.Errorf("reset left tax state behind: active=%v registrations=%d", s.TaxActive(), s.TaxRegistrations.Count())
	}
}

func TestTaxSettingsStateNeedsActiveHeadOffice(t *testing.T) {
	for name, seed := range map[string]string{
		"pending":        `{"tax_settings": {"object": "tax.settings", "status": "pending", "head_office": {"address": {"country": "CH"}}}}`,
		"no head office": `{"tax_settings": {"object": "tax.settings", "status": "active"}}`,
		"no country":     `{"tax_settings": {"object": "tax.settings", "status": "active", "head_office": {"address": {}}}}`,
	} {
		s := New()
		if err := s.LoadState([]byte(seed)); err != nil {
			t.Fatal(err)
		}
		if s.TaxActive() {
			t.Errorf("%s: tax active, want off", name)
		}
	}
}
