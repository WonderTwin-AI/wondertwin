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
