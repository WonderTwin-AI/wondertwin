package twincore

import (
	"os"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/cimode"
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
