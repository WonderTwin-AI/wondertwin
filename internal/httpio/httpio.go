// Package httpio defines small HTTP-related constants shared across
// the CLI. The point is to centralize the per-response body cap so a
// future change (raise the cap, vendor a real reader, etc.) is one
// edit instead of one per call site.
package httpio

// MaxResponseBytes is the upper bound on a single HTTP response body
// the CLI is willing to read into memory. Binary downloads (twin
// installers) are bounded by this; JSON responses are much smaller
// in practice but share the same ceiling for consistency.
//
// 100 MiB is comfortable above any realistic twin binary while still
// preventing OOM if a malicious or misbehaving server returns a 50
// GiB body. The 2026-06-04 audit's "unbounded io.ReadAll" finding
// was about every HTTP body in the CLI not being bounded at all;
// even a high ceiling like this closes that gap.
const MaxResponseBytes = 100 << 20
