package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

// orgHashSalt rotates with breaking telemetry-format revisions. It is
// concatenated into the SHA-256 input so org hashes are not directly
// comparable across major schema versions; bump together with
// SchemaVersion when the event shape changes incompatibly.
const orgHashSalt = "wondertwin/v1"

// orgHashSourceVars enumerates, in priority order, the environment
// variables consulted by OrgHash before falling back to the hostname.
var orgHashSourceVars = []string{
	"GITHUB_REPOSITORY_OWNER",
	"CI_PROJECT_NAMESPACE",
	"BUILDKITE_ORGANIZATION_SLUG",
	"CIRCLE_PROJECT_USERNAME",
}

// OrgHash returns an opaque, deterministic identifier for the
// customer organization. It is the SHA-256 of the salted first
// non-empty value among:
//
//   - GITHUB_REPOSITORY_OWNER
//   - CI_PROJECT_NAMESPACE        (GitLab)
//   - BUILDKITE_ORGANIZATION_SLUG
//   - CIRCLE_PROJECT_USERNAME
//   - the host name (os.Hostname)
//   - the literal string "anonymous"
//
// The hash is truncated to the first 16 hex characters (64 bits) for
// compactness; collision probability is negligible at WonderTwin
// scale (millions of orgs ≪ 2^32 birthday limit). The hash is opaque
// server-side: sales correlation happens via separately-issued
// license org_id values, never by reversing this hash.
func OrgHash() string {
	return OrgHashFromEnv(os.Getenv, hostname)
}

// OrgHashFromEnv is the testable variant of OrgHash. getenv is
// expected to mirror os.Getenv; hostname is expected to mirror
// os.Hostname (returning value, ok).
func OrgHashFromEnv(getenv func(string) string, hostname func() (string, bool)) string {
	src := pickOrgSource(getenv, hostname)
	h := sha256.Sum256([]byte(orgHashSalt + ":" + src))
	return hex.EncodeToString(h[:])[:16]
}

func pickOrgSource(getenv func(string) string, hostname func() (string, bool)) string {
	for _, v := range orgHashSourceVars {
		if got := getenv(v); got != "" {
			return got
		}
	}
	if name, ok := hostname(); ok && name != "" {
		return name
	}
	return "anonymous"
}

func hostname() (string, bool) {
	name, err := os.Hostname()
	if err != nil {
		return "", false
	}
	return name, true
}
