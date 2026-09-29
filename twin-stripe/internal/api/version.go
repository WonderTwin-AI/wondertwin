package api

import (
	"fmt"
	"net/http"
	"strings"
)

// DefaultAPIVersion is the Stripe API version the app emulator serves and the
// account default when none is configured: the version Stripe's docs name
// current and every current server SDK pins (stripe-go v86, stripe-node v22,
// stripe-python v15).
const DefaultAPIVersion = "2026-08-26.dahlia"

// servedVersions are the dahlia monthly versions. Monthly versions inside one
// release name carry no breaking change, so a client on any of them is served
// compatibly by the 2026-08-26.dahlia shape.
var servedVersions = []string{
	"2026-03-25.dahlia",
	"2026-04-22.dahlia",
	"2026-05-27.dahlia",
	"2026-06-24.dahlia",
	"2026-07-29.dahlia",
	"2026-08-26.dahlia",
}

// IsServedVersion reports whether v is a dahlia version this app emulator
// serves.
func IsServedVersion(v string) bool {
	for _, s := range servedVersions {
		if s == v {
			return true
		}
	}
	return false
}

func servedRange() string {
	return servedVersions[0] + " to " + servedVersions[len(servedVersions)-1]
}

// SetDefaultAPIVersion sets the emulated account's default API version, the
// version a request without Stripe-Version is served in and events are
// rendered in. Stripe sets it on the account's first request; here the
// account exists before any request, so it is configuration.
func (h *Handler) SetDefaultAPIVersion(v string) error {
	if !IsServedVersion(v) {
		return fmt.Errorf("unsupported Stripe API version %q: this app emulator serves the dahlia release (%s)", v, servedRange())
	}
	h.apiVersion = v
	return nil
}

// DefaultVersion returns the emulated account's default API version.
func (h *Handler) DefaultVersion() string {
	if h.apiVersion == "" {
		return DefaultAPIVersion
	}
	return h.apiVersion
}

// versionMiddleware refuses any Stripe-Version outside the dahlia release.
// Stripe would serve an older release in its own shape; serving it the
// dahlia shape silently would be harder to diagnose than a 400, so this is a
// declared divergence (twin-stripe/divergences.json).
func (h *Handler) versionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := strings.TrimSpace(r.Header.Get("Stripe-Version"))
		if v != "" && !IsServedVersion(v) {
			writeError(w, http.StatusBadRequest, apiError{
				Type: "invalid_request_error",
				Message: "Invalid Stripe API version: " + v + ". This app emulator serves the dahlia release (" +
					servedRange() + "), in the shape of " + DefaultAPIVersion + ".",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
