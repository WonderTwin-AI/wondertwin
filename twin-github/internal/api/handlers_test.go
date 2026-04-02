package api_test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

func setupGitHub(t *testing.T) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-github-test"}
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

var ghHeaders = map[string]string{
	"Authorization": "Bearer ghp_test_token",
	"Accept":        "application/vnd.github+json",
}

func ghGet(tc *testutil.TwinClient, path string) *testutil.Response {
	return tc.DoWithHeaders("GET", path, nil, ghHeaders)
}

func ghPost(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("POST", path, body, ghHeaders)
}

func ghPatch(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("PATCH", path, body, ghHeaders)
}

func ghPut(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("PUT", path, body, ghHeaders)
}

func ghDelete(tc *testutil.TwinClient, path string) *testutil.Response {
	return tc.DoWithHeaders("DELETE", path, nil, ghHeaders)
}

func createRepo(tc *testutil.TwinClient, name string) {
	ghPost(tc, "/user/repos", map[string]any{
		"name":      name,
		"auto_init": true,
	}).AssertStatus(201)
}

// --- Auth Tests ---

func TestAuthRequired(t *testing.T) {
	_, tc := setupGitHub(t)
	resp := tc.Get("/user")
	resp.AssertStatus(401)
}

func TestAuthBearer(t *testing.T) {
	_, tc := setupGitHub(t)
	resp := ghGet(tc, "/user")
	resp.AssertStatus(200)
}

func TestRateLimit(t *testing.T) {
	_, tc := setupGitHub(t)
	resp := ghGet(tc, "/rate_limit")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["rate"] == nil {
		t.Error("expected rate in response")
	}
}

// --- Repo Tests ---

func TestCreateAndGetRepo(t *testing.T) {
	_, tc := setupGitHub(t)

	resp := ghPost(tc, "/user/repos", map[string]any{
		"name":        "my-repo",
		"description": "Test repo",
		"auto_init":   true,
	})
	resp.AssertStatus(201)
	repo := resp.JSONMap()
	if repo["name"] != "my-repo" {
		t.Errorf("expected name=my-repo, got %v", repo["name"])
	}

	resp = ghGet(tc, "/repos/twin-bot/my-repo")
	resp.AssertStatus(200)
	if resp.JSONMap()["full_name"] != "twin-bot/my-repo" {
		t.Error("expected full_name=twin-bot/my-repo")
	}
}

func TestRepoNotFound(t *testing.T) {
	_, tc := setupGitHub(t)
	resp := ghGet(tc, "/repos/nobody/nothing")
	resp.AssertStatus(404)
}

func TestUpdateRepo(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "update-test")

	resp := ghPatch(tc, "/repos/twin-bot/update-test", map[string]any{
		"description": "updated",
	})
	resp.AssertStatus(200)
	if resp.JSONMap()["description"] != "updated" {
		t.Error("expected updated description")
	}
}

func TestDeleteRepo(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "delete-me")

	ghDelete(tc, "/repos/twin-bot/delete-me").AssertStatus(204)
	ghGet(tc, "/repos/twin-bot/delete-me").AssertStatus(404)
}

// --- Issue Tests ---

func TestCreateAndGetIssue(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "issue-repo")

	resp := ghPost(tc, "/repos/twin-bot/issue-repo/issues", map[string]any{
		"title":  "Bug report",
		"body":   "Something is broken",
		"labels": []string{"bug"},
	})
	resp.AssertStatus(201)
	issue := resp.JSONMap()
	num := int(issue["number"].(float64))

	resp = ghGet(tc, fmt.Sprintf("/repos/twin-bot/issue-repo/issues/%d", num))
	resp.AssertStatus(200)
	if resp.JSONMap()["title"] != "Bug report" {
		t.Error("expected title=Bug report")
	}
}

func TestListIssues(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "list-issues")

	ghPost(tc, "/repos/twin-bot/list-issues/issues", map[string]any{"title": "Issue 1"})
	ghPost(tc, "/repos/twin-bot/list-issues/issues", map[string]any{"title": "Issue 2"})

	resp := ghGet(tc, "/repos/twin-bot/list-issues/issues")
	resp.AssertStatus(200)

	var issues []map[string]any
	json.Unmarshal(resp.Body, &issues)
	if len(issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(issues))
	}
}

func TestCloseIssue(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "close-issue")

	resp := ghPost(tc, "/repos/twin-bot/close-issue/issues", map[string]any{"title": "Will close"})
	num := int(resp.JSONMap()["number"].(float64))

	resp = ghPatch(tc, fmt.Sprintf("/repos/twin-bot/close-issue/issues/%d", num), map[string]any{
		"state": "closed",
	})
	resp.AssertStatus(200)
	if resp.JSONMap()["state"] != "closed" {
		t.Error("expected state=closed")
	}
}

