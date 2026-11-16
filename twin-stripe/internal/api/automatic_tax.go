package api

import (
	"net/http"
)

// Stripe's refusal when automatic tax is asked of an account where Stripe Tax is
// not set up. It is not the documented stripe_tax_inactive code: Stripe answers
// 400 invalid_request_error with no code and no param, and Checkout's message
// has two spaces before "Visit" (quirk error-006, captured 2026-11-15).
const (
	taxNotSetUpMessage         = "You must have a valid head office address to enable automatic tax calculation in test mode. Visit https://dashboard.stripe.com/test/settings/tax to update it."
	taxNotSetUpCheckoutMessage = "You must have a valid head office address to enable automatic tax calculation in test mode.  Visit https://dashboard.stripe.com/test/settings/tax to update it."
)

// automaticTaxRequested reports whether the request sets automatic_tax[enabled]=true.
func automaticTaxRequested(r *http.Request) bool {
	return r.FormValue("automatic_tax[enabled]") == "true"
}

// requireTaxSetUp is the one check every route that accepts automatic_tax calls
// before anything else about tax: when the request asks for automatic tax and
// the account has no active tax settings, it writes Stripe's refusal and returns
// false. Checkout passes checkout=true for its own message.
func (h *Handler) requireTaxSetUp(w http.ResponseWriter, r *http.Request, checkout bool) bool {
	if !automaticTaxRequested(r) || h.store.TaxActive() {
		return true
	}
	msg := taxNotSetUpMessage
	if checkout {
		msg = taxNotSetUpCheckoutMessage
	}
	writeError(w, http.StatusBadRequest, apiError{Type: "invalid_request_error", Message: msg})
	return false
}
