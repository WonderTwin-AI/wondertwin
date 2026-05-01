package twincore

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/cimode"
)

// newTwinForBannerTest builds a Twin, swaps in a captured logger, and
// returns both. Callers re-invoke emitLicenseBanner explicitly after
// mutating Twin.License / Twin.LicenseStatus so the banner reflects
// the test-controlled state rather than whatever LoadLicense found on
// disk.
func newTwinForBannerTest(t *testing.T, name string, env map[string]string) (*Twin, *bytes.Buffer) {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
	tw := New(&Config{Name: name})
	buf := &bytes.Buffer{}
	tw.Logger = slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return tw, buf
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
	tw.emitLicenseBanner()
	if buf.Len() != 0 {
		t.Errorf("local mode should emit no banner; got: %s", buf.String())
	}
}

func TestBannerCIWithoutLicense(t *testing.T) {
	tw, buf := newTwinForBannerTest(t, "twin-test", map[string]string{
		"CI":             "true",
		"GITHUB_ACTIONS": "",
	})
	if tw.Mode != cimode.ModeCI {
		t.Fatalf("Mode = %v, want ci", tw.Mode)
	}
	tw.License = nil
	tw.LicenseStatus = cimode.Status{Reason: cimode.ReasonNoLicense}
	tw.emitLicenseBanner()
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
	tw, buf := newTwinForBannerTest(t, "twin-test", map[string]string{
		"CI":             "true",
		"GITHUB_ACTIONS": "",
	})
	if tw.Mode != cimode.ModeCI {
		t.Fatalf("Mode = %v", tw.Mode)
	}
	tw.License = &cimode.License{Key: "lic", OrgID: "org"}
	tw.LicenseStatus = cimode.Status{Reason: cimode.ReasonExpired, ExpiresIn: -time.Hour}
	tw.emitLicenseBanner()
	if !strings.Contains(buf.String(), cimode.ReasonExpired) {
		t.Errorf("expected expired reason; got: %s", buf.String())
	}
}

func TestBannerCIWithValidLicense(t *testing.T) {
	tw, buf := newTwinForBannerTest(t, "twin-test", map[string]string{
		"CI":             "true",
		"GITHUB_ACTIONS": "",
	})
	tw.License = &cimode.License{Key: "lic", OrgID: "org_acme"}
	tw.LicenseStatus = cimode.Status{Valid: true, ExpiresIn: 24 * time.Hour}
	tw.emitLicenseBanner()
	out := buf.String()
	if !strings.Contains(out, "license active") {
		t.Errorf("expected info banner; got: %s", out)
	}
	if !strings.Contains(out, "org_acme") {
		t.Errorf("expected org id; got: %s", out)
	}
}
