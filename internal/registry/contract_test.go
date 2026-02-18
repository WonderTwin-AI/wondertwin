package registry

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestContractGoldenRegistryParseable loads the golden registry.json and verifies
// the registry client can parse every field. If anyone changes a struct tag, this breaks.
func TestContractGoldenRegistryParseable(t *testing.T) {
	data, err := os.ReadFile("testdata/registry.json")
	if err != nil {
		t.Fatalf("reading golden registry: %v", err)
	}

	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatalf("parsing golden registry: %v", err)
	}

	if reg.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", reg.SchemaVersion)
	}
	if len(reg.Twins) != 2 {
		t.Fatalf("expected 2 twins, got %d", len(reg.Twins))
	}

	// Verify stripe entry
	stripe, ok := reg.Twins["stripe"]
	if !ok {
		t.Fatal("missing twin 'stripe'")
	}
	if stripe.Description == "" {
		t.Error("stripe description is empty")
	}
	if stripe.Repo == "" {
		t.Error("stripe repo is empty")
	}
	if stripe.Category == "" {
		t.Error("stripe category is empty")
	}
	if stripe.Author == "" {
		t.Error("stripe author is empty")
	}
	if stripe.Latest != "0.1.0" {
		t.Errorf("stripe latest = %q, want %q", stripe.Latest, "0.1.0")
	}

	// Verify latest points to an actual version
	ver, ok := stripe.Versions[stripe.Latest]
	if !ok {
		t.Fatalf("stripe latest %q not found in versions", stripe.Latest)
	}

	// Verify version fields
	if ver.Released == "" {
		t.Error("released is empty")
	}
	if ver.SDKPackage != "github.com/stripe/stripe-go" {
		t.Errorf("sdk_package = %q", ver.SDKPackage)
	}
	if ver.SDKVersion == "" {
		t.Error("sdk_version is empty")
	}
	if ver.APIVersion != "2024-12-18" {
		t.Errorf("api_version = %q, want %q", ver.APIVersion, "2024-12-18")
	}
	if ver.Tier != "free" {
		t.Errorf("tier = %q", ver.Tier)
	}

	// Verify all 4 platforms exist in binary_urls and checksums
	platforms := []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64"}
	for _, p := range platforms {
		if _, ok := ver.BinaryURLs[p]; !ok {
			t.Errorf("missing binary_url for platform %s", p)
		}
		cs, ok := ver.Checksums[p]
		if !ok {
			t.Errorf("missing checksum for platform %s", p)
			continue
		}
		// Verify checksum format matches what installer.go expects: "sha256:<hex>"
		if !strings.HasPrefix(cs, "sha256:") {
			t.Errorf("checksum for %s doesn't start with 'sha256:': %q", p, cs)
		}
		hex := strings.TrimPrefix(cs, "sha256:")
		if len(hex) != 64 {
			t.Errorf("checksum hex for %s has length %d, want 64", p, len(hex))
		}
	}

	// Verify twilio entry also parses
	twilio, ok := reg.Twins["twilio"]
	if !ok {
		t.Fatal("missing twin 'twilio'")
	}
	if twilio.Latest == "" {
		t.Error("twilio latest is empty")
	}
	if _, ok := twilio.Versions[twilio.Latest]; !ok {
		t.Errorf("twilio latest %q not found in versions", twilio.Latest)
	}
}

// TestContractVersionResolution verifies that ResolveVersion works with the golden registry.
func TestContractVersionResolution(t *testing.T) {
	data, err := os.ReadFile("testdata/registry.json")
	if err != nil {
		t.Fatalf("reading golden registry: %v", err)
	}

	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatalf("parsing golden registry: %v", err)
	}

	// "latest" resolves correctly
	v, ver, err := reg.ResolveVersion("stripe", "latest")
	if err != nil {
		t.Fatalf("ResolveVersion(stripe, latest): %v", err)
	}
	if v != "0.1.0" {
		t.Errorf("latest resolved to %q, want %q", v, "0.1.0")
	}
	if ver.SDKPackage == "" {
		t.Error("resolved version has empty sdk_package")
	}

	// Exact version resolves correctly
	v2, _, err := reg.ResolveVersion("stripe", "0.1.0")
	if err != nil {
		t.Fatalf("ResolveVersion(stripe, 0.1.0): %v", err)
	}
	if v2 != "0.1.0" {
		t.Errorf("exact version resolved to %q", v2)
	}

	// Non-existent version fails
	_, _, err = reg.ResolveVersion("stripe", "99.99.99")
	if err == nil {
		t.Error("expected error for non-existent version")
	}

	// Non-existent twin fails
	_, _, err = reg.ResolveVersion("nonexistent", "latest")
	if err == nil {
		t.Error("expected error for non-existent twin")
	}
}

// TestContractGenRegistryOutputParseable verifies that gen-registry's output format
// is consumable by the registry client. This uses the same JSON structure that
// gen-registry produces.
func TestContractGenRegistryOutputParseable(t *testing.T) {
	// Simulate gen-registry output format (same struct field names and JSON tags)
	genOutput := `{
  "schema_version": 1,
  "twins": {
    "test-twin": {
      "description": "Test twin",
      "repo": "https://github.com/wondertwin-ai/wondertwin",
      "category": "testing",
      "author": "WonderTwin",
      "latest": "0.1.0",
      "versions": {
        "0.1.0": {
          "released": "2026-02-17",
          "sdk_package": "github.com/test/test-go",
          "sdk_version": "v1",
          "api_version": "2024-01-01",
          "tier": "free",
          "checksums": {
            "darwin-amd64": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
            "darwin-arm64": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
            "linux-amd64": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
            "linux-arm64": "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
          },
          "binary_urls": {
            "darwin-amd64": "https://github.com/wondertwin-ai/registry/releases/download/twin-test-twin-v0.1.0/twin-test-twin-darwin-amd64",
            "darwin-arm64": "https://github.com/wondertwin-ai/registry/releases/download/twin-test-twin-v0.1.0/twin-test-twin-darwin-arm64",
            "linux-amd64": "https://github.com/wondertwin-ai/registry/releases/download/twin-test-twin-v0.1.0/twin-test-twin-linux-amd64",
            "linux-arm64": "https://github.com/wondertwin-ai/registry/releases/download/twin-test-twin-v0.1.0/twin-test-twin-linux-arm64"
          }
        }
      }
    }
  }
}`

	var reg Registry
	if err := json.Unmarshal([]byte(genOutput), &reg); err != nil {
		t.Fatalf("registry client cannot parse gen-registry output: %v", err)
	}

	entry := reg.Twins["test-twin"]
	if entry.Latest != "0.1.0" {
		t.Errorf("latest = %q", entry.Latest)
	}
	ver := entry.Versions["0.1.0"]
	if ver.SDKPackage != "github.com/test/test-go" {
		t.Errorf("sdk_package = %q", ver.SDKPackage)
	}
	if ver.APIVersion != "2024-01-01" {
		t.Errorf("api_version = %q", ver.APIVersion)
	}
	if len(ver.BinaryURLs) != 4 {
		t.Errorf("expected 4 binary_urls, got %d", len(ver.BinaryURLs))
	}
	if len(ver.Checksums) != 4 {
		t.Errorf("expected 4 checksums, got %d", len(ver.Checksums))
	}
}
