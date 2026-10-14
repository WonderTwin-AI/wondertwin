package main

import (
	"testing"
)

// The published checksums of slack 0.1.0, and the release digest and id
// they give (internal/releaseid's golden vector).
var slackChecksums = map[string]string{
	"darwin-amd64": "sha256:c4b3f1cf32cd0645d8f33639f3435f0b79c13651cb4e261e25ee1f304af7437c",
	"darwin-arm64": "sha256:e1fe69325e118a844c36d5d57c13200b371c41471c0631bde772de20e6d5ff74",
	"linux-amd64":  "sha256:2165f4f9796e9d3ba0ad71dc5c9ae83f77da2cfba0f146e60069c2fa254c6ec6",
	"linux-arm64":  "sha256:df4858846d3ba27c14cd4f0c3d53572038006833630b5cdcb838a29284254ca2",
}

const (
	slackDigest = "sha256:56de41a05a98e18df036ad9780c735a642928227fec8efe2361ac06978df1eeb"
	slackID     = "56de41a05a98"
)

// slackRegistry is a public registry with one slack version under key,
// carrying the published slack checksums and the given digest.
func slackRegistry(key, digest string) registrySchema {
	return registrySchema{
		SchemaVersion: 2,
		Twins: map[string]twinEntry{
			"slack": {
				Description: "Slack twin",
				Repo:        "https://github.com/wondertwin-ai/wondertwin",
				Tier:        "community",
				Latest:      key,
				Versions: map[string]versionDef{
					key: {
						Released:      "2026-10-13",
						Tier:          "free",
						Checksums:     slackChecksums,
						BinaryURLs:    map[string]string{},
						ReleaseDigest: digest,
					},
				},
			},
		},
	}
}

func assertAllPass(t *testing.T, results []checkResult) {
	t.Helper()
	for _, r := range results {
		if !r.Passed {
			t.Errorf("check %q failed: %s", r.Name, r.Detail)
		}
	}
}

func TestPublicAcceptsReleaseID(t *testing.T) {
	results := runWithPolicy(serveRegistry(t, slackRegistry(slackID, slackDigest)), true)
	assertAllPass(t, results)
}

func TestPublicAcceptsEveryLegacyRelease(t *testing.T) {
	reg := registrySchema{SchemaVersion: 2, Twins: map[string]twinEntry{}}
	for name, versions := range legacyReleases {
		entry := twinEntry{Tier: "community", Latest: versions[len(versions)-1], Versions: map[string]versionDef{}}
		for _, v := range versions {
			entry.Versions[v] = versionDef{Tier: "free", Checksums: slackChecksums, BinaryURLs: map[string]string{}}
		}
		reg.Twins[name] = entry
	}
	assertAllPass(t, runWithPolicy(serveRegistry(t, reg), true))
}

func TestPublicRejectsBadReleaseIDs(t *testing.T) {
	otherDigest := "sha256:" + "0123456789ab" + slackDigest[len("sha256:")+12:]
	for _, tc := range []struct {
		name   string
		key    string
		digest string
		check  string
	}{
		{"missing digest", slackID, "", "[slack@" + slackID + "] release_digest"},
		{"malformed digest", slackID, "sha256:abc", "[slack@" + slackID + "] release_digest"},
		{"key does not match digest", "0123456789ab", slackDigest, "[slack@0123456789ab] release id matches digest"},
		{"digest does not match checksums", "0123456789ab", otherDigest, "[slack@0123456789ab] release_digest matches checksums"},
	} {
		results := runWithPolicy(serveRegistry(t, slackRegistry(tc.key, tc.digest)), true)
		if failedCheck(results, tc.check) == nil {
			t.Errorf("%s: expected %q to fail", tc.name, tc.check)
		}
	}
}

func TestPublicRejectsNewSemver(t *testing.T) {
	results := runWithPolicy(serveRegistry(t, slackRegistry("0.2.0", "")), true)
	if failedCheck(results, "[slack@0.2.0] version key") == nil {
		t.Error("expected a new semver release to fail the version key check")
	}
}

func TestNonPublicPolicyIgnoresVersionKeys(t *testing.T) {
	results := runWithPolicy(serveRegistry(t, slackRegistry("0.2.0", "")), false)
	assertAllPass(t, results)
}