// --- Comment Tests ---

func TestIssueComments(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "comment-repo")

	resp := ghPost(tc, "/repos/twin-bot/comment-repo/issues", map[string]any{"title": "Commented"})
	num := int(resp.JSONMap()["number"].(float64))

	resp = ghPost(tc, fmt.Sprintf("/repos/twin-bot/comment-repo/issues/%d/comments", num), map[string]any{
		"body": "Nice work!",
	})
	resp.AssertStatus(201)
	commentID := resp.JSONMap()["id"].(float64)

	resp = ghGet(tc, fmt.Sprintf("/repos/twin-bot/comment-repo/issues/%d/comments", num))
	var comments []map[string]any
	json.Unmarshal(resp.Body, &comments)
	if len(comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(comments))
	}

	// Delete
	ghDelete(tc, fmt.Sprintf("/repos/twin-bot/comment-repo/issues/comments/%d", int(commentID))).AssertStatus(204)
}

// --- Label Tests ---

func TestLabels(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "label-repo")

	resp := ghPost(tc, "/repos/twin-bot/label-repo/labels", map[string]any{
		"name":  "bug",
		"color": "d73a4a",
	})
	resp.AssertStatus(201)

	resp = ghGet(tc, "/repos/twin-bot/label-repo/labels")
	var labels []map[string]any
	json.Unmarshal(resp.Body, &labels)
	if len(labels) != 1 {
		t.Errorf("expected 1 label, got %d", len(labels))
	}

	ghDelete(tc, "/repos/twin-bot/label-repo/labels/bug").AssertStatus(204)
}

// --- PR Tests ---

func TestCreateAndMergePR(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "pr-repo")

	resp := ghPost(tc, "/repos/twin-bot/pr-repo/pulls", map[string]any{
		"title": "Add feature",
		"head":  "feature",
		"base":  "main",
	})
	resp.AssertStatus(201)
	pr := resp.JSONMap()
	num := int(pr["number"].(float64))
	if pr["state"] != "open" {
		t.Error("expected state=open")
	}

	// Merge
	resp = ghPut(tc, fmt.Sprintf("/repos/twin-bot/pr-repo/pulls/%d/merge", num), nil)
	resp.AssertStatus(200)
	if resp.JSONMap()["merged"] != true {
		t.Error("expected merged=true")
	}

	// Check merged
	ghGet(tc, fmt.Sprintf("/repos/twin-bot/pr-repo/pulls/%d/merge", num)).AssertStatus(204)
}

// --- Commit Status Tests ---

func TestCommitStatuses(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "status-repo")
	sha := "abc123"

	ghPost(tc, fmt.Sprintf("/repos/twin-bot/status-repo/statuses/%s", sha), map[string]any{
		"state":   "success",
		"context": "ci/tests",
	}).AssertStatus(201)

	ghPost(tc, fmt.Sprintf("/repos/twin-bot/status-repo/statuses/%s", sha), map[string]any{
		"state":   "success",
		"context": "ci/lint",
	}).AssertStatus(201)

	resp := ghGet(tc, fmt.Sprintf("/repos/twin-bot/status-repo/commits/%s/status", sha))
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["state"] != "success" {
		t.Errorf("expected combined state=success, got %v", m["state"])
	}
	if m["total_count"].(float64) != 2 {
		t.Error("expected 2 statuses")
	}
}

// --- Release Tests ---

func TestReleases(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "release-repo")

	resp := ghPost(tc, "/repos/twin-bot/release-repo/releases", map[string]any{
		"tag_name": "v1.0.0",
		"name":     "Release 1.0",
		"body":     "First release",
	})
	resp.AssertStatus(201)

	resp = ghGet(tc, "/repos/twin-bot/release-repo/releases/latest")
	resp.AssertStatus(200)
	if resp.JSONMap()["tag_name"] != "v1.0.0" {
		t.Error("expected tag_name=v1.0.0")
	}
}

// --- Webhook Tests ---

