package cimode

import "time"

// Stable Reason strings emitted by ValidateLicense. These values are
// part of the public contract: they appear in `wt license status`
// output and (in a follow-up) in telemetry payloads.
const (
	ReasonNoLicense       = "no license"
	ReasonInvalidSig      = "invalid signature"
	ReasonExpired         = "license expired"
	ReasonTwinOutOfScope  = "twin not in scope"
)

// Status is the result of ValidateLicense.
type Status struct {
	// Valid is true when the license is structurally sound, signed by
	// the active verification key, not expired, and covers the
	// requested twin.
	Valid bool

	// Reason is one of the Reason* constants above when Valid is
	// false; "" when Valid is true.
	Reason string

	// ExpiresIn is positive when the license is still valid (time
	// remaining until NotAfter), negative when expired, and zero when
	// no license is present.
	ExpiresIn time.Duration
}

// ValidateLicense reports whether lic authorizes twinName at the given
// clock. A nil license is the soft-gate path: Valid=false,
// Reason=ReasonNoLicense.
//
// The check order is signature → expiry → scope. The first failing
// check determines the reported Reason.
func ValidateLicense(lic *License, twinName string, now time.Time) Status {
	if lic == nil {
		return Status{Reason: ReasonNoLicense}
	}
	if !verifySignature(*lic) {
		return Status{Reason: ReasonInvalidSig}
	}
	if now.After(lic.NotAfter) {
		return Status{Reason: ReasonExpired, ExpiresIn: lic.NotAfter.Sub(now)}
	}
	if !scopeIncludes(lic.TwinScope, twinName) {
		return Status{Reason: ReasonTwinOutOfScope, ExpiresIn: lic.NotAfter.Sub(now)}
	}
	return Status{Valid: true, ExpiresIn: lic.NotAfter.Sub(now)}
}

func scopeIncludes(scope []string, twinName string) bool {
	for _, s := range scope {
		if s == "*" || s == twinName {
			return true
		}
	}
	return false
}
