package telemetry

import (
	"strings"
	"testing"
)

func envFunc(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestOrgHashPrecedence(t *testing.T) {
	t.Parallel()
	gh := OrgHashFromEnv(envFunc(map[string]string{
		"GITHUB_REPOSITORY_OWNER":     "acme-gh",
		"CI_PROJECT_NAMESPACE":        "acme-gl",
		"BUILDKITE_ORGANIZATION_SLUG": "acme-bk",
		"CIRCLE_PROJECT_USERNAME":     "acme-cc",
	}), func() (string, bool) { return "host", true })

	gl := OrgHashFromEnv(envFunc(map[string]string{
		"CI_PROJECT_NAMESPACE":        "acme-gl",
		"BUILDKITE_ORGANIZATION_SLUG": "acme-bk",
	}), func() (string, bool) { return "host", true })

	bk := OrgHashFromEnv(envFunc(map[string]string{
		"BUILDKITE_ORGANIZATION_SLUG": "acme-bk",
	}), func() (string, bool) { return "host", true })

	cc := OrgHashFromEnv(envFunc(map[string]string{
		"CIRCLE_PROJECT_USERNAME": "acme-cc",
	}), func() (string, bool) { return "host", true })

	host := OrgHashFromEnv(envFunc(nil), func() (string, bool) { return "host", true })

	if gh == gl || gl == bk || bk == cc || cc == host {
		t.Errorf("expected distinct hashes per priority tier; got %s/%s/%s/%s/%s",
			gh, gl, bk, cc, host)
	}
}

func TestOrgHashFallsThroughToAnonymous(t *testing.T) {
	t.Parallel()
	got := OrgHashFromEnv(envFunc(nil), func() (string, bool) { return "", false })
	want := OrgHashFromEnv(envFunc(map[string]string{}), func() (string, bool) { return "", false })
	if got != want {
		t.Errorf("anonymous fallback non-deterministic: %s vs %s", got, want)
	}
	if len(got) != 16 || !isHex(got) {
		t.Errorf("hash should be 16 hex chars; got %q", got)
	}
}

func TestOrgHashIsDeterministic(t *testing.T) {
	t.Parallel()
	a := OrgHashFromEnv(envFunc(map[string]string{"GITHUB_REPOSITORY_OWNER": "acme"}), func() (string, bool) { return "x", true })
	b := OrgHashFromEnv(envFunc(map[string]string{"GITHUB_REPOSITORY_OWNER": "acme"}), func() (string, bool) { return "x", true })
	if a != b {
		t.Errorf("hash non-deterministic: %s vs %s", a, b)
	}
}

func TestOrgHashLengthAndCharset(t *testing.T) {
	t.Parallel()
	got := OrgHash()
	if len(got) != 16 {
		t.Errorf("length = %d, want 16", len(got))
	}
	if !isHex(got) {
		t.Errorf("non-hex chars in %q", got)
	}
}

func isHex(s string) bool {
	const hex = "0123456789abcdef"
	for _, c := range s {
		if !strings.ContainsRune(hex, c) {
			return false
		}
	}
	return true
}
