package twincore

import (
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/cimode"
)

// licenseLink is the URL surfaced in the soft-gate warn banner so
// operators tailing CI logs know where to acquire a license.
const licenseLink = "https://wondertwin.dev/license"

// loadLicense populates Twin.License and Twin.LicenseStatus. License
// loading errors (malformed file, etc.) are logged but never abort
// startup — license absence is a normal state.
func (t *Twin) loadLicense() {
	lic, err := cimode.LoadLicense()
	if err != nil {
		t.Logger.Warn("failed to load WonderTwin license; continuing unlicensed", "err", err)
		t.LicenseStatus = cimode.Status{Reason: cimode.ReasonNoLicense}
		return
	}
	t.License = lic
	t.LicenseStatus = cimode.ValidateLicense(lic, t.Config.Name, time.Now())
}

// emitLicenseBanner writes a single startup log line in CI mode
// describing license state. Local mode is silent. The soft-gate
// design forbids refusing traffic on license absence — this banner
// is the only signal.
func (t *Twin) emitLicenseBanner() {
	if t.Mode != cimode.ModeCI {
		return
	}
	if t.LicenseStatus.Valid {
		org := ""
		if t.License != nil {
			org = t.License.OrgID
		}
		t.Logger.Info("WonderTwin license active",
			"twin", t.Config.Name,
			"org", org,
			"expires_in", t.LicenseStatus.ExpiresIn.String(),
		)
		return
	}
	t.Logger.Warn("WonderTwin running in CI without valid license",
		"twin", t.Config.Name,
		"reason", t.LicenseStatus.Reason,
		"link", licenseLink,
	)
}
