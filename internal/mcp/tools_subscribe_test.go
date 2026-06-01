package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/internal/config"
	"github.com/wondertwin-ai/wondertwin/internal/platform"
)

func runSubscribeTool(t *testing.T, cfg *config.Config, params map[string]string) ToolResult {
	t.Helper()
	pc := platform.New("http://unused", "")
	entry := subscribeTool(pc, cfg)
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return entry.Handler(nil, nil, raw)
}

func TestSubscribeTool_DayZeroRoutesToSignup(t *testing.T) {
	t.Parallel()
	res := runSubscribeTool(t, &config.Config{}, map[string]string{})
	got := unmarshalResult(t, res)
	if got["outcome"] != string(platform.OutcomeSetupRequired) {
		t.Errorf("outcome: want setup_required, got %v", got["outcome"])
	}
	detail, _ := got["outcome_detail"].(map[string]any)
	if detail["reason_code"] != "no_account" {
		t.Errorf("reason_code: want no_account on Day Zero, got %v", detail["reason_code"])
	}
	url, _ := detail["setup_url"].(string)
	if !strings.Contains(url, "/signup") || !strings.Contains(url, "action=subscribe") {
		t.Errorf("Day-Zero subscribe URL should be /signup?action=subscribe, got %s", url)
	}
}

func TestSubscribeTool_AuthenticatedReturnsPlanPage(t *testing.T) {
	t.Parallel()
	res := runSubscribeTool(t, &config.Config{APIKey: "k", OrgID: "org_t"}, map[string]string{})
	got := unmarshalResult(t, res)
	if got["outcome"] != string(platform.OutcomeSetupRequired) {
		t.Errorf("outcome: want setup_required, got %v", got["outcome"])
	}
	detail, _ := got["outcome_detail"].(map[string]any)
	if detail["reason_code"] != "plan_change_requires_browser" {
		t.Errorf("reason_code: want plan_change_requires_browser, got %v", detail["reason_code"])
	}
	url, _ := detail["setup_url"].(string)
	if !strings.Contains(url, "/org_t/admin/plan") {
		t.Errorf("authenticated subscribe URL should target /{org}/admin/plan, got %s", url)
	}
}

func TestSubscribeTool_TargetPlanPropagatesToURL(t *testing.T) {
	t.Parallel()
	res := runSubscribeTool(t, &config.Config{APIKey: "k", OrgID: "org_t"},
		map[string]string{"target_plan": "enterprise"})
	got := unmarshalResult(t, res)
	detail, _ := got["outcome_detail"].(map[string]any)
	url, _ := detail["setup_url"].(string)
	if !strings.Contains(url, "target_plan=enterprise") {
		t.Errorf("target_plan should propagate to URL, got %s", url)
	}
	msg, _ := got["client_facing_message"].(string)
	if !strings.Contains(msg, "enterprise") {
		t.Errorf("client_facing_message should mention target plan, got %s", msg)
	}
}

func TestSubscribeTool_AgentClientIDPropagatesOnDayZero(t *testing.T) {
	t.Parallel()
	res := runSubscribeTool(t, &config.Config{},
		map[string]string{"agent_client_id": "cursor"})
	got := unmarshalResult(t, res)
	detail, _ := got["outcome_detail"].(map[string]any)
	url, _ := detail["setup_url"].(string)
	if !strings.Contains(url, "agent_client=cursor") {
		t.Errorf("agent_client_id should propagate to signup URL, got %s", url)
	}
}

func TestSubscribeTool_NoTwinIDRequired(t *testing.T) {
	t.Parallel()
	// Previous wt_subscribe required twin_name. The refactored ADR
	// version is org-scoped, no twin parameter.
	res := runSubscribeTool(t, &config.Config{APIKey: "k", OrgID: "org_t"}, nil)
	got := unmarshalResult(t, res)
	if got["outcome"] != string(platform.OutcomeSetupRequired) {
		t.Errorf("no-param call should succeed; got outcome=%v", got["outcome"])
	}
}

func TestSubscribeTool_EnvelopeShape(t *testing.T) {
	t.Parallel()
	res := runSubscribeTool(t, &config.Config{APIKey: "k", OrgID: "org_t"}, nil)
	got := unmarshalResult(t, res)
	for _, key := range []string{"outcome", "twin_id", "outcome_detail", "client_facing_message", "schema_version"} {
		if _, ok := got[key]; !ok {
			t.Errorf("envelope missing required field %q", key)
		}
	}
	if got["twin_id"] != "*" {
		t.Errorf("twin_id: want '*' for org-scoped tool, got %v", got["twin_id"])
	}
}
