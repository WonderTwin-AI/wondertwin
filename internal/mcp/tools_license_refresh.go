package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/wondertwin-ai/wondertwin/internal/client"
	"github.com/wondertwin-ai/wondertwin/internal/config"
	"github.com/wondertwin-ai/wondertwin/internal/manifest"
	"github.com/wondertwin-ai/wondertwin/internal/platform"
	"github.com/wondertwin-ai/wondertwin/internal/registry"
)

// licenseRefreshTool is the wt_license_refresh MCP tool entry per
// adr-license-delivery-via-mcp §3 (refresh-on-MCP-call freshness).
//
// Typical use: the runtime calls this in-band after receiving an
// MCP response whose mcp_metadata.license_current_issued_at is
// newer than its local license.json's IssuedAt — see
// refreshLicenseIfStale below. Agents rarely invoke it directly,
// but the tool is exposed so:
//
//   - An agent client can offer a manual "refresh license" affordance
//     for the rare case the user wants to force a sync.
//   - Diagnostics workflows can call it without going through a
//     specific install.
//
// V1 implementation phones home unconditionally — the server returns
// the freshly-issued license, runtime writes it atomically. The
// no-op-on-already-current optimization (server says "you're already
// at IssuedAt X, here's a no-op response") is deferred; for V1 we
// always pay the IssueLicense cost server-side. Cheap enough.
func licenseRefreshTool(pc *platform.Client, cfg *config.Config) toolEntry {
	return toolEntry{
		Tool: Tool{
			Name: "wt_license_refresh",
			Description: "Force-refresh the local WonderTwin license to match " +
				"the server's current view of the org's entitlements. Normally " +
				"the runtime calls this automatically when it detects a stale " +
				"local license (via mcp_metadata.license_current_issued_at on " +
				"prior responses). Returns the structured envelope.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
		Handler: func(_ *manifest.Manifest, _ *client.AdminClient, _ json.RawMessage) ToolResult {
			if cfg.APIKey == "" || cfg.OrgID == "" {
				return jsonResult(map[string]any{
					"outcome":               string(platform.OutcomeSetupRequired),
					"twin_id":               "*",
					"client_facing_message": "You're not signed in. Open the WonderTwin signup page first, then try again.",
					"schema_version":        1,
					"outcome_detail": map[string]string{
						"setup_url":   "https://app.wondertwin.ai/signup?ref=mcp",
						"reason_code": "no_account",
					},
				})
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			env, err := pc.LicenseRefresh(ctx, cfg.OrgID)
			if err != nil {
				return errorResult(fmt.Sprintf("license-refresh: %v", err))
			}

			if env.Outcome == platform.OutcomeLicenseRefreshed {
				if err := commitLicenseIfPresent(env); err != nil {
					return jsonResult(map[string]any{
						"outcome":               string(platform.OutcomePolicyError),
						"twin_id":               "*",
						"client_facing_message": fmt.Sprintf("Server refreshed the license but the local commit failed: %v.", err),
						"schema_version":        1,
						"outcome_detail": map[string]string{
							"error_code":    "license_commit_failed",
							"error_message": err.Error(),
						},
					})
				}
			}

			return jsonResult(env)
		},
	}
}

// commitLicenseIfPresent atomic-writes the license file from a
// license-refresh envelope's bundle. Sister to commitBundleIfPresent
// in tools_install.go but license-only (no binary). Returns an error
// if the bundle is missing or malformed — that's a server-contract
// violation, not a normal branch.
//
// licenseCommitter is the indirection layer that tests swap out; in
// production it writes to ~/.wondertwin/license.json via the registry
// helper. Mirrors the bundleCommitter pattern from tools_install.go.
func commitLicenseIfPresent(env *platform.InstallEnvelope) error {
	detail, err := platform.DecodeLicenseRefreshedDetail(env)
	if err != nil {
		return fmt.Errorf("decode license-refreshed detail: %w", err)
	}
	if detail == nil || detail.LicenseBundle == nil {
		return fmt.Errorf("license-refreshed outcome has no license_bundle")
	}
	return licenseCommitter(detail.LicenseBundle)
}

// licenseCommitter is the package-level commit function. Production
// uses commitLicenseToHome (writes ~/.wondertwin/license.json); tests
// override to write to t.TempDir() without depending on the user's
// HOME. Mirrors bundleCommitter in tools_install.go.
var licenseCommitter = commitLicenseToHome

func commitLicenseToHome(bundle *platform.LicenseBundle) error {
	licensePath := registry.ExpandPath("~/.wondertwin/license.json")
	return registry.WriteLicenseFromB64(licensePath, bundle.LicenseFileB64)
}

// refreshLicenseIfStale checks the MCP envelope's metadata against
// the local license file and triggers a wt_license_refresh in-band
// if the server's IssuedAt is newer.
//
// Called by other MCP tool handlers (today: wt_install) after a
// successful response. The freshness check is best-effort — any
// error reading the local file, parsing the metadata timestamp, or
// performing the refresh is logged but not surfaced to the caller.
// The original MCP call's outcome stands; the refresh is purely
// background hygiene.
//
// Production wires this in main.go. Today the only tool that
// produces install-bearing responses with mcp_metadata is wt_install,
// so wt_install is the only call site. As more tools land
// (wt_trial / wt_subscribe / etc. with server endpoints), they'll
// pick up the same helper.
func refreshLicenseIfStale(ctx context.Context, pc *platform.Client, cfg *config.Config, env *platform.InstallEnvelope, logger *slog.Logger) {
	if env == nil || env.MCPMetadata == nil || env.MCPMetadata.LicenseCurrentIssuedAt == "" {
		return
	}
	if cfg.APIKey == "" || cfg.OrgID == "" {
		return // unauthenticated; can't refresh anyway
	}

	local, err := registry.ReadLocalLicenseInfo(registry.ExpandPath("~/.wondertwin/license.json"))
	if err != nil {
		logger.WarnContext(ctx, "license-refresh: read local license failed",
			"err", err)
		return
	}
	if !registry.IsLocalLicenseStale(local, env.MCPMetadata.LicenseCurrentIssuedAt) {
		return
	}

	refreshEnv, err := pc.LicenseRefresh(ctx, cfg.OrgID)
	if err != nil {
		logger.WarnContext(ctx, "license-refresh: in-band refresh failed",
			"err", err, "server_issued_at", env.MCPMetadata.LicenseCurrentIssuedAt)
		return
	}
	if refreshEnv.Outcome != platform.OutcomeLicenseRefreshed {
		logger.WarnContext(ctx, "license-refresh: server returned non-success outcome",
			"outcome", refreshEnv.Outcome,
			"message", refreshEnv.ClientFacingMessage)
		return
	}
	if err := commitLicenseIfPresent(refreshEnv); err != nil {
		logger.WarnContext(ctx, "license-refresh: commit failed",
			"err", err)
	}
}
