package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/wondertwin-ai/wondertwin/internal/client"
	"github.com/wondertwin-ai/wondertwin/internal/config"
	"github.com/wondertwin-ai/wondertwin/internal/manifest"
	"github.com/wondertwin-ai/wondertwin/internal/platform"
)

// subscribeTool is the wt_subscribe MCP tool entry — repurposed from
// its previous per-twin shape to the plan-relationship-change verb
// per adr-license-delivery-via-mcp §4.
//
// The distinction from wt_install is meaningful:
//   - wt_install delivers a SPECIFIC twin to the local runtime. The
//     agent's primary action; runs many times per session.
//   - wt_subscribe changes the ORG's BILLING RELATIONSHIP with
//     WonderTwin (trial -> paid; paid -> larger plan; etc.). Rare;
//     typically once per customer lifecycle, driven proactively by
//     an agent client offering a "manage plan" affordance.
//
// V1 implementation is client-side URL construction only — the agent
// surfaces the plan-management URL to the user, who handles the
// actual plan change in the web app. A future PR adds the wondertwin-
// app /v1/accounts/{id}/mcp/subscribe endpoint with smarter branching
// (subscribed when the requested plan is granted; already_entitled
// when already on it; upgrade_required when manual intervention
// needed; setup_required when card collection is needed).
//
// The previous per-twin behavior (route to legacy /v1/orgs/{id}/subscribe
// endpoint) is fully superseded by wt_install. No tool slot is being
// taken away from agents — the wt_subscribe name now has the
// semantics the ADR pinned, and the per-twin install path lives at
// wt_install where it belongs.
//
// Note: the unused pc parameter is preserved for when the server
// endpoint lands. Marked _ for now to satisfy the linter.
func subscribeTool(_ *platform.Client, cfg *config.Config) toolEntry {
	return toolEntry{
		Tool: Tool{
			Name: "wt_subscribe",
			Description: "Change the organization's WonderTwin plan or billing " +
				"relationship — trial-to-paid, paid-to-larger-plan, etc. NOT for " +
				"adding a specific twin (use wt_install for that). Surfaces a " +
				"plan-management URL the user opens in their browser. Returns the " +
				"structured envelope.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"target_plan": {
						"type": "string",
						"description": "Optional plan hint (e.g. pro, enterprise). The web app handles the actual plan change; this is informational telemetry."
					},
					"agent_client_id": {
						"type": "string",
						"description": "MCP client identifier for telemetry (e.g. claude-code, cursor)"
					}
				}
			}`),
		},
		Handler: func(_ *manifest.Manifest, _ *client.AdminClient, params json.RawMessage) ToolResult {
			var p struct {
				TargetPlan    string `json:"target_plan"`
				AgentClientID string `json:"agent_client_id"`
			}
			if len(params) > 0 {
				if err := json.Unmarshal(params, &p); err != nil {
					return errorResult(fmt.Sprintf("decode params: %v", err))
				}
			}

			if cfg.APIKey == "" || cfg.OrgID == "" {
				// Day Zero: no account at all. Plan changes are
				// nonsensical here — there's no plan to change. Route
				// to signup with action=subscribe so the web app's
				// post-signup flow can land them on the plan page.
				signupURL := "https://app.wondertwin.ai/signup?ref=mcp&action=subscribe"
				if p.AgentClientID != "" {
					signupURL += "&agent_client=" + p.AgentClientID
				}
				return jsonResult(map[string]any{
					"outcome": string(platform.OutcomeSetupRequired),
					"twin_id": "*",
					"client_facing_message": "You'll need a WonderTwin account before you can manage a plan. " +
						"Opening the signup page.",
					"schema_version": 1,
					"outcome_detail": map[string]string{
						"setup_url":   signupURL,
						"reason_code": "no_account",
					},
				})
			}

			// Authenticated: open the plan-management page where the
			// web app handles the actual relationship change. Per the
			// ADR, plan changes are rare and human-driven for V1; the
			// server-side endpoint that returns subscribed /
			// already_entitled / upgrade_required directly is deferred.
			planURL := "https://app.wondertwin.ai/" + cfg.OrgID + "/admin/plan"
			if p.TargetPlan != "" {
				planURL += "?target_plan=" + p.TargetPlan
			}
			msg := "Opening the plan-management page so you can change your WonderTwin plan."
			if p.TargetPlan != "" {
				msg = "Opening the plan-management page so you can change your WonderTwin plan to " + p.TargetPlan + "."
			}
			return jsonResult(map[string]any{
				"outcome":               string(platform.OutcomeSetupRequired),
				"twin_id":               "*",
				"client_facing_message": msg,
				"schema_version":        1,
				"outcome_detail": map[string]string{
					"setup_url":   planURL,
					"reason_code": "plan_change_requires_browser",
				},
			})
		},
	}
}