func TestWebhooks(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "hook-repo")

	resp := ghPost(tc, "/repos/twin-bot/hook-repo/hooks", map[string]any{
		"events": []string{"push", "pull_request"},
		"config": map[string]any{
			"url":          "https://example.com/webhook",
			"content_type": "json",
		},
	})
	resp.AssertStatus(201)
	hookID := int(resp.JSONMap()["id"].(float64))

	resp = ghGet(tc, "/repos/twin-bot/hook-repo/hooks")
	var hooks []map[string]any
	json.Unmarshal(resp.Body, &hooks)
	if len(hooks) != 1 {
		t.Errorf("expected 1 webhook, got %d", len(hooks))
	}

	ghDelete(tc, fmt.Sprintf("/repos/twin-bot/hook-repo/hooks/%d", hookID)).AssertStatus(204)
}

// --- Milestone Tests ---

func TestMilestones(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "ms-repo")

	resp := ghPost(tc, "/repos/twin-bot/ms-repo/milestones", map[string]any{
		"title": "v1.0",
	})
	resp.AssertStatus(201)
	num := int(resp.JSONMap()["number"].(float64))

	resp = ghGet(tc, fmt.Sprintf("/repos/twin-bot/ms-repo/milestones/%d", num))
	resp.AssertStatus(200)
	if resp.JSONMap()["title"] != "v1.0" {
		t.Error("expected title=v1.0")
	}

	ghDelete(tc, fmt.Sprintf("/repos/twin-bot/ms-repo/milestones/%d", num)).AssertStatus(204)
}

// --- PR Review Tests ---

func TestPRReviews(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "review-repo")

	resp := ghPost(tc, "/repos/twin-bot/review-repo/pulls", map[string]any{
		"title": "Review me", "head": "feature", "base": "main",
	})
	num := int(resp.JSONMap()["number"].(float64))

	resp = ghPost(tc, fmt.Sprintf("/repos/twin-bot/review-repo/pulls/%d/reviews", num), map[string]any{
		"body":  "LGTM",
		"event": "APPROVE",
	})
	resp.AssertStatus(200)
	if resp.JSONMap()["state"] != "APPROVED" {
		t.Error("expected state=APPROVED")
	}

	resp = ghGet(tc, fmt.Sprintf("/repos/twin-bot/review-repo/pulls/%d/reviews", num))
	var reviews []map[string]any
	json.Unmarshal(resp.Body, &reviews)
	if len(reviews) != 1 {
		t.Errorf("expected 1 review, got %d", len(reviews))
	}
}

// --- PR Review Comment Tests ---

func TestPRReviewComments(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "rc-repo")

	resp := ghPost(tc, "/repos/twin-bot/rc-repo/pulls", map[string]any{
		"title": "Comment me", "head": "feature", "base": "main",
	})
	num := int(resp.JSONMap()["number"].(float64))

	resp = ghPost(tc, fmt.Sprintf("/repos/twin-bot/rc-repo/pulls/%d/comments", num), map[string]any{
		"body": "Nitpick here",
		"path": "main.go",
	})
	resp.AssertStatus(201)
	commentID := int(resp.JSONMap()["id"].(float64))

	resp = ghGet(tc, fmt.Sprintf("/repos/twin-bot/rc-repo/pulls/%d/comments", num))
	var comments []map[string]any
	json.Unmarshal(resp.Body, &comments)
	if len(comments) != 1 {
		t.Errorf("expected 1 review comment, got %d", len(comments))
	}

	ghDelete(tc, fmt.Sprintf("/repos/twin-bot/rc-repo/pulls/comments/%d", commentID)).AssertStatus(204)
}

// --- Check Run Tests ---

func TestCheckRuns(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "check-repo")
	sha := "abc123"

	resp := ghPost(tc, "/repos/twin-bot/check-repo/check-runs", map[string]any{
		"name":       "ci/test",
		"head_sha":   sha,
		"status":     "completed",
		"conclusion": "success",
	})
	resp.AssertStatus(201)
	crID := int(resp.JSONMap()["id"].(float64))

	resp = ghGet(tc, fmt.Sprintf("/repos/twin-bot/check-repo/check-runs/%d", crID))
	resp.AssertStatus(200)
	if resp.JSONMap()["conclusion"] != "success" {
		t.Error("expected conclusion=success")
	}

	resp = ghGet(tc, fmt.Sprintf("/repos/twin-bot/check-repo/commits/%s/check-runs", sha))
	m := resp.JSONMap()
	if m["total_count"].(float64) != 1 {
		t.Error("expected 1 check run")
	}
}

// --- Contents Tests ---

func TestContents(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "content-repo")

	// Create a file
	resp := ghPut(tc, "/repos/twin-bot/content-repo/contents/hello.txt", map[string]any{
		"message": "add hello",
		"content": "SGVsbG8gV29ybGQ=", // "Hello World" base64
	})
	resp.AssertStatus(201)

	// Read it back
	resp = ghGet(tc, "/repos/twin-bot/content-repo/contents/hello.txt")
	resp.AssertStatus(200)
	if resp.JSONMap()["name"] != "hello.txt" {
		t.Error("expected name=hello.txt")
	}
}

