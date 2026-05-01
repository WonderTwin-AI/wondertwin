package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/cimode"
)

// writeUnverifiableLicense writes a syntactically well-formed but
// signature-invalid license file. It exercises the parse → validate
// → reject path of the wt CLI without minting a synthetic valid
// signature: the verifier here is the production embedded public
// key, which has no test override anymore.
func writeUnverifiableLicense(t *testing.T, path string, lic cimode.License) {
	t.Helper()
	lic.SetSignature([]byte("not-a-real-signature"))
	data, _ := json.Marshal(lic)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLicenseInstallRefusesUnsignedLicense(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "input.json")
	now := time.Now().UTC()
	writeUnverifiableLicense(t, src, cimode.License{
		Key: "lic", OrgID: "org", TwinScope: []string{"*"},
		IssuedAt: now, NotAfter: now.Add(24 * time.Hour),
	})

	home := t.TempDir()
	t.Setenv(cimode.EnvHome, home)
	t.Setenv(cimode.EnvLicenseFile, "")

	err := cmdLicenseInstall([]string{src})
	if err == nil {
		t.Fatal("expected install to refuse license without valid signature")
	}
	if !strings.Contains(err.Error(), cimode.ReasonInvalidSig) {
		t.Errorf("error = %v, want %q", err, cimode.ReasonInvalidSig)
	}
	dst := filepath.Join(home, cimode.DefaultLicenseFilename)
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("destination should not exist after rejection; stat err = %v", err)
	}
}

func TestLicenseInstallRefusesMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "garbage.json")
	if err := os.WriteFile(src, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv(cimode.EnvHome, home)
	t.Setenv(cimode.EnvLicenseFile, "")
	if err := cmdLicenseInstall([]string{src}); err == nil {
		t.Fatal("expected install to fail on malformed JSON")
	}
}

func TestLicenseInspectReportsInvalidWithoutInstall(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "input.json")
	now := time.Now().UTC()
	writeUnverifiableLicense(t, src, cimode.License{
		Key: "lic", OrgID: "org", TwinScope: []string{"*"},
		IssuedAt: now, NotAfter: now.Add(24 * time.Hour),
	})

	home := t.TempDir()
	t.Setenv(cimode.EnvHome, home)
	t.Setenv(cimode.EnvLicenseFile, "")

	out := captureStdout(t, func() {
		_ = cmdLicenseInspect([]string{src})
	})
	if !strings.Contains(out, cimode.ReasonInvalidSig) {
		t.Errorf("expected reason in output; got: %s", out)
	}
	if _, err := os.Stat(filepath.Join(home, cimode.DefaultLicenseFilename)); !os.IsNotExist(err) {
		t.Errorf("inspect must not install; stat err = %v", err)
	}
}

func TestLicenseInstallJSONOnInvalidEmitsParseable(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "input.json")
	now := time.Now().UTC()
	writeUnverifiableLicense(t, src, cimode.License{
		Key: "lic", OrgID: "org_acme", TwinScope: []string{"*"},
		IssuedAt: now, NotAfter: now.Add(24 * time.Hour),
	})
	home := t.TempDir()
	t.Setenv(cimode.EnvHome, home)
	t.Setenv(cimode.EnvLicenseFile, "")

	out := captureStdout(t, func() {
		_ = cmdLicenseInstall([]string{"--json", src})
	})
	var r licenseReport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if r.Valid || r.Installed {
		t.Errorf("expected invalid+not-installed; report = %+v", r)
	}
	if r.Reason != cimode.ReasonInvalidSig {
		t.Errorf("Reason = %q, want %q", r.Reason, cimode.ReasonInvalidSig)
	}
}
