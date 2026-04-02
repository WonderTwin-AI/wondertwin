package api_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-linear/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-linear/internal/store"
)

func setupLinear(t *testing.T) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-linear-test"}
	twin := twincore.New(cfg)
	handler := api.NewHandler(memStore, twin.Middleware(), nil, nil)
	handler.Routes(twin.Router)
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	tc := testutil.NewTwinClient(t, srv)
	return srv, tc
}

var linearHeaders = map[string]string{
	"Authorization": "lin_api_test_key",
	"Content-Type":  "application/json",
}

func gql(tc *testutil.TwinClient, query string, variables map[string]any) *testutil.Response {
	body := map[string]any{"query": query}
	if variables != nil {
		body["variables"] = variables
	}
	return tc.DoWithHeaders("POST", "/graphql", body, linearHeaders)
}

func gqlData(t *testing.T, resp *testutil.Response) map[string]any {
	t.Helper()
	m := resp.JSONMap()
	if errs, ok := m["errors"]; ok && errs != nil {
		t.Fatalf("GraphQL errors: %v", errs)
	}
	data, ok := m["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data in response")
	}
	return data
}

// Seed a team for issue creation
func seedTeam(t *testing.T, tc *testutil.TwinClient) string {
	t.Helper()
	tc.Post("/admin/state", json.RawMessage(`{"teams":{"team_001":{"id":"team_001","name":"Engineering","key":"ENG"}}}`)).AssertStatus(200)
	return "team_001"
}

// --- Auth ---

func TestAuthRequired(t *testing.T) {
	_, tc := setupLinear(t)
	resp := tc.Post("/graphql", map[string]any{"query": "{ viewer { id } }"})
	resp.AssertStatus(401)
}

// --- Viewer ---

func TestViewer(t *testing.T) {
	_, tc := setupLinear(t)
	resp := gql(tc, `{ viewer { id name email } }`, nil)
	resp.AssertStatus(200)
	data := gqlData(t, resp)
	viewer := data["viewer"].(map[string]any)
	if viewer["email"] == nil {
		t.Error("expected email in viewer")
	}
}

// --- Issues ---

func TestIssueCreateAndQuery(t *testing.T) {
	_, tc := setupLinear(t)
	teamID := seedTeam(t, tc)

	// Create
	resp := gql(tc, `mutation { issueCreate(input: {title: "Bug report", teamId: "`+teamID+`"}) { success issue { id title identifier } } }`, nil)
	resp.AssertStatus(200)
	data := gqlData(t, resp)
	result := data["issueCreate"].(map[string]any)
	if result["success"] != true {
		t.Error("expected success=true")
	}
	issue := result["issue"].(map[string]any)
	issueID := issue["id"].(string)
	if issue["identifier"] == nil {
		t.Error("expected identifier like ENG-1")
	}

	// Query by ID
	resp = gql(tc, `{ issue(id: "`+issueID+`") { id title } }`, nil)
	resp.AssertStatus(200)
	data = gqlData(t, resp)
	if data["issue"].(map[string]any)["title"] != "Bug report" {
		t.Error("expected title=Bug report")
	}

	// List all issues
	resp = gql(tc, `{ issues { nodes { id title } } }`, nil)
	resp.AssertStatus(200)
	data = gqlData(t, resp)
	conn := data["issues"].(map[string]any)
	nodes := conn["nodes"].([]any)
	if len(nodes) != 1 {
		t.Errorf("expected 1 issue, got %d", len(nodes))
	}
}

func TestIssueUpdateAndArchive(t *testing.T) {
	_, tc := setupLinear(t)
	teamID := seedTeam(t, tc)

	resp := gql(tc, `mutation { issueCreate(input: {title: "Will update", teamId: "`+teamID+`"}) { issue { id } } }`, nil)
	issueID := gqlData(t, resp)["issueCreate"].(map[string]any)["issue"].(map[string]any)["id"].(string)

	// Update
	resp = gql(tc, `mutation { issueUpdate(id: "`+issueID+`", input: {title: "Updated title", priority: 1}) { success issue { title priority } } }`, nil)
	data := gqlData(t, resp)
	issue := data["issueUpdate"].(map[string]any)["issue"].(map[string]any)
	if issue["title"] != "Updated title" {
		t.Error("expected updated title")
	}

	// Archive
	resp = gql(tc, `mutation { issueArchive(id: "`+issueID+`") { success } }`, nil)
	data = gqlData(t, resp)
	if data["issueArchive"].(map[string]any)["success"] != true {
		t.Error("expected archive success")
	}

	// Delete
	resp = gql(tc, `mutation { issueDelete(id: "`+issueID+`") { success } }`, nil)
	gqlData(t, resp)
}

// --- Comments ---