// --- Deployment Tests ---

func TestDeployments(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "deploy-repo")

	resp := ghPost(tc, "/repos/twin-bot/deploy-repo/deployments", map[string]any{
		"ref":         "main",
		"environment": "production",
	})
	resp.AssertStatus(201)
	deployID := int(resp.JSONMap()["id"].(float64))

	ghPost(tc, fmt.Sprintf("/repos/twin-bot/deploy-repo/deployments/%d/statuses", deployID), map[string]any{
		"state": "success",
	}).AssertStatus(201)

	resp = ghGet(tc, fmt.Sprintf("/repos/twin-bot/deploy-repo/deployments/%d/statuses", deployID))
	var statuses []map[string]any
	json.Unmarshal(resp.Body, &statuses)
	if len(statuses) != 1 {
		t.Errorf("expected 1 deployment status, got %d", len(statuses))
	}
}

// --- Reaction Tests ---

func TestIssueReactions(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "react-repo")

	resp := ghPost(tc, "/repos/twin-bot/react-repo/issues", map[string]any{"title": "React to me"})
	num := int(resp.JSONMap()["number"].(float64))

	resp = ghPost(tc, fmt.Sprintf("/repos/twin-bot/react-repo/issues/%d/reactions", num), map[string]any{
		"content": "+1",
	})
	resp.AssertStatus(201)
	rxID := int(resp.JSONMap()["id"].(float64))

	resp = ghGet(tc, fmt.Sprintf("/repos/twin-bot/react-repo/issues/%d/reactions", num))
	var reactions []map[string]any
	json.Unmarshal(resp.Body, &reactions)
	if len(reactions) != 1 {
		t.Errorf("expected 1 reaction, got %d", len(reactions))
	}

	ghDelete(tc, fmt.Sprintf("/repos/twin-bot/react-repo/issues/%d/reactions/%d", num, rxID)).AssertStatus(204)
}

// --- Org/Team Tests ---

func TestOrgTeams(t *testing.T) {
	_, tc := setupGitHub(t)

	resp := ghPost(tc, "/orgs/test-org/teams", map[string]any{
		"name": "Engineering",
	})
	resp.AssertStatus(201)
	if resp.JSONMap()["slug"] != "engineering" {
		t.Error("expected slug=engineering")
	}

	resp = ghGet(tc, "/orgs/test-org/teams")
	var teams []map[string]any
	json.Unmarshal(resp.Body, &teams)
	if len(teams) != 1 {
		t.Errorf("expected 1 team, got %d", len(teams))
	}

	ghDelete(tc, "/orgs/test-org/teams/engineering").AssertStatus(204)
}

// --- Search Tests ---

func TestSearchIssues(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "search-repo")
	ghPost(tc, "/repos/twin-bot/search-repo/issues", map[string]any{"title": "Search me"})

	resp := ghGet(tc, "/search/issues?q=search")
	resp.AssertStatus(200)
	if resp.JSONMap()["total_count"].(float64) < 1 {
		t.Error("expected at least 1 result")
	}
}

// --- Fork Tests ---

func TestCreateFork(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "fork-me")

	resp := ghPost(tc, "/repos/twin-bot/fork-me/forks", nil)
	resp.AssertStatus(202)
	if resp.JSONMap()["fork"] != true {
		t.Error("expected fork=true")
	}
}

// --- Actions Workflow Tests ---

func TestActionsWorkflowDispatch(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "actions-repo")

	// Seed a workflow
	tc.Post("/admin/state", json.RawMessage(`{"workflows":{"wf_001":{
		"id":1,"name":"CI","path":".github/workflows/ci.yml","state":"active",
		"repo_owner":"twin-bot","repo_name":"actions-repo"
	}}}`)).AssertStatus(200)

	// Trigger it
	resp := ghPost(tc, "/repos/twin-bot/actions-repo/actions/workflows/1/dispatches", map[string]any{
		"ref": "main",
	})
	resp.AssertStatus(204)

	// List runs
	resp = ghGet(tc, "/repos/twin-bot/actions-repo/actions/runs")
	m := resp.JSONMap()
	if m["total_count"].(float64) != 1 {
		t.Errorf("expected 1 run, got %v", m["total_count"])
	}
}

