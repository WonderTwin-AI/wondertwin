package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/wondertwin-ai/wondertwin/internal/client"
	"github.com/wondertwin-ai/wondertwin/internal/config"
	"github.com/wondertwin-ai/wondertwin/internal/manifest"
	"github.com/wondertwin-ai/wondertwin/internal/platform"
)

// trialTool is the wt_trial MCP tool entry — surfaces the free-trial
// affordance to brand-new users. Distinct from wt_install (day-to-day
// twin delivery) and wt_subscribe (plan-relationship changes) per
// adr-license-delivery-via-mcp §4.
//
// **Product rule: one trial per account, ever.** Once an account has
// had a trial (whether expired without conversion or cancelled
// mid-trial), no new trial can be started — the customer must
// subscribe to regain Pro functionality. The tool reflects that:
//
//   - Day Zero (no account). Returns setup_required with a signup
//     URL. The signup flow itself activates the account's one-and-
//     only free trial. Tool message says so explicitly so agents
//     surface "your free trial starts when you finish signup."
//
//   - Authenticated. The CLI cannot determine trial-eligibility
//     locally (server holds the state). Routes the user to the
//     plan-management page where the web app decides what's
//     actually available — trial active, trial expired (and so
//     must subscribe), already on paid plan. The tool MUST NOT
//     claim "you can start a trial" for authenticated users; that
//     determination belongs server-side and is gated by the
//     one-trial-per-account rule.
//
// V1 implementation is client-side URL construction only. The web
// app handles eligibility, card collection, and trial activation.
// When the wondertwin-app /v1/accounts/{id}/mcp/trial endpoint lands
// (separate PR pending Clerk JWT verifier), this tool will call
// through for the smarter branching: trial_started ONLY for accounts
// that have never had a trial; trial_already_active when in-progress;
// upgrade_required (with the subscribe URL) when the trial is used
// and the customer needs to subscribe to get Pro back.
//
// Returns the standard MCP envelope per mcp-subscribe-outcome-v1.json.
// Agents read env.Outcome and surface env.ClientFacingMessage verbatim.
func trialTool(_ *platform.Client, cfg *config.Config) toolEntry {
	return toolEntry{
		Tool: Tool{
			Name: "wt_trial",
			Description: "Surface the free-trial signup affordance to the user. " +
				"For Day-Zero users (no account), returns a signup URL — the " +
				"account's one-and-only free trial activates as part of signup. " +
				"For authenticated users, routes to the plan-management page; " +
				"the web app determines eligibility (one trial per account; " +
				"expired trials require subscribing, not re-trialing). Returns " +
				"the structured envelope.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"agent_client_id": {
						"type": "string",
						"description": "MCP client identifier for telemetry (e.g. claude-code, cursor)"
					}
				}
			}`),
		},
		Handler: func(_ *manifest.Manifest, _ *client.AdminClient, params json.RawMessage) ToolResult {
			var p struct {
				AgentClientID string `json:"agent_client_id"`
			}
			if len(params) > 0 {
				if err := json.Unmarshal(params, &p); err != nil {
					return errorResult(fmt.Sprintf("decode params: %v", err))
				}
			}

			signupURL := "https://app.wondertwin.ai/signup?ref=mcp&action=trial"
			if p.AgentClientID != "" {
				signupURL += "&agent_client=" + p.AgentClientID
			}

			if cfg.APIKey == "" || cfg.OrgID == "" {
				return jsonResult(map[string]any{
					"outcome": string(platform.OutcomeSetupRequired),
					"twin_id": "*",
					"client_facing_message": "Open the signup page to create your WonderTwin account. " +
						"Your one free trial activates as part of signup — you only get one, so " +
						"there's nothing extra to do beyond signing up.",
					"schema_version": 1,
					"outcome_detail": map[string]string{
						"setup_url":   signupURL,
						"reason_code": "no_account",
					},
				})
			}

			// Authenticated. The CLI cannot determine trial-eligibility
			// locally — server holds the state — and per the one-trial-
			// per-account rule, we MUST NOT imply they can start a new
			// trial. Route to the plan-management page where the web app
			// shows the right action: trial-active timer, or
			// subscribe-to-Pro if the trial has ended, or already-on-paid
			// plan management.
			//
			// reason_code='plan_management' (not 'already_authenticated')
			// so the agent's client_facing_message doesn't accidentally
			// suggest "you can start a trial."
			planURL := "https://app.wondertwin.ai/" + cfg.OrgID + "/admin/plan"
			return jsonResult(map[string]any{
				"outcome": string(platform.OutcomeSetupRequired),
				"twin_id": "*",
				"client_facing_message": "You're signed in. Opening the plan page — the web app will " +
					"show your current trial / plan status. (Trials are one per account; if yours " +
					"has ended, you'll subscribe to Pro from this page.)",
				"schema_version": 1,
				"outcome_detail": map[string]string{
					"setup_url":   planURL,
					"reason_code": "plan_management",
				},
			})
		},
	}
}
