// Package identity defines the typed identity model for twins.
//
// A twin's identity has four orthogonal dimensions, each previously
// conflated as a bare string field at runtime:
//
//   - Tier:      community vs commercial (license-enforcement layer)
//   - Publisher: who authored and ships the twin (e.g. "wondertwin")
//   - Vendor:    the vendor's name in the world (e.g. "stripe")
//   - TwinID:    the directory/binary identity within a tier+publisher
//     scope (always "twin-" + Vendor)
//
// The fully-qualified identity is a Twin: a (Tier, Publisher, Vendor)
// tuple. TwinID alone is insufficient outside of a tier+publisher-scoped
// context; manifests, license entitlements, replay artifacts, telemetry,
// and wondertwin.json keys carry the full Twin.
//
// Canonical qualified form is "tier:publisher/vendor", e.g.
// "commercial:wondertwin/stripe". This package owns parsing, formatting,
// and the regex patterns for each dimension as the single source of truth.
//
// References:
//   - wondertwin-docs/thinking/explorations/twin-naming-design.md (adopted)
//   - wondertwin-docs/thinking/explorations/twin-versioning-design.md (adopted)
package identity
