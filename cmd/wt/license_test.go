package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/cimode"
)

func writeLicenseFile(t *testing.T, path string, lic cimode.License, priv ed25519.PrivateKey) {
	t.Helper()
	bytes, err := lic.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	if priv != nil {
		sig := ed25519.Sign(priv, bytes)
		lic.SetSignature(sig)
	}
	data, _ := json.Marshal(lic)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLicenseInstallValid(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	restore := cimode.SetVerificationKeyForTest(pub)
	defer restore()

	dir := t.TempDir()
	src := filepath.Join(dir, "input.json")
	now := time.Now().UTC()
	writeLicenseFile(t, src, cimode.License{
		Key: "lic", OrgID: "org", TwinScope: []string{"*"},
		IssuedAt: now, NotAfter: now.Add(24 * time.Hour),
	}, priv)

	home := t.TempDir()
	t.Setenv(cimode.EnvHome, home)
	t.Setenv(cimode.EnvLicenseFile, "")

	out := captureStdout(t, func() {
		if err := cmdLicenseInstall([]string{src}); err != nil {
			t.Fatalf("install: %v", err)
		}
	})
	if !strings.Contains(out, "valid") {
		t.Errorf("expected 'valid' in output; got: %s", out)
	}
	dst := filepath.Join(home, cimode.DefaultLicenseFilename)
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("expected license at %s; %v", dst, err)
	}
}

func TestLicenseInstallRejectsTampered(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	restore := cimode.SetVerificationKeyForTest(pub)
	defer restore()

	dir := t.TempDir()
	src := filepath.Join(dir, "input.json")
	now := time.Now().UTC()
	lic := cimode.License{
		Key: "lic", OrgID: "org", TwinScope: []string{"*"},
		IssuedAt: now, NotAfter: now.Add(24 * time.Hour),
	}
	writeLicenseFile(t, src, lic, priv)
	// Tamper the file: read, mutate OrgID, write back.
	raw, _ := os.ReadFile(src)
	tampered := strings.Replace(string(raw), `"org_id":"org"`, `"org_id":"evil"`, 1)
	os.WriteFile(src, []byte(tampered), 0o600)

	home := t.TempDir()
	t.Setenv(cimode.EnvHome, home)
	t.Setenv(cimode.EnvLicenseFile, "")

	err := cmdLicenseInstall([]string{src})
	if err == nil {
		t.Fatal("expected install to refuse tampered license")
	}
	if !strings.Contains(err.Error(), cimode.ReasonInvalidSig) {
		t.Errorf("error = %v, want invalid signature", err)
	}
	dst := filepath.Join(home, cimode.DefaultLicenseFilename)
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("destination should not exist after rejection; stat err = %v", err)
	}
}

func TestLicenseInspectDoesNotInstall(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	restore := cimode.SetVerificationKeyForTest(pub)
	defer restore()

	dir := t.TempDir()
	src := filepath.Join(dir, "input.json")
	now := time.Now().UTC()
	writeLicenseFile(t, src, cimode.License{
		Key: "lic", OrgID: "org", TwinScope: []string{"*"},
		IssuedAt: now, NotAfter: now.Add(24 * time.Hour),
	}, priv)

	home := t.TempDir()
	t.Setenv(cimode.EnvHome, home)
	t.Setenv(cimode.EnvLicenseFile, "")

	out := captureStdout(t, func() {
		if err := cmdLicenseInspect([]string{src}); err != nil {
			t.Fatalf("inspect: %v", err)
		}
	})
	if !strings.Contains(out, "valid") {
		t.Errorf("expected valid output; got: %s", out)
	}
	if _, err := os.Stat(filepath.Join(home, cimode.DefaultLicenseFilename)); !os.IsNotExist(err) {
		t.Errorf("inspect should not install; stat err = %v", err)
	}
}

func TestLicenseInstallJSON(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	restore := cimode.SetVerificationKeyForTest(pub)
	defer restore()

	dir := t.TempDir()
	src := filepath.Join(dir, "input.json")
	now := time.Now().UTC()
	writeLicenseFile(t, src, cimode.License{
		Key: "lic", OrgID: "org_acme", TwinScope: []string{"*"},
		IssuedAt: now, NotAfter: now.Add(24 * time.Hour),
	}, priv)

	home := t.TempDir()
	t.Setenv(cimode.EnvHome, home)
	t.Setenv(cimode.EnvLicenseFile, "")

	out := captureStdout(t, func() {
		if err := cmdLicenseInstall([]string{"--json", src}); err != nil {
			t.Fatalf("install --json: %v", err)
		}
	})
	var r licenseReport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !r.Valid || !r.Installed {
		t.Errorf("report = %+v", r)
	}
	if r.License == nil || r.License.OrgID != "org_acme" {
		t.Errorf("license missing or wrong org: %+v", r.License)
	}
}
