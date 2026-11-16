package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

const activeTaxSeed = `{"tax_settings": {"object": "tax.settings", "status": "active", "head_office": {"address": {"country": "CH"}}}}`

func gateRequest(t *testing.T, h *Handler, body string, checkout bool) (bool, *httptest.ResponseRecorder) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/v1/subscriptions", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	return h.requireTaxSetUp(w, r, checkout), w
}

// The refusal Stripe answered on 2026-11-15 (quirk error-006): 400
// invalid_request_error with no code and no param. Stripe also sends an
// account-specific request_log_url, which the app emulator omits on every error.
func TestAutomaticTaxGateRefusesWhenTaxIsNotSetUp(t *testing.T) {
	h := NewHandler(store.New(), nil, nil)
	for _, c := range []struct {
		checkout bool
		message  string
	}{
		{false, "You must have a valid head office address to enable automatic tax calculation in test mode. Visit https://dashboard.stripe.com/test/settings/tax to update it."},
		{true, "You must have a valid head office address to enable automatic tax calculation in test mode.  Visit https://dashboard.stripe.com/test/settings/tax to update it."},
	} {
		ok, w := gateRequest(t, h, "customer=cus_1&automatic_tax[enabled]=true", c.checkout)
		if ok || w.Code != http.StatusBadRequest {
			t.Fatalf("checkout=%v: ok=%v status=%d, want refused with 400", c.checkout, ok, w.Code)
		}
		var body map[string]map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"type": "invalid_request_error", "message": c.message}
		if len(body["error"]) != len(want) {
			t.Errorf("checkout=%v: error has fields %v, want only type and message", c.checkout, body["error"])
		}
		for k, v := range want {
			if body["error"][k] != v {
				t.Errorf("checkout=%v: error.%s = %q, want %q", c.checkout, k, body["error"][k], v)
			}
		}
	}
}

func TestAutomaticTaxGatePassesWhenTaxIsSetUpOrNotAsked(t *testing.T) {
	off := NewHandler(store.New(), nil, nil)
	if ok, w := gateRequest(t, off, "customer=cus_1", false); !ok || w.Body.Len() != 0 {
		t.Errorf("no automatic_tax: refused (%s)", w.Body.String())
	}
	if ok, _ := gateRequest(t, off, "automatic_tax[enabled]=false", false); !ok {
		t.Error("automatic_tax[enabled]=false: refused")
	}

	s := store.New()
	if err := s.LoadState([]byte(activeTaxSeed)); err != nil {
		t.Fatal(err)
	}
	if ok, w := gateRequest(t, NewHandler(s, nil, nil), "automatic_tax[enabled]=true", true); !ok {
		t.Errorf("tax set up: refused (%s)", w.Body.String())
	}
}
