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

// installTool is the wt_install MCP tool entry — the agent's primary
// action for delivering a commercial twin to the local runtime. Per
// adr-license-delivery-via-mcp §4, this is the day-to-day verb;
// wt_trial and wt_subscribe (separate entries, future PR) cover the
// rarer cases of explicit trial-start and plan changes.
//
// Behavior per the matching server-side handler (wondertwin-app
// PR #5):
//
//   - ALLOW outcome (installed, already_installed_refreshed,
//     trial_started_and_installed) → unpack the install bundle,
//     download binary + verify checksum, atomic-write the license
//     to ~/.wondertwin/license.json. Surface the envelope's
//     client_facing_message to the agent.
//   - QUEUE outcome → no install; surface the queue_id and
//     status_url so the agent can poll wt_subscribe_status
//     (separate PR) for the terminal disposition.
//   - DENY / UPGRADE_REQUIRED / SETUP_REQUIRED → surface the
//     envelope verbatim; agent reports to user and does not retry.
//   - POLICY_ERROR (server-side misconfig or unknown outcome
//     fallback) → surface with correlation_id for support.
//
// Forward-compat: unknown outcome values from the server are passed
// through verbatim. The agent client is responsible for the safe-
// degradation behavior (treat as policy_error, surface message,
// stop) per adr-mcp-subscribe-outcomes §4.
func installTool(pc *platform.Client, cfg *config.Config) toolEntry {
	return toolEntry{
		Tool: Tool{
			Name: "wt_install",
			Description: "Install a commercial twin into the local runtime. " +
				"Calls the WonderTwin MCP install endpoint, which evaluates the " +
				"org's agent install policy and either delivers the twin + license " +
				"bundle or queues for human approval. Returns the structured " +
				"envelope from the server.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"twin_name": {"type": "string", "description": "Name of the twin to install (e.g. stripe, hubspot)"},
					"pricing_id": {"type": "string", "description": "Pricing identifier; defaults to price_commercial_monthly"},
					"agent_client_id": {"type": "string", "description": "MCP client identifier for telemetry (e.g. claude-code, cursor)"}
				},
				"required": ["twin_name"]
			}`),
		},
		Handler: func(_ *manifest.Manifest, _ *client.AdminClient, params json.RawMessage) ToolResult {
			var p struct {
				TwinName      string `json:"twin_name"`
				PricingID     string `json:"pricing_id"`
				AgentClientID string `json:"agent_client_id"`
			}
			if len(params) > 0 {
				if err := json.Unmarshal(params, &p); err != nil {
					return errorResult(fmt.Sprintf("decode params: %v", err))
				}
			}
			if p.TwinName == "" {
				return errorResult("twin_name is required")
			}
			if p.PricingID == "" {
				p.PricingID = "price_commercial_monthly"
			}

			if cfg.APIKey == "" || cfg.OrgID == "" {
				return jsonResult(map[string]any{
					"outcome":               string(platform.OutcomeSetupRequired),
					"twin_id":               p.TwinName,
					"client_facing_message": "You're not signed in. Open the WonderTwin signup page to start your trial, then try installing again.",
					"schema_version":        1,
					"outcome_detail": map[string]string{
						"setup_url":   fmt.Sprintf("https://app.wondertwin.ai/signup?ref=mcp&twin=%s", p.TwinName),
						"reason_code": "no_account",
					},
				})
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			env, err := pc.Install(ctx, cfg.OrgID, platform.InstallRequest{
				TwinID:        p.TwinName,
				PricingID:     p.PricingID,
				AgentClientID: p.AgentClientID,
			})
			if err != nil {
				return errorResult(fmt.Sprintf("install: %v", err))
			}

			// On install-bearing outcomes, commit the bundle locally
			// before returning to the agent. Non-install outcomes
			// (queued / denied / setup_required / policy_error)
			// return the envelope as-is.
			if isInstallBearing(env.Outcome) {
				if err := commitBundleIfPresent(env); err != nil {
					// Bundle commit failed after a successful server-
					// side install. Return a policy_error envelope so
					// the agent reports the partial state to the user;
					// the next install attempt will succeed (server
					// already has the entitlement; bundle is just
					// re-delivered).
					return jsonResult(map[string]any{
						"outcome":               string(platform.OutcomePolicyError),
						"twin_id":               p.TwinName,
						"client_facing_message": fmt.Sprintf("Server installed %s but the local install failed: %v. Try again.", p.TwinName, err),
						"schema_version":        1,
						"outcome_detail": map[string]string{
							"error_code":    "bundle_commit_failed",
							"error_message": err.Error(),
						},
					})
				}
			}

			// Refresh-on-MCP-call hygiene per adr-license-delivery-via-mcp §3.
			// For install responses the local license is already up-to-date
			// after commitBundleIfPresent so this is normally a no-op, but
			// it exercises the same path that future non-install MCP tools
			// (subscribe-status polling, etc.) will use. Best-effort: any
			// failure is logged, not surfaced — the original install's
			// outcome stands.
			refreshLicenseIfStale(ctx, pc, cfg, env, slog.Default())

			return jsonResult(env)
		},
	}
}

// isInstallBearing reports whether the outcome carries an
// InstallBundle the runtime should commit. Mirrors the per-outcome
// schema in schemas/mcp-subscribe-outcome-v1.json.
func isInstallBearing(o platform.InstallOutcome) bool {
	switch o {
	case platform.OutcomeInstalled,
		platform.OutcomeAlreadyInstalledRefresh,
		platform.OutcomeTrialStartedInstalled:
		return true
	}
	return false
}

// bundleCommitter is the install-bundle commit function. Production
// uses commitBundleToHome (writes to ~/.wondertwin/); tests override
// to write to t.TempDir() without depending on the user's HOME.
var bundleCommitter = commitBundleToHome

// commitBundleIfPresent decodes the install bundle from the envelope
// and writes both halves locally. Returns an error if the envelope
// outcome is install-bearing but the bundle is missing or malformed —
// that's a server-contract violation, not a normal branch.
func commitBundleIfPresent(env *platform.InstallEnvelope) error {
	detail, err := platform.DecodeInstalledDetail(env)
	if err != nil {
		return fmt.Errorf("decode installed detail: %w", err)
	}
	if detail == nil || detail.InstallBundle == nil {
		return fmt.Errorf("install-bearing outcome %s has no install_bundle", env.Outcome)
	}
	return bundleCommitter(env.TwinID, registry.InstallBundle{
		BinaryURL:      detail.InstallBundle.BinaryURL,
		BinarySHA256:   detail.InstallBundle.BinarySHA256,
		BinaryVersion:  detail.InstallBundle.BinaryVersion,
		LicenseFileB64: detail.InstallBundle.LicenseFileB64,
	})
}

// commitBundleToHome is the default bundleCommitter — writes the
// binary to ~/.wondertwin/bin/ and the license to ~/.wondertwin/
// license.json.
func commitBundleToHome(twinName string, bundle registry.InstallBundle) error {
	return registry.CommitInstallBundle(
		twinName, bundle,
		registry.ExpandPath("~/.wondertwin/bin"),
		registry.ExpandPath("~/.wondertwin/license.json"),
	)
}