func TestActionsRunCancel(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "cancel-repo")

	// Seed workflow + trigger
	tc.Post("/admin/state", json.RawMessage(`{"workflows":{"wf_001":{
		"id":1,"name":"CI","path":".github/workflows/ci.yml","state":"active",
		"repo_owner":"twin-bot","repo_name":"cancel-repo"
	}}}`)).AssertStatus(200)

	ghPost(tc, "/repos/twin-bot/cancel-repo/actions/workflows/1/dispatches", map[string]any{"ref": "main"})

	resp := ghGet(tc, "/repos/twin-bot/cancel-repo/actions/runs")
	runs := resp.JSONMap()["workflow_runs"].([]any)
	runID := int(runs[0].(map[string]any)["id"].(float64))

	// Cancel
	resp = ghPost(tc, fmt.Sprintf("/repos/twin-bot/cancel-repo/actions/runs/%d/cancel", runID), nil)
	resp.AssertStatus(202)

	// Verify cancelled
	resp = ghGet(tc, fmt.Sprintf("/repos/twin-bot/cancel-repo/actions/runs/%d", runID))
	if resp.JSONMap()["conclusion"] != "cancelled" {
		t.Error("expected conclusion=cancelled")
	}
}

func TestActionsSecrets(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "secret-repo")

	// Create secret
	ghPut(tc, "/repos/twin-bot/secret-repo/actions/secrets/MY_SECRET", map[string]any{
		"encrypted_value": "xxx",
		"key_id":          "012345678912345678",
	}).AssertStatus(201)

	// List
	resp := ghGet(tc, "/repos/twin-bot/secret-repo/actions/secrets")
	if resp.JSONMap()["total_count"].(float64) != 1 {
		t.Error("expected 1 secret")
	}

	// Delete
	ghDelete(tc, "/repos/twin-bot/secret-repo/actions/secrets/MY_SECRET").AssertStatus(204)
}

// --- Git Data Tests ---

func TestGitRefs(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "git-repo")

	// Create ref
	resp := ghPost(tc, "/repos/twin-bot/git-repo/git/refs", map[string]any{
		"ref": "refs/heads/feature",
		"sha": "abc123",
	})
	resp.AssertStatus(201)
	if resp.JSONMap()["ref"] != "refs/heads/feature" {
		t.Error("expected ref=refs/heads/feature")
	}

	// Get ref
	resp = ghGet(tc, "/repos/twin-bot/git-repo/git/ref/heads/feature")
	resp.AssertStatus(200)

	// Delete ref
	ghDelete(tc, "/repos/twin-bot/git-repo/git/refs/heads/feature").AssertStatus(204)
}

func TestGitCommitsAndTrees(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "gitdata-repo")

	// Create blob
	resp := ghPost(tc, "/repos/twin-bot/gitdata-repo/git/blobs", map[string]any{
		"content":  "SGVsbG8=",
		"encoding": "base64",
	})
	resp.AssertStatus(201)
	blobSHA := resp.JSONMap()["sha"].(string)

	// Create tree
	resp = ghPost(tc, "/repos/twin-bot/gitdata-repo/git/trees", map[string]any{
		"tree": []map[string]any{
			{"path": "hello.txt", "mode": "100644", "type": "blob", "sha": blobSHA},
		},
	})
	resp.AssertStatus(201)
	treeSHA := resp.JSONMap()["sha"].(string)

	// Create commit
	resp = ghPost(tc, "/repos/twin-bot/gitdata-repo/git/commits", map[string]any{
		"message": "initial commit",
		"tree":    treeSHA,
	})
	resp.AssertStatus(201)
	if resp.JSONMap()["message"] != "initial commit" {
		t.Error("expected message=initial commit")
	}
}

func TestGitTags(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "tag-repo")

	resp := ghPost(tc, "/repos/twin-bot/tag-repo/git/tags", map[string]any{
		"tag":     "v1.0.0",
		"message": "Release v1.0.0",
		"object":  "abc123",
		"type":    "commit",
	})
	resp.AssertStatus(201)
	if resp.JSONMap()["tag"] != "v1.0.0" {
		t.Error("expected tag=v1.0.0")
	}
}

// --- Admin Tests ---

func TestAdminHealth(t *testing.T) {
	_, tc := setupGitHub(t)
	tc.Get("/admin/health").AssertStatus(200)
}

func TestAdminReset(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "reset-me")

	tc.Post("/admin/reset", nil).AssertStatus(200)

	resp := tc.Get("/admin/repos")
	m := resp.JSONMap()
	if m["total"].(float64) != 0 {
		t.Error("expected 0 repos after reset")
	}
}
