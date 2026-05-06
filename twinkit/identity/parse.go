package identity

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidQualified is returned when a qualified-form string does not
// match the "tier:publisher/vendor" grammar.
var ErrInvalidQualified = errors.New("identity: invalid qualified twin form")

// TwinID returns the directory/binary identity for this twin.
func (t Twin) TwinID() TwinID {
	return TwinID("twin-" + t.Vendor)
}

// String renders as "tier:publisher/vendor", the canonical qualified
// form used in wondertwin.json keys, error messages, log lines, and
// license entitlements.
func (t Twin) String() string {
	return string(t.Tier) + ":" + string(t.Publisher) + "/" + string(t.Vendor)
}

// Validate checks each dimension against its pattern. Returns an error
// describing the first failure.
func (t Twin) Validate() error {
	if !tierRE.MatchString(string(t.Tier)) {
		return fmt.Errorf("identity: tier %q does not match %s", t.Tier, TierPattern)
	}
	if !publisherRE.MatchString(string(t.Publisher)) {
		return fmt.Errorf("identity: publisher %q does not match %s", t.Publisher, PublisherPattern)
	}
	if !vendorRE.MatchString(string(t.Vendor)) {
		return fmt.Errorf("identity: vendor %q does not match %s", t.Vendor, VendorPattern)
	}
	return nil
}

// ParseQualified accepts the canonical "tier:publisher/vendor" form
// (e.g. "commercial:wondertwin/stripe") and returns the Twin.
//
// Bare names ("stripe") and partially-qualified forms are rejected;
// resolution of bare or partial inputs to a full Twin is the
// responsibility of higher-level CLI code that has access to the
// installed-binary inventory and license state.
func ParseQualified(s string) (Twin, error) {
	if !qualifiedRE.MatchString(s) {
		return Twin{}, fmt.Errorf("%w: %q (expected tier:publisher/vendor)", ErrInvalidQualified, s)
	}
	colon := strings.IndexByte(s, ':')
	slash := strings.IndexByte(s, '/')
	return Twin{
		Tier:      Tier(s[:colon]),
		Publisher: Publisher(s[colon+1 : slash]),
		Vendor:    VendorName(s[slash+1:]),
	}, nil
}

// ParseTwinID validates and returns a TwinID from a string.
func ParseTwinID(s string) (TwinID, error) {
	if !twinIDRE.MatchString(s) {
		return "", fmt.Errorf("identity: %q does not match %s", s, TwinIDPattern)
	}
	return TwinID(s), nil
}
