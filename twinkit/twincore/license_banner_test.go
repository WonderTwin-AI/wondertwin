package twincore

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/cimode"
)

// captureBanner installs a buffered slog handler on the Twin so tests
// can assert against the banner output. It is invoked after New() so
// the constructor's Logger ends up replaced; the existing banner has
// already been emitted to the original logger. To capture the banner,
// callers should set the buffer first via newTwinForBannerTest.
func newTwinForBannerTest(t *testing.T, name string, env map[string]string) (*Twin, *bytes.Buffer) {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
	buf := &bytes.Buffer{}
	// Hook the logger by replacing os.Stdout temporarily? Simpler:
	// build a Twin and then re-emit the banner against a captured
	// logger — the banner is deterministic given Twin state.
	tw := New(&Config{Name: name})
	tw.Logger = slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	tw.emitLicenseBanner()
	return tw, buf
}

func writeSignedLicense(t *testing.T, dir string, lic cimode.License, priv ed25519.PrivateKey) {
	t.Helper()
	bytes, err := lic.SigningBytes()
	if err != nil {
		t.Fatalf("SigningBytes: %v", err)
	}
	sig := ed25519.Sign(priv, bytes)
	lic.SetSignature(sig)
	data, _ := json.Marshal(lic)
	if err := os.WriteFile(filepath.Join(dir, cimode.DefaultLicenseFilename), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBannerSilentInLocalMode(t *testing.T) {
	// CI vars unset → ModeLocal.
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("CIRCLECI", "")
	t.Setenv("BUILDKITE", "")
	t.Setenv("GITLAB_CI", "")
	tw, buf := newTwinForBannerTest(t, "twin-test", nil)
	if tw.Mode != cimode.ModeLocal {
		t.Fatalf("Mode = %v, want local", tw.Mode)
	}
	if buf.Len() != 0 {
		t.Errorf("local mode should emit no banner; got: %s", buf.String())
	}
}

func TestBannerCIWithoutLicense(t *testing.T) {
	dir := t.TempDir()
	tw, buf := newTwinForBannerTest(t, "twin-test", map[string]string{
		"CI":             "true",
		"GITHUB_ACTIONS": "",
		cimode.EnvHome:   dir,
	})
	if tw.Mode != cimode.ModeCI {
		t.Fatalf("Mode = %v, want ci", tw.Mode)
	}
	out := buf.String()
	if !strings.Contains(out, "without valid license") {
		t.Errorf("expected warn banner; got: %s", out)
	}
	if !strings.Contains(out, cimode.ReasonNoLicense) {
		t.Errorf("expected reason %q; got: %s", cimode.ReasonNoLicense, out)
	}
	if !strings.Contains(out, licenseLink) {
		t.Errorf("expected license link; got: %s", out)
	}
}

func TestBannerCIWithExpiredLicense(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restore := cimode.SetVerificationKeyForTest(pub)
	defer restore()

	dir := t.TempDir()
	issued := time.Now().Add(-48 * time.Hour)
	expired := time.Now().Add(-24 * time.Hour)
	writeSignedLicense(t, dir, cimode.License{
		Key: "lic", OrgID: "org", TwinScope: []string{"*"},
		IssuedAt: issued, NotAfter: expired,
	}, priv)

	tw, buf := newTwinForBannerTest(t, "twin-test", map[string]string{
		"CI":             "true",
		"GITHUB_ACTIONS": "",
		cimode.EnvHome:   dir,
	})
	if tw.Mode != cimode.ModeCI {
		t.Fatalf("Mode = %v", tw.Mode)
	}
	if !strings.Contains(buf.String(), cimode.ReasonExpired) {
		t.Errorf("expected expired reason; got: %s", buf.String())
	}
}

func TestBannerCIWithValidLicense(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restore := cimode.SetVerificationKeyForTest(pub)
	defer restore()

	dir := t.TempDir()
	now := time.Now().UTC()
	writeSignedLicense(t, dir, cimode.License{
		Key: "lic", OrgID: "org_acme", TwinScope: []string{"*"},
		IssuedAt: now, NotAfter: now.Add(30 * 24 * time.Hour),
	}, priv)

	tw, buf := newTwinForBannerTest(t, "twin-test", map[string]string{
		"CI":             "true",
		"GITHUB_ACTIONS": "",
		cimode.EnvHome:   dir,
	})
	if !tw.LicenseStatus.Valid {
		t.Fatalf("LicenseStatus = %+v", tw.LicenseStatus)
	}
	out := buf.String()
	if !strings.Contains(out, "license active") {
		t.Errorf("expected info banner; got: %s", out)
	}
	if !strings.Contains(out, "org_acme") {
		t.Errorf("expected org id; got: %s", out)
	}
}
