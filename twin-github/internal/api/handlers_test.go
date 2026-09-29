package api_test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-github/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func setupGitHub(t *testing.T) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-github-test"}
	twin := twincore.New(cfg)
	handler := api.NewHandler(memStore, twin.Middleware())
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
	if _, ok := m["rate"]; ok {
		t.Error("2026-03-10 removes the top-level rate object")
	}
	core := m["resources"].(map[string]any)["core"].(map[string]any)
	if core["limit"] == nil {
		t.Error("expected resources.core.limit")
	}
}

func TestBadCredentials(t *testing.T) {
	_, tc := setupGitHub(t)
	for _, auth := range []string{"Bearer not-a-github-token", "token nope", "Basic abc"} {
		resp := tc.DoWithHeaders("GET", "/user", nil, map[string]string{"Authorization": auth})
		resp.AssertStatus(401)
		if m := resp.JSONMap(); m["message"] != "Bad credentials" || m["status"] != "401" {
			t.Errorf("%s: unexpected body %v", auth, m)
		}
	}
}

func TestPublicReadsWithoutToken(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "open-source")
	resp := tc.Get("/repos/twin-bot/open-source")
	resp.AssertStatus(200)
	if resp.Headers.Get("X-RateLimit-Limit") != "60" {
		t.Errorf("unauthenticated callers get the 60/hour limit, got %q", resp.Headers.Get("X-RateLimit-Limit"))
	}
	core := tc.Get("/rate_limit").AssertStatus(200).JSONMap()["resources"].(map[string]any)["core"].(map[string]any)
	if core["limit"].(float64) != 60 {
		t.Errorf("expected core limit 60 unauthenticated, got %v", core["limit"])
	}
	tc.Post("/repos/twin-bot/open-source/issues", map[string]any{"title": "x"}).AssertStatus(401)
	tc.Get("/repos/twin-bot/open-source/hooks").AssertStatus(401)
}

func TestPrivateRepoHiddenWithoutToken(t *testing.T) {
	_, tc := setupGitHub(t)
	ghPost(tc, "/user/repos", map[string]any{"name": "secret", "private": true}).AssertStatus(201)
	tc.Get("/repos/twin-bot/secret").AssertStatus(404)
	ghGet(tc, "/repos/twin-bot/secret").AssertStatus(200)
}

func TestSeededTokenAuthenticatesAsItsUser(t *testing.T) {
	_, tc := setupGitHub(t)
	admin := testutil.NewAdminClient(tc)
	admin.LoadState(map[string]any{
		"users":  map[string]any{"reviewer": map[string]any{"id": 190000500, "login": "reviewer", "type": "User"}},
		"tokens": map[string]any{"reviewer-token": map[string]any{"token": "reviewer-token", "login": "reviewer", "kind": "user"}},
	}).AssertStatus(200)
	u := tc.DoWithHeaders("GET", "/user", nil, map[string]string{"Authorization": "Bearer reviewer-token"}).AssertStatus(200).JSONMap()
	if u["login"] != "reviewer" {
		t.Errorf("expected reviewer, got %v", u["login"])
	}
	if ghGet(tc, "/user").AssertStatus(200).JSONMap()["login"] != "twin-bot" {
		t.Error("an unregistered well-formed token is the default user")
	}
}

// --- API Version Tests ---

