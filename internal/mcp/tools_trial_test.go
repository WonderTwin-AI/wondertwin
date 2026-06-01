package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/internal/config"
	"github.com/wondertwin-ai/wondertwin/internal/platform"
)

func runTrialTool(t *testing.T, cfg *config.Config, params map[string]string) ToolResult {
	t.Helper()
	pc := platform.New("http://unused", "")
	entry := trialTool(pc, cfg)
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return entry.Handler(nil, nil, raw)
}

func TestTrialTool_DayZeroReturnsSetupRequiredWithSignupURL(t *testing.T) {
	t.Parallel()
	res := runTrialTool(t, &config.Config{}, map[string]string{})
	got := unmarshalResult(t, res)
	if got["outcome"] != string(platform.OutcomeSetupRequired) {
		t.Errorf("outcome: want setup_required, got %v", got["outcome"])
	}
	detail, _ := got["outcome_detail"].(map[string]any)
	if detail["reason_code"] != "no_account" {
		t.Errorf("reason_code: want no_account, got %v", detail["reason_code"])
	}
	url, _ := detail["setup_url"].(string)
	if !strings.Contains(url, "/signup") || !strings.Contains(url, "action=trial") {
		t.Errorf("setup_url should be signup with action=trial, got %s", url)
	}
}

func TestTrialTool_AuthenticatedRoutesToPlanPageWithoutTrialClaim(t *testing.T) {
	t.Parallel()
	// Per the one-trial-per-account rule: authenticated users routing
	// through wt_trial must NOT be told "you can start a trial." The
	// envelope routes them to the plan page where the web app
	// determines what's actually available (trial active, expired,
	// already on paid plan).
	res := runTrialTool(t, &config.Config{APIKey: "k", OrgID: "org_t"}, map[string]string{})
	got := unmarshalResult(t, res)
	if got["outcome"] != string(platform.OutcomeSetupRequired) {
		t.Errorf("outcome: want setup_required, got %v", got["outcome"])
	}
	detail, _ := got["outcome_detail"].(map[string]any)
	if detail["reason_code"] != "plan_management" {
		t.Errorf("reason_code: want plan_management, got %v", detail["reason_code"])
	}
	url, _ := detail["setup_url"].(string)
	if !strings.Contains(url, "/admin/plan") {
		t.Errorf("authenticated wt_trial URL should target /admin/plan, got %s", url)
	}
	if !strings.Contains(url, "/org_t/") {
		t.Errorf("setup_url should be scoped to the org, got %s", url)
	}
	// The URL must NOT carry action=trial — that would claim the
	// user can start one, which violates one-trial-per-account.
	if strings.Contains(url, "action=trial") {
		t.Errorf("authenticated URL must NOT carry action=trial (claims new trial possible), got %s", url)
	}
	// Client-facing message must not promise a trial restart.
	msg, _ := got["client_facing_message"].(string)
	if !strings.Contains(strings.ToLower(msg), "one per account") {
		t.Errorf("message should explain one-per-account so agent surfaces it accurately, got %q", msg)
	}
}

func TestTrialTool_DayZeroMessageMentionsAutomaticTrial(t *testing.T) {
	t.Parallel()
	res := runTrialTool(t, &config.Config{}, map[string]string{})
	got := unmarshalResult(t, res)
	msg, _ := got["client_facing_message"].(string)
	if !strings.Contains(strings.ToLower(msg), "one free trial") {
		t.Errorf("Day-Zero message should mention the one-free-trial framing, got %q", msg)
	}
}

func TestTrialTool_AgentClientIDPropagatesToURL(t *testing.T) {
	t.Parallel()
	res := runTrialTool(t, &config.Config{},
		map[string]string{"agent_client_id": "claude-code"})
	got := unmarshalResult(t, res)
	detail, _ := got["outcome_detail"].(map[string]any)
	url, _ := detail["setup_url"].(string)
	if !strings.Contains(url, "agent_client=claude-code") {
		t.Errorf("agent_client_id should propagate to signup URL, got %s", url)
	}
}

func TestTrialTool_EnvelopeSchemaVersionIs1(t *testing.T) {
	t.Parallel()
	res := runTrialTool(t, &config.Config{}, map[string]string{})
	got := unmarshalResult(t, res)
	if v, _ := got["schema_version"].(float64); v != 1 {
		t.Errorf("schema_version: want 1, got %v", got["schema_version"])
	}
	if got["client_facing_message"] == nil || got["client_facing_message"] == "" {
		t.Errorf("client_facing_message must be populated for forward-compat")
	}
}

func TestTrialTool_TwinIDIsWildcard(t *testing.T) {
	t.Parallel()
	// wt_trial isn't twin-specific; the envelope's twin_id field
	// is '*' by convention per mcp-subscribe-outcome-v1.json.
	res := runTrialTool(t, &config.Config{}, map[string]string{})
	got := unmarshalResult(t, res)
	if got["twin_id"] != "*" {
		t.Errorf("twin_id: want '*' for non-twin-specific tool, got %v", got["twin_id"])
	}
}
