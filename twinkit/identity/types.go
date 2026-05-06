package identity

import "regexp"

// Tier is a twin's commercial layer. Community twins live in MIT and
// have no license enforcement; commercial twins live in BSL and require
// a valid license at startup.
//
// The schema regex is permissive (^[a-z]+$) for forward-compat with
// future tiers (enterprise, hosted) without a schema-breaking change.
type Tier string

const (
	TierCommunity  Tier = "community"
	TierCommercial Tier = "commercial"
)

// Publisher is the identity that authored and ships the twin.
// "wondertwin" is reserved for first-party WonderTwin-published twins.
// Third-party publishers use their own kebab-case identifier (analogous
// to npm scopes).
type Publisher string

const (
	PublisherWonderTwin Publisher = "wondertwin"
)

// VendorName is the vendor's identity in the world. Lowercase, no
// prefix, kebab-case for multi-word vendors.
type VendorName string

// TwinID is the directory/binary identity within a tier+publisher
// scope. Always "twin-" + VendorName.
type TwinID string

// Twin is the FULL identity: a (Tier, Publisher, Vendor) tuple.
// TwinID alone is insufficient outside of a tier+publisher-scoped
// context. Carry all three at any wire-format or persistence boundary.
type Twin struct {
	Tier      Tier
	Publisher Publisher
	Vendor    VendorName
}

// Regex patterns for each dimension. Exported as compiled regexps for
// use by schemas and external validators; the source-of-truth string
// patterns are the Pattern* constants.
const (
	// TierPattern is permissive for forward-compat. Concrete values
	// recognized today are TierCommunity and TierCommercial.
	TierPattern = `^[a-z]+$`

	// PublisherPattern is kebab-case starting with a letter.
	PublisherPattern = `^[a-z][a-z0-9-]*$`

	// VendorPattern is kebab-case starting with a letter.
	VendorPattern = `^[a-z][a-z0-9-]*$`

	// TwinIDPattern is "twin-" + VendorPattern.
	TwinIDPattern = `^twin-[a-z][a-z0-9-]*$`

	// QualifiedTwinPattern is the canonical qualified form
	// "tier:publisher/vendor". Permissive on tier for forward-compat.
	QualifiedTwinPattern = `^[a-z]+:[a-z][a-z0-9-]*/[a-z][a-z0-9-]*$`
)

var (
	tierRE      = regexp.MustCompile(TierPattern)
	publisherRE = regexp.MustCompile(PublisherPattern)
	vendorRE    = regexp.MustCompile(VendorPattern)
	twinIDRE    = regexp.MustCompile(TwinIDPattern)
	qualifiedRE = regexp.MustCompile(QualifiedTwinPattern)
)