func withHeaders(extra map[string]string) map[string]string {
	h := map[string]string{}
	for k, v := range ghHeaders {
		h[k] = v
	}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

func TestVersionsListsOnlyServedVersion(t *testing.T) {
	_, tc := setupGitHub(t)
	resp := tc.Get("/versions")
	resp.AssertStatus(200)
	var versions []string
	resp.JSON(&versions)
	if len(versions) != 1 || versions[0] != "2026-03-10" {
		t.Errorf("expected [2026-03-10], got %v", versions)
	}
}

func TestVersionDefaultAndExplicitAreEchoed(t *testing.T) {
	_, tc := setupGitHub(t)
	for _, hdr := range []map[string]string{ghHeaders, withHeaders(map[string]string{"X-GitHub-Api-Version": "2026-03-10"})} {
		resp := tc.DoWithHeaders("GET", "/rate_limit", nil, hdr)
		resp.AssertStatus(200)
		if got := resp.Headers.Get("X-GitHub-Api-Version-Selected"); got != "2026-03-10" {
			t.Errorf("expected selected version 2026-03-10, got %q", got)
		}
	}
}

func TestUnsupportedVersionsAreRefused(t *testing.T) {
	_, tc := setupGitHub(t)
	for _, v := range []string{"2022-11-28", "2024-01-01", "latest"} {
		resp := tc.DoWithHeaders("GET", "/rate_limit", nil, withHeaders(map[string]string{"X-GitHub-Api-Version": v}))
		resp.AssertStatus(400)
		if resp.Headers.Get("X-GitHub-Api-Version-Selected") != "" {
			t.Errorf("%s: the 400 must not echo a selected version", v)
		}
		m := resp.JSONMap()
		errs, _ := m["errors"].(string)
		if m["message"] != "Bad Request" || m["status"] != "400" {
			t.Errorf("%s: unexpected body %v", v, m)
		}
		if !strings.Contains(errs, `"`+v+`"`) || !strings.Contains(errs, `"2026-03-10" (most recent)`) {
			t.Errorf("%s: errors should name the value and the supported version, got %q", v, errs)
		}
	}
}

func TestUnknownRouteIsGitHubNotFound(t *testing.T) {
	_, tc := setupGitHub(t)
	resp := ghGet(tc, "/orgs/acme/teams/core/discussions")
	resp.AssertStatus(404)
	if m := resp.JSONMap(); m["message"] != "Not Found" || m["status"] != "404" {
		t.Errorf("unexpected 404 body %v", m)
	}
}

func TestRootAndZen(t *testing.T) {
	_, tc := setupGitHub(t)
	root := tc.Get("/").AssertStatus(200).JSONMap()
	if _, ok := root["authorizations_url"]; ok {
		t.Error("2026-03-10 root drops authorizations_url")
	}
	if !strings.HasSuffix(root["rate_limit_url"].(string), "/rate_limit") {
		t.Errorf("unexpected rate_limit_url %v", root["rate_limit_url"])
	}
	if z := tc.Get("/zen").AssertStatus(200); len(z.Body) == 0 {
		t.Error("expected a zen line")
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

func TestDuplicateRepoIs422(t *testing.T) {
	_, tc := setupGitHub(t)
	first := ghPost(tc, "/user/repos", map[string]any{"name": "dup"}).AssertStatus(201).JSONMap()
	resp := ghPost(tc, "/user/repos", map[string]any{"name": "dup"})
	resp.AssertStatus(422)
	m := resp.JSONMap()
	errs := m["errors"].([]any)
	if m["message"] != "Repository creation failed." || errs[0].(map[string]any)["message"] != "name already exists on this account" {
		t.Errorf("unexpected 422 body %v", m)
	}
	if got := ghGet(tc, "/repos/twin-bot/dup").JSONMap()["id"]; got != first["id"] {
		t.Errorf("the first repository must survive, got id %v want %v", got, first["id"])
	}
}

// --- 2026-03-10 Shape Tests ---

func TestRepoCarriesFullRepositoryFields(t *testing.T) {
	_, tc := setupGitHub(t)
	repo := ghPost(tc, "/user/repos", map[string]any{"name": "shape", "auto_init": true}).AssertStatus(201).JSONMap()
	assertRequired(t, "full-repository", repo)
	assertRequired(t, "simple-user", repo["owner"].(map[string]any))
	for _, gone := range []string{"has_downloads", "use_squash_pr_title_as_default", "master_branch"} {
		if _, ok := repo[gone]; ok {
			t.Errorf("2026-03-10 removes %s", gone)
		}
	}
	if !strings.HasPrefix(repo["node_id"].(string), "R_") {
		t.Errorf("unexpected node_id %v", repo["node_id"])
	}
	if !strings.HasPrefix(repo["url"].(string), "http://127.0.0.1:") {
		t.Errorf("API URLs point at the emulator, got %v", repo["url"])
	}
	var list []map[string]any
	ghGet(tc, "/user/repos").AssertStatus(200).JSON(&list)
	assertRequired(t, "repository", list[0])
}

func TestIssueCarriesIssueFields(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "issue-shape")
	issue := ghPost(tc, "/repos/twin-bot/issue-shape/issues", map[string]any{
		"title": "Broken", "labels": []string{"bug"}, "assignees": []string{"twin-bot"},
	}).AssertStatus(201).JSONMap()
	assertRequired(t, "issue", issue)
	if _, ok := issue["assignee"]; ok {
		t.Error("2026-03-10 removes the singular assignee")
	}
	labels := issue["labels"].([]any)
	if len(labels) != 1 {
		t.Fatalf("expected the bug label, got %v", labels)
	}
	assertRequired(t, "label", labels[0].(map[string]any))
	if issue["author_association"] != "OWNER" {
		t.Errorf("expected OWNER, got %v", issue["author_association"])
	}
	// Labelling with a new name creates the label, as GitHub does.
	ghGet(tc, "/repos/twin-bot/issue-shape/labels/bug").AssertStatus(200)

	comment := ghPost(tc, "/repos/twin-bot/issue-shape/issues/1/comments", map[string]any{"body": "seen"}).AssertStatus(201).JSONMap()
	assertRequired(t, "issue-comment", comment)

	closed := ghPatch(tc, "/repos/twin-bot/issue-shape/issues/1", map[string]any{"state": "closed", "state_reason": "not_planned"}).AssertStatus(200).JSONMap()
	if closed["state_reason"] != "not_planned" || closed["closed_by"] == nil || closed["closed_at"] == nil {
		t.Errorf("unexpected closed issue %v %v %v", closed["state_reason"], closed["closed_by"], closed["closed_at"])
	}
}

func TestDuplicateLabelIsAlreadyExists(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "labels-dup")
	ghPost(tc, "/repos/twin-bot/labels-dup/labels", map[string]any{"name": "triage", "color": "#fbca04"}).AssertStatus(201)
	resp := ghPost(tc, "/repos/twin-bot/labels-dup/labels", map[string]any{"name": "triage"})
	resp.AssertStatus(422)
	errs := resp.JSONMap()["errors"].([]any)
	if errs[0].(map[string]any)["code"] != "already_exists" {
		t.Errorf("expected already_exists, got %v", errs)
	}
	if c := ghGet(tc, "/repos/twin-bot/labels-dup/labels/triage").JSONMap()["color"]; c != "fbca04" {
		t.Errorf("a leading # is dropped from the colour, got %v", c)
	}
}

func TestIssuesListNewestFirst(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "order")
	admin := testutil.NewAdminClient(tc)
	for _, title := range []string{"first", "second"} {
		ghPost(tc, "/repos/twin-bot/order/issues", map[string]any{"title": title}).AssertStatus(201)
		admin.AdvanceTime("1m").AssertStatus(200)
	}
	var list []map[string]any
	ghGet(tc, "/repos/twin-bot/order/issues").JSON(&list)
	if list[0]["title"] != "second" {
		t.Errorf("default sort is created desc, got %v first", list[0]["title"])
	}
	ghGet(tc, "/repos/twin-bot/order/issues?direction=asc").JSON(&list)
	if list[0]["title"] != "first" {
		t.Errorf("direction=asc puts the oldest first, got %v", list[0]["title"])
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

// --- Pagination Tests ---

func TestIssuesPaginateWithLinkHeader(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "paged")
	for i := 1; i <= 5; i++ {
		ghPost(tc, "/repos/twin-bot/paged/issues", map[string]any{"title": fmt.Sprintf("issue %d", i)}).AssertStatus(201)
	}

	resp := ghGet(tc, "/repos/twin-bot/paged/issues?per_page=2&page=2")
	resp.AssertStatus(200)
	var page []map[string]any
	resp.JSON(&page)
	if len(page) != 2 {
		t.Fatalf("expected 2 issues on page 2, got %d", len(page))
	}
	link := resp.Headers.Get("Link")
	for _, want := range []string{`page=1&per_page=2>; rel="prev"`, `page=3&per_page=2>; rel="next"`, `page=3&per_page=2>; rel="last"`, `rel="first"`} {
		if !strings.Contains(link, want) {
			t.Errorf("Link %q lacks %s", link, want)
		}
	}
	if !strings.HasPrefix(link, "<http://127.0.0.1:") {
		t.Errorf("Link must point back at the emulator, got %q", link)
	}

	last := ghGet(tc, "/repos/twin-bot/paged/issues?per_page=2&page=3")
	last.JSON(&page)
	if len(page) != 1 || strings.Contains(last.Headers.Get("Link"), `rel="next"`) {
		t.Errorf("last page should hold 1 issue and no next link, got %d and %q", len(page), last.Headers.Get("Link"))
	}

	all := ghGet(tc, "/repos/twin-bot/paged/issues")
	all.JSON(&page)
	if len(page) != 5 || all.Headers.Get("Link") != "" {
		t.Errorf("one page of 5 needs no Link header, got %d items and %q", len(page), all.Headers.Get("Link"))
	}
}

func TestEmptyListIsArray(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "empty-list")
	resp := ghGet(tc, "/repos/twin-bot/empty-list/labels")
	resp.AssertStatus(200)
	if strings.TrimSpace(string(resp.Body)) != "[]" {
		t.Errorf("expected [], got %s", resp.Body)
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

func mainSHA(tc *testutil.TwinClient, repo string) string {
	return ghGet(tc, "/repos/twin-bot/"+repo+"/git/ref/heads/main").AssertStatus(200).JSONMap()["object"].(map[string]any)["sha"].(string)
}

func TestGitRefs(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "git-repo")
	base := mainSHA(tc, "git-repo")

	ghPost(tc, "/repos/twin-bot/git-repo/git/refs", map[string]any{"ref": "refs/heads/feature", "sha": "0123456789abcdef0123456789abcdef01234567"}).AssertStatus(422)

	resp := ghPost(tc, "/repos/twin-bot/git-repo/git/refs", map[string]any{"ref": "refs/heads/feature", "sha": base})
	resp.AssertStatus(201)
	if resp.JSONMap()["ref"] != "refs/heads/feature" {
		t.Error("expected ref=refs/heads/feature")
	}
	ghPost(tc, "/repos/twin-bot/git-repo/git/refs", map[string]any{"ref": "refs/heads/feature", "sha": base}).AssertStatus(422)

	got := ghGet(tc, "/repos/twin-bot/git-repo/git/ref/heads/feature").AssertStatus(200).JSONMap()
	if got["object"].(map[string]any)["sha"] != base {
		t.Errorf("new branch should point at main, got %v", got["object"])
	}
	var branches []map[string]any
	ghGet(tc, "/repos/twin-bot/git-repo/branches").AssertStatus(200).JSON(&branches)
	if len(branches) != 2 {
		t.Errorf("a ref under refs/heads is a branch; expected 2 branches, got %d", len(branches))
	}

	ghDelete(tc, "/repos/twin-bot/git-repo/git/refs/heads/feature").AssertStatus(204)
	ghGet(tc, "/repos/twin-bot/git-repo/git/ref/heads/feature").AssertStatus(404)
}

func TestEmptyRepoRefIs409(t *testing.T) {
	_, tc := setupGitHub(t)
	ghPost(tc, "/user/repos", map[string]any{"name": "blank"}).AssertStatus(201)
	ghGet(tc, "/repos/twin-bot/blank/git/ref/heads/main").AssertStatus(409)
	var branches []map[string]any
	ghGet(tc, "/repos/twin-bot/blank/branches").AssertStatus(200).JSON(&branches)
	if len(branches) != 0 {
		t.Errorf("an empty repository has no branches, got %d", len(branches))
	}
}

func TestContentsBootstrapAnEmptyRepo(t *testing.T) {
	_, tc := setupGitHub(t)
	ghPost(tc, "/user/repos", map[string]any{"name": "boot"}).AssertStatus(201)

	put := ghPut(tc, "/repos/twin-bot/boot/contents/README.md", map[string]any{
		"message": "init", "content": "IyBoZWxsbwo=",
	}).AssertStatus(201).JSONMap()
	commit := put["commit"].(map[string]any)
	blob := put["content"].(map[string]any)["sha"].(string)
	// `printf '# hello\n' | git hash-object --stdin`
	if blob != "8954bb97349bfe2a7799e6a7a64c6f747c635d6c" {
		t.Errorf("unexpected blob sha %s", blob)
	}
	// `git write-tree` over the same single file
	if tree := commit["tree"].(map[string]any)["sha"]; tree != "5e7818ac0625a0a2c14f7fb43e344a856cbbc8cb" {
		t.Errorf("unexpected tree sha %v", tree)
	}

	var branches []map[string]any
	ghGet(tc, "/repos/twin-bot/boot/branches").AssertStatus(200).JSON(&branches)
	if len(branches) != 1 || branches[0]["name"] != "main" || branches[0]["commit"].(map[string]any)["sha"] != commit["sha"] {
		t.Fatalf("the first commit creates main at that commit, got %v", branches)
	}

	file := ghGet(tc, "/repos/twin-bot/boot/contents/README.md").AssertStatus(200).JSONMap()
	if file["type"] != "file" || file["sha"] != blob || file["content"] != "IyBoZWxsbwo=\n" {
		t.Errorf("unexpected file %v", file)
	}

	// Updating needs the current blob sha.
	ghPut(tc, "/repos/twin-bot/boot/contents/README.md", map[string]any{"message": "again", "content": "eAo="}).AssertStatus(422)
	upd := ghPut(tc, "/repos/twin-bot/boot/contents/README.md", map[string]any{"message": "again", "content": "eAo=", "sha": blob}).AssertStatus(200).JSONMap()
	parents := upd["commit"].(map[string]any)["parents"].([]any)
	if len(parents) != 1 || parents[0].(map[string]any)["sha"] != commit["sha"] {
		t.Errorf("the update's parent is the first commit, got %v", parents)
	}
}

func TestGitBlobSHAMatchesGit(t *testing.T) {
	// `printf 'Hello' | git hash-object --stdin`
	if got := store.BlobSHA([]byte("Hello")); got != "5ab2f8a4323abafb10abb68657d9d39f1a775057" {
		t.Errorf("blob sha = %s", got)
	}
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
		"parents": []string{mainSHA(tc, "gitdata-repo")},
	})
	resp.AssertStatus(201)
	c := resp.JSONMap()
	if c["message"] != "initial commit" {
		t.Error("expected message=initial commit")
	}
	ghGet(tc, "/repos/twin-bot/gitdata-repo/git/commits/"+c["sha"].(string)).AssertStatus(200)

	// Moving main to it is a fast-forward.
	ghPatch(tc, "/repos/twin-bot/gitdata-repo/git/refs/heads/main", map[string]any{"sha": c["sha"]}).AssertStatus(200)
	if mainSHA(tc, "gitdata-repo") != c["sha"] {
		t.Error("main should have moved")
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