func TestComments(t *testing.T) {
	_, tc := setupLinear(t)
	teamID := seedTeam(t, tc)

	resp := gql(tc, `mutation { issueCreate(input: {title: "Commented", teamId: "`+teamID+`"}) { issue { id } } }`, nil)
	issueID := gqlData(t, resp)["issueCreate"].(map[string]any)["issue"].(map[string]any)["id"].(string)

	resp = gql(tc, `mutation { commentCreate(input: {body: "LGTM", issueId: "`+issueID+`"}) { success comment { id body } } }`, nil)
	data := gqlData(t, resp)
	comment := data["commentCreate"].(map[string]any)["comment"].(map[string]any)
	if comment["body"] != "LGTM" {
		t.Error("expected body=LGTM")
	}
	commentID := comment["id"].(string)

	// Delete
	gql(tc, `mutation { commentDelete(id: "`+commentID+`") { success } }`, nil)
}

// --- Projects ---

func TestProjectLifecycle(t *testing.T) {
	_, tc := setupLinear(t)

	resp := gql(tc, `mutation { projectCreate(input: {name: "Q1 Launch"}) { success project { id name state } } }`, nil)
	data := gqlData(t, resp)
	project := data["projectCreate"].(map[string]any)["project"].(map[string]any)
	projectID := project["id"].(string)
	if project["name"] != "Q1 Launch" {
		t.Error("expected name=Q1 Launch")
	}

	// Update state
	resp = gql(tc, `mutation { projectUpdate(id: "`+projectID+`", input: {state: "started"}) { project { state } } }`, nil)
	data = gqlData(t, resp)
	if data["projectUpdate"].(map[string]any)["project"].(map[string]any)["state"] != "started" {
		t.Error("expected state=started")
	}

	// List
	resp = gql(tc, `{ projects { nodes { id name } } }`, nil)
	data = gqlData(t, resp)
	nodes := data["projects"].(map[string]any)["nodes"].([]any)
	if len(nodes) != 1 {
		t.Errorf("expected 1 project, got %d", len(nodes))
	}
}

// --- Teams ---

func TestTeamCreate(t *testing.T) {
	_, tc := setupLinear(t)

	resp := gql(tc, `mutation { teamCreate(input: {name: "Design", key: "DES"}) { success team { id name key } } }`, nil)
	data := gqlData(t, resp)
	team := data["teamCreate"].(map[string]any)["team"].(map[string]any)
	if team["key"] != "DES" {
		t.Error("expected key=DES")
	}
}

// --- Labels ---

func TestLabelCRUD(t *testing.T) {
	_, tc := setupLinear(t)

	resp := gql(tc, `mutation { issueLabelCreate(input: {name: "bug", color: "#d73a4a"}) { success issueLabel { id name color } } }`, nil)
	data := gqlData(t, resp)
	label := data["issueLabelCreate"].(map[string]any)["issueLabel"].(map[string]any)
	labelID := label["id"].(string)
	if label["name"] != "bug" {
		t.Error("expected name=bug")
	}

	// List
	resp = gql(tc, `{ issueLabels { nodes { id name } } }`, nil)
	data = gqlData(t, resp)
	nodes := data["issueLabels"].(map[string]any)["nodes"].([]any)
	if len(nodes) != 1 {
		t.Errorf("expected 1 label, got %d", len(nodes))
	}

	// Archive (delete)
	gql(tc, `mutation { issueLabelArchive(id: "`+labelID+`") { success } }`, nil)
}

// --- Webhooks ---

func TestWebhookCRUD(t *testing.T) {
	_, tc := setupLinear(t)

	resp := gql(tc, `mutation { webhookCreate(input: {url: "https://example.com/hook", label: "My Hook"}) { success webhook { id url enabled } } }`, nil)
	data := gqlData(t, resp)
	wh := data["webhookCreate"].(map[string]any)["webhook"].(map[string]any)
	whID := wh["id"].(string)
	if wh["enabled"] != true {
		t.Error("expected enabled=true")
	}

	// Delete
	gql(tc, `mutation { webhookDelete(id: "`+whID+`") { success } }`, nil)
}

// --- Introspection ---

func TestIntrospection(t *testing.T) {
	_, tc := setupLinear(t)
	resp := gql(tc, `{ __schema { queryType { name } } }`, nil)
	resp.AssertStatus(200)
	data := gqlData(t, resp)
	schema := data["__schema"].(map[string]any)
	if schema["queryType"].(map[string]any)["name"] != "Query" {
		t.Error("expected queryType.name=Query")
	}
}

// --- Admin ---

func TestAdminHealth(t *testing.T) {
	_, tc := setupLinear(t)
	tc.Get("/admin/health").AssertStatus(200)
}

func TestAdminReset(t *testing.T) {
	_, tc := setupLinear(t)
	seedTeam(t, tc)

	tc.Post("/admin/reset", nil).AssertStatus(200)

	resp := tc.Get("/admin/teams")
	if resp.JSONMap()["total"].(float64) != 0 {
		t.Error("expected 0 teams after reset")
	}
}
