package twincore

import (
	"os"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/cimode"
	"github.com/wondertwin-ai/wondertwin/twinkit/replay"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
)

// startTelemetry constructs the per-twin Reporter, mounts the
// per-request middleware on the router, and emits the startup
// twin_started + license_status events.
func (t *Twin) startTelemetry() {
	endpoint := os.Getenv(envTelemetryEndpoint)
	optOut := telemetry.IsTelemetryDisabled(os.Getenv(envTelemetry))

	rep := telemetry.New(telemetry.Config{
		Endpoint:  endpoint,
		OptOut:    optOut,
		OrgHash:   telemetry.OrgHash(),
		LicenseID: licenseIDIfValid(t.License, t.LicenseStatus),
		Logger:    t.Logger,
	})
	t.Telemetry = rep

	if optOut {
		t.Logger.Info("WonderTwin telemetry disabled", "twin", t.Config.Name)
		return
	}

	statusFn := func() (cimode.Status, *cimode.License) {
		t.mu.RLock()
		defer t.mu.RUnlock()
		return t.LicenseStatus, t.License
	}
	t.Router.Use(rep.Middleware(t.Config.Name, t.telemetryTwinVersion(), modeString(t.Mode), t.Detection.Platform, statusFn))

	now := time.Now().UTC()
	commonAttribution := telemetry.Event{
		TwinName:   t.Config.Name,
		TwinVer:    t.telemetryTwinVersion(),
		Mode:       modeString(t.Mode),
		Platform:   t.Detection.Platform,
		OrgHash:    rep.OrgHash(),
		LicenseID:  licenseIDIfValid(t.License, t.LicenseStatus),
		LicenseOK:  t.LicenseStatus.Valid,
		LicenseRsn: t.LicenseStatus.Reason,
		Timestamp:  now,
	}
	twinStarted := commonAttribution
	twinStarted.Type = "twin_started"
	rep.Record(twinStarted)

	licenseEvt := commonAttribution
	licenseEvt.Type = "license_status"
	rep.Record(licenseEvt)
}

// telemetryTwinVersion returns the version string the reporter stamps
// onto events. twincore.Config currently has no Version field; we
// surface an empty string so the registry-side ingestion treats it as
// "running from source." Future configs can override.
func (t *Twin) telemetryTwinVersion() string {
	return ""
}

// modeString stringifies a cimode.Mode for telemetry payloads.
func modeString(m cimode.Mode) string { return m.String() }

// TwinName implements admin.RunAttribution.
func (t *Twin) TwinName() string { return t.Config.Name }

// TwinVersion implements admin.RunAttribution. Always empty today;
// session 2A or a future config-bump can wire a real value.
func (t *Twin) TwinVersion() string { return t.telemetryTwinVersion() }

// Mode implements admin.RunAttribution.
func (t *Twin) ModeString() string { return modeString(t.Mode) }

// Platform implements admin.RunAttribution.
func (t *Twin) Platform() string { return t.Detection.Platform }

// OrgHash implements admin.RunAttribution. Returns the reporter's
// configured hash (or empty if telemetry was opted out).
func (t *Twin) OrgHash() string {
	if t.Telemetry == nil {
		return ""
	}
	return t.Telemetry.OrgHash()
}

// LicenseID implements admin.RunAttribution. Returns the license key
// only when valid — invalid licenses must not leak their identifier.
func (t *Twin) LicenseID() string {
	return licenseIDIfValid(t.License, t.LicenseStatus)
}

// LicenseOK implements admin.RunAttribution.
func (t *Twin) LicenseOK() bool { return t.LicenseStatus.Valid }

// LicenseReason implements admin.RunAttribution.
func (t *Twin) LicenseReason() string { return t.LicenseStatus.Reason }

// ReplayRecorder implements admin.TwinSubject.
func (t *Twin) ReplayRecorder() *replay.Recorder { return t.Replay }

// TelemetryReporter implements admin.TwinSubject.
func (t *Twin) TelemetryReporter() *telemetry.Reporter { return t.Telemetry }

// Reseed implements admin.Reseeder by delegating to twin.Rand.
func (t *Twin) Reseed(seed int64) {
	if t.Rand != nil {
		t.Rand.Reseed(seed)
	}
}

// licenseIDIfValid returns the license key only when the license is
// valid. An invalid license must not leak its identifier into
// telemetry payloads — only its Reason.
func licenseIDIfValid(lic *cimode.License, st cimode.Status) string {
	if lic == nil || !st.Valid {
		return ""
	}
	return lic.Key
}

// Close releases per-twin resources (currently the telemetry reporter
// and the replay recorder). Safe to call multiple times.
func (t *Twin) Close() {
	if t == nil {
		return
	}
	if t.Telemetry != nil {
		t.Telemetry.Close()
	}
	if t.Replay != nil {
		t.Replay.Close()
	}
}
