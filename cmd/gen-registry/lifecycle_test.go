package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEmitsLifecycleFromFlags(t *testing.T) {
	dir := setupManifest(t)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)

	err := run([]string{
		"--twin", "stripe",
		"--version", "1.0.0",
		"--checksums-file", checksumsPath,
		"--registry-file", registryPath,
		"--tier", "commercial",
		"--release-type", "beta",
		"--maintenance-status", "security_only",
		"--maintenance-until", "2027-01-01",
		"--bsl-expiry-date", "2030-01-01",
		"--changelog", "initial release",
		"--breaking-change", "removed legacy auth",
		"--breaking-change", "renamed Charge.Source -> PaymentMethod",
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	data, _ := os.ReadFile(registryPath)
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatal(err)
	}

	if reg.SchemaVersion != 2 {
		t.Errorf("schema_version = %d, want 2", reg.SchemaVersion)
	}

	v := reg.Twins["stripe"].Versions["1.0.0"]
	if v.ReleaseType != "beta" {
		t.Errorf("release_type = %q, want beta", v.ReleaseType)
	}
	if v.MaintenanceStatus != "security_only" {
		t.Errorf("maintenance_status = %q", v.MaintenanceStatus)
	}
	if v.MaintenanceUntil != "2027-01-01" {
		t.Errorf("maintenance_until = %q", v.MaintenanceUntil)
	}
	if v.BSLExpiryDate != "2030-01-01" {
		t.Errorf("bsl_expiry_date = %q", v.BSLExpiryDate)
	}
	if v.Changelog != "initial release" {
		t.Errorf("changelog = %q", v.Changelog)
	}
	if len(v.BreakingChanges) != 2 {
		t.Errorf("breaking_changes len = %d", len(v.BreakingChanges))
	}
}

func TestRejectsInvalidReleaseType(t *testing.T) {
	dir := setupManifest(t)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)

	err := run([]string{
		"--twin", "stripe", "--version", "0.1.0",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
		"--release-type", "experimental",
	})
	if err == nil || !strings.Contains(err.Error(), "release_type") {
		t.Fatalf("expected release_type error, got %v", err)
	}
}

func TestRejectsInvalidDate(t *testing.T) {
	dir := setupManifest(t)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)

	err := run([]string{
		"--twin", "stripe", "--version", "0.1.0",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
		"--maintenance-until", "next-tuesday",
	})
	if err == nil || !strings.Contains(err.Error(), "maintenance_until") {
		t.Fatalf("expected maintenance_until error, got %v", err)
	}
}

func TestRejectsBSLOnFreeTier(t *testing.T) {
	dir := setupManifest(t)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)

	err := run([]string{
		"--twin", "stripe", "--version", "0.1.0",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
		"--tier", "community",
		"--bsl-expiry-date", "2030-01-01",
	})
	if err == nil || !strings.Contains(err.Error(), "commercial") {
		t.Fatalf("expected commercial-tier-only error, got %v", err)
	}
}

func TestManifestLifecycleSeedsEmittedVersion(t *testing.T) {
	dir := t.TempDir()
	twinDir := filepath.Join(dir, "twin-stripe")
	os.MkdirAll(twinDir, 0o755)
	manifest := `{
  "twin": "stripe",
  "description": "Stripe",
  "category": "payments",
  "sdk_target": {"primary": {"package": "github.com/stripe/stripe-go", "version": "v80"}},
  "lifecycle": {
    "release_type": "rc",
    "maintenance_status": "active",
    "breaking_changes": ["foo"]
  }
}`
	os.WriteFile(filepath.Join(twinDir, "twin-manifest.json"), []byte(manifest), 0o644)
	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	if err := run([]string{
		"--twin", "stripe", "--version", "0.1.0",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
	}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(registryPath)
	var reg Registry
	json.Unmarshal(data, &reg)
	v := reg.Twins["stripe"].Versions["0.1.0"]
	if v.ReleaseType != "rc" {
		t.Errorf("manifest release_type not seeded; got %q", v.ReleaseType)
	}
	if len(v.BreakingChanges) != 1 || v.BreakingChanges[0] != "foo" {
		t.Errorf("breaking_changes = %v", v.BreakingChanges)
	}
}

func TestFlagOverridesManifestLifecycle(t *testing.T) {
	dir := t.TempDir()
	twinDir := filepath.Join(dir, "twin-stripe")
	os.MkdirAll(twinDir, 0o755)
	manifest := `{
  "twin": "stripe",
  "description": "Stripe",
  "category": "payments",
  "sdk_target": {"primary": {"package": "github.com/stripe/stripe-go", "version": "v80"}},
  "lifecycle": {"release_type": "beta"}
}`
	os.WriteFile(filepath.Join(twinDir, "twin-manifest.json"), []byte(manifest), 0o644)
	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	if err := run([]string{
		"--twin", "stripe", "--version", "0.1.0",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
		"--release-type", "stable",
	}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(registryPath)
	var reg Registry
	json.Unmarshal(data, &reg)
	v := reg.Twins["stripe"].Versions["0.1.0"]
	if v.ReleaseType != "stable" {
		t.Errorf("flag override failed; got %q", v.ReleaseType)
	}
}

func TestChangelogFromFile(t *testing.T) {
	dir := setupManifest(t)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	changelogPath := filepath.Join(dir, "CHANGELOG.md")
	os.WriteFile(changelogPath, []byte("# v0.1.0\n\nFirst release.\n"), 0o644)

	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)
	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	if err := run([]string{
		"--twin", "stripe", "--version", "0.1.0",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
		"--changelog", "@" + changelogPath,
	}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(registryPath)
	var reg Registry
	json.Unmarshal(data, &reg)
	v := reg.Twins["stripe"].Versions["0.1.0"]
	if !strings.Contains(v.Changelog, "First release") {
		t.Errorf("changelog file not loaded; got %q", v.Changelog)
	}
}
