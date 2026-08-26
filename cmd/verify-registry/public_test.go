package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// serveRegistry publishes reg over httptest with reachable binary URLs and
// returns the registry URL, so public-policy tests fail on the rule under test
// rather than on unreachable downloads.
func serveRegistry(t *testing.T, reg registrySchema) string {
	t.Helper()

	binaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(binaryServer.Close)

	for _, entry := range reg.Twins {
		for _, vd := range entry.Versions {
			for _, p := range requiredPlatforms {
				vd.BinaryURLs[p] = binaryServer.URL + "/" + p
			}
		}
	}

	data, _ := json.Marshal(reg)
	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(data)
	}))
	t.Cleanup(regServer.Close)

	return regServer.URL
}

// commercialRegistry mirrors the shape of the twin-measure entry that reached
// the public registry on 2026-05-08: commercial tier, auth-gated download, and
// a repo pointing at wondertwin-pro.
func commercialRegistry() registrySchema {
	reg := validRegistry()
	entry := reg.Twins["stripe"]
	entry.Tier = "commercial"
	entry.DownloadAuth = "required"
	entry.Repo = "https://github.com/wondertwin-ai/wondertwin-pro"
	reg.Twins["stripe"] = entry
	return reg
}

func TestPublicRejectsCommercialTier(t *testing.T) {
	reg := validRegistry()
	entry := reg.Twins["stripe"]
	entry.Tier = "commercial"
	reg.Twins["stripe"] = entry

	results := runWithPolicy(serveRegistry(t, reg), true)
	if failedCheck(results, "[stripe] public tier") == nil {
		t.Error("expected a commercial-tier entry to fail the public gate")
	}
}

func TestPublicRejectsUnknownTier(t *testing.T) {
	// An allowlist, not a "commercial" denylist: a tier nobody has classified
	// yet must fail rather than ship silently.
	reg := validRegistry()
	entry := reg.Twins["stripe"]
	entry.Tier = "enterprise"
	reg.Twins["stripe"] = entry

	results := runWithPolicy(serveRegistry(t, reg), true)
	if failedCheck(results, "[stripe] public tier") == nil {
		t.Error("expected an unrecognised tier to fail the public gate")
	}
}

func TestPublicRejectsCommercialVersionTier(t *testing.T) {
	// The twin-level tier can be clean while a single version is not.
	reg := validRegistry()
	vd := reg.Twins["stripe"].Versions["0.1.0"]
	vd.Tier = "commercial"
	reg.Twins["stripe"].Versions["0.1.0"] = vd

	results := runWithPolicy(serveRegistry(t, reg), true)
	if failedCheck(results, "[stripe@0.1.0] public tier") == nil {
		t.Error("expected a commercial version tier to fail the public gate")
	}
}

func TestPublicRejectsDownloadAuth(t *testing.T) {
	reg := validRegistry()
	entry := reg.Twins["stripe"]
	entry.DownloadAuth = "required"
	reg.Twins["stripe"] = entry

	results := runWithPolicy(serveRegistry(t, reg), true)
	if failedCheck(results, "[stripe] public download_auth") == nil {
		t.Error("expected download_auth on a public entry to fail the public gate")
	}
}

func TestPublicRejectsNonPublicRepo(t *testing.T) {
	reg := validRegistry()
	entry := reg.Twins["stripe"]
	entry.Repo = "https://github.com/wondertwin-ai/wondertwin-pro"
	reg.Twins["stripe"] = entry

	results := runWithPolicy(serveRegistry(t, reg), true)
	if failedCheck(results, "[stripe] public repo") == nil {
		t.Error("expected a wondertwin-pro repo URL to fail the public gate")
	}
}

func TestPublicAcceptsFreeAndCommunityAndEmptyTiers(t *testing.T) {
	for _, tier := range []string{"", "free", "community"} {
		reg := validRegistry()
		entry := reg.Twins["stripe"]
		entry.Tier = tier
		entry.Repo = "https://github.com/wondertwin-ai/wondertwin"
		reg.Twins["stripe"] = entry

		results := runWithPolicy(serveRegistry(t, reg), true)
		if c := failedCheck(results, "[stripe] public tier"); c != nil {
			t.Errorf("tier %q should be publishable, got failure: %s", tier, c.Detail)
		}
		if failedCheck(results, "[stripe] public repo") != nil {
			t.Errorf("tier %q: public repo URL should pass", tier)
		}
	}
}

func TestNonPublicPolicyAllowsCommercialEntries(t *testing.T) {
	// The same registry validated as a commercial one must pass; these rules
	// are about where an entry is published, not whether it is well-formed.
	results := runWithPolicy(serveRegistry(t, commercialRegistry()), false)

	for _, name := range []string{"[stripe] public tier", "[stripe] public download_auth", "[stripe] public repo"} {
		if failedCheck(results, name) != nil {
			t.Errorf("%s should not be enforced when public=false", name)
		}
	}
	for _, r := range results {
		if !r.Passed {
			t.Errorf("commercial registry failed an unrelated check: %s — %s", r.Name, r.Detail)
		}
	}
}

func TestRunAcceptsLocalRegistryFile(t *testing.T) {
	// CI gates the registry.json proposed in a pull request, which is not
	// published at any URL yet, so a filesystem path must work as a source.
	binaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(binaryServer.Close)

	reg := validRegistry()
	vd := reg.Twins["stripe"].Versions["0.1.0"]
	for _, p := range requiredPlatforms {
		vd.BinaryURLs[p] = binaryServer.URL + "/" + p
	}
	reg.Twins["stripe"].Versions["0.1.0"] = vd

	data, _ := json.Marshal(reg)
	path := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	results := runWithPolicy(path, true)
	for _, r := range results {
		if !r.Passed {
			t.Errorf("local file check failed: %s — %s", r.Name, r.Detail)
		}
	}
}

func TestRunReportsMissingLocalRegistryFile(t *testing.T) {
	results := runWithPolicy(filepath.Join(t.TempDir(), "absent.json"), true)
	if failedCheck(results, "Fetch registry") == nil {
		t.Error("expected a missing registry file to fail the fetch check")
	}
}

func TestPublicPolicyCatchesTheMeasureRegression(t *testing.T) {
	// End to end on the real shape of the incident: all three rules fire.
	results := runWithPolicy(serveRegistry(t, commercialRegistry()), true)

	for _, name := range []string{"[stripe] public tier", "[stripe] public download_auth", "[stripe] public repo"} {
		if failedCheck(results, name) == nil {
			t.Errorf("expected %s to fail for a commercial entry in the public registry", name)
		}
	}
}
