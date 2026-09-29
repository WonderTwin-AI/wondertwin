package api_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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
	adminHandler.SetFlusher(handler)
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
	me := ghGet(tc, "/user").AssertStatus(200).JSONMap()
	assertRequired(t, "private-user", me)
	if me["login"] != "twin-bot" {
		t.Error("an unregistered well-formed token is the default user")
	}
}

// --- GitHub App Tests ---

// appJWT builds the JWT a GitHub App signs. The emulator checks the claims,
// not the signature, so the signature segment is a placeholder.
func appJWT(iss any, exp time.Time) string {
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return enc(map[string]string{"alg": "RS256", "typ": "JWT"}) + "." +
		enc(map[string]any{"iat": time.Now().Add(-30 * time.Second).Unix(), "exp": exp.Unix(), "iss": iss}) + ".c2lnbmF0dXJl"
}

func bearer(tok string) map[string]string { return map[string]string{"Authorization": "Bearer " + tok} }

func TestAppInstallationTokenFlow(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "installed")

	// The JWT's iss is the app's ID; find it with a client-id JWT first.
	app := tc.DoWithHeaders("GET", "/app", nil, bearer(appJWT("1000001", time.Now().Add(9*time.Minute)))).AssertStatus(200).JSONMap()
	assertRequired(t, "integration", app)
	jwt := appJWT(app["id"], time.Now().Add(9*time.Minute))
	if tc.DoWithHeaders("GET", "/app", nil, bearer(appJWT(app["client_id"], time.Now().Add(9*time.Minute)))).StatusCode != 200 {
		t.Error("the client ID is also a valid issuer")
	}

	var installs []map[string]any
	tc.DoWithHeaders("GET", "/app/installations", nil, bearer(jwt)).AssertStatus(200).JSON(&installs)
	if len(installs) != 1 || installs[0]["app_id"] != app["id"] {
		t.Fatalf("expected one installation of the app, got %v", installs)
	}
	assertRequired(t, "installation", installs[0])

	tokPath := fmt.Sprintf("/app/installations/%d/access_tokens", int64(installs[0]["id"].(float64)))
	minted := tc.DoWithHeaders("POST", tokPath, nil, bearer(jwt)).AssertStatus(201).JSONMap()
	assertRequired(t, "installation-token", minted)
	tok := minted["token"].(string)
	if !strings.HasPrefix(tok, "ghs_") || len(tok) != 40 {
		t.Errorf("unexpected token format %q", tok)
	}

	repos := tc.DoWithHeaders("GET", "/installation/repositories", nil, bearer(tok)).AssertStatus(200).JSONMap()
	if repos["total_count"].(float64) != 1 || repos["repository_selection"] != "all" {
		t.Errorf("unexpected installation repositories %v", repos)
	}
	issue := tc.DoWithHeaders("POST", "/repos/twin-bot/installed/issues", map[string]any{"title": "from the app"}, bearer(tok)).AssertStatus(201).JSONMap()
	if u := issue["user"].(map[string]any); u["login"] != "wondertwin-app[bot]" || u["type"] != "Bot" {
		t.Errorf("an installation token acts as the app's bot, got %v", u)
	}

	// Wrong credential for each route.
	ghGet(tc, "/installation/repositories").AssertStatus(403)
	ghGet(tc, "/app").AssertStatus(401)
	tc.DoWithHeaders("GET", "/user", nil, bearer(jwt)).AssertStatus(403)
	tc.DoWithHeaders("GET", "/app", nil, bearer(appJWT(app["id"], time.Now().Add(-time.Minute)))).AssertStatus(401)
	tc.DoWithHeaders("GET", "/app", nil, bearer(appJWT(app["id"], time.Now().Add(time.Hour)))).AssertStatus(401)
	tc.DoWithHeaders("GET", "/app", nil, bearer(appJWT("999", time.Now().Add(time.Minute)))).AssertStatus(401)
	tc.DoWithHeaders("POST", "/app/installations/1/access_tokens", nil, bearer(jwt)).AssertStatus(404)
	tc.DoWithHeaders("GET", "/installation/repositories", nil, bearer("ghs_notMintedByTheEmulator000000000000")).AssertStatus(401)

	// Installation tokens expire after an hour.
	testutil.NewAdminClient(tc).AdvanceTime("61m").AssertStatus(200)
	tc.DoWithHeaders("GET", "/installation/repositories", nil, bearer(tok)).AssertStatus(401)
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

// openPR branches feature from main, commits a file to it and opens a pull
// request, the way a client has to on GitHub.
func openPR(t *testing.T, tc *testutil.TwinClient, repo, title string) int {
	t.Helper()
	ghPost(tc, "/repos/twin-bot/"+repo+"/git/refs", map[string]any{"ref": "refs/heads/feature", "sha": mainSHA(tc, repo)}).AssertStatus(201)
	ghPut(tc, "/repos/twin-bot/"+repo+"/contents/CHANGELOG.md", map[string]any{
		"message": "add changelog", "content": "IyBDaGFuZ2Vsb2cK", "branch": "feature",
	}).AssertStatus(201)
	resp := ghPost(tc, "/repos/twin-bot/"+repo+"/pulls", map[string]any{"title": title, "head": "feature", "base": "main"})
	resp.AssertStatus(201)
	return int(resp.JSONMap()["number"].(float64))
}

func reviewerHeaders(tc *testutil.TwinClient) map[string]string {
	testutil.NewAdminClient(tc).LoadState(map[string]any{
		"tokens": map[string]any{"ghp_reviewer": map[string]any{"token": "ghp_reviewer", "login": "reviewer", "kind": "user"}},
	}).AssertStatus(200)
	return map[string]string{"Authorization": "Bearer ghp_reviewer"}
}

func TestCreateAndMergePR(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "pr-repo")

	// A branch with nothing new cannot be proposed.
	ghPost(tc, "/repos/twin-bot/pr-repo/pulls", map[string]any{"title": "x", "head": "main", "base": "main"}).AssertStatus(422)
	ghPost(tc, "/repos/twin-bot/pr-repo/pulls", map[string]any{"title": "x", "head": "nope", "base": "main"}).AssertStatus(422)

	num := openPR(t, tc, "pr-repo", "Add feature")
	pr := ghGet(tc, fmt.Sprintf("/repos/twin-bot/pr-repo/pulls/%d", num)).AssertStatus(200).JSONMap()
	assertRequired(t, "pull-request", pr)
	if _, ok := pr["merge_commit_sha"]; ok {
		t.Error("2026-03-10 removes merge_commit_sha")
	}
	if pr["state"] != "open" || pr["commits"].(float64) != 1 || pr["changed_files"].(float64) != 1 || pr["mergeable"] != true {
		t.Errorf("unexpected pull request %v %v %v %v", pr["state"], pr["commits"], pr["changed_files"], pr["mergeable"])
	}
	headSHA := pr["head"].(map[string]any)["sha"].(string)

	var list []map[string]any
	ghGet(tc, "/repos/twin-bot/pr-repo/pulls").JSON(&list)
	assertRequired(t, "pull-request-simple", list[0])

	// The issues API lists the pull request in its issue form.
	var issues []map[string]any
	ghGet(tc, "/repos/twin-bot/pr-repo/issues").JSON(&issues)
	if len(issues) != 1 || issues[0]["pull_request"] == nil {
		t.Errorf("expected the pull request among issues, got %v", issues)
	}

	ghPost(tc, "/repos/twin-bot/pr-repo/pulls", map[string]any{"title": "again", "head": "feature", "base": "main"}).AssertStatus(422)
	ghPut(tc, fmt.Sprintf("/repos/twin-bot/pr-repo/pulls/%d/merge", num), map[string]any{"sha": "0000000000000000000000000000000000000000"}).AssertStatus(409)

	resp := ghPut(tc, fmt.Sprintf("/repos/twin-bot/pr-repo/pulls/%d/merge", num), map[string]any{"merge_method": "squash", "sha": headSHA})
	resp.AssertStatus(200)
	merge := resp.JSONMap()
	if merge["merged"] != true {
		t.Error("expected merged=true")
	}
	if mainSHA(tc, "pr-repo") != merge["sha"] {
		t.Error("the merge commit becomes the head of main")
	}
	ghGet(tc, "/repos/twin-bot/pr-repo/contents/CHANGELOG.md").AssertStatus(200)
	ghGet(tc, fmt.Sprintf("/repos/twin-bot/pr-repo/pulls/%d/merge", num)).AssertStatus(204)
	after := ghGet(tc, fmt.Sprintf("/repos/twin-bot/pr-repo/pulls/%d", num)).JSONMap()
	if after["state"] != "closed" || after["merged"] != true || after["merged_by"] == nil {
		t.Errorf("unexpected merged pull request %v %v %v", after["state"], after["merged"], after["merged_by"])
	}
	ghPut(tc, fmt.Sprintf("/repos/twin-bot/pr-repo/pulls/%d/merge", num), nil).AssertStatus(405)
}

func TestMergeMethodsKeepHistory(t *testing.T) {
	for _, method := range []string{"merge", "rebase"} {
		_, tc := setupGitHub(t)
		createRepo(tc, "m")
		base := mainSHA(tc, "m")
		num := openPR(t, tc, "m", method)
		merged := ghPut(tc, fmt.Sprintf("/repos/twin-bot/m/pulls/%d/merge", num), map[string]any{"merge_method": method}).AssertStatus(200).JSONMap()
		c := ghGet(tc, "/repos/twin-bot/m/git/commits/"+merged["sha"].(string)).AssertStatus(200).JSONMap()
		parents := c["parents"].([]any)
		if method == "merge" && len(parents) != 2 {
			t.Errorf("a merge commit has two parents, got %d", len(parents))
		}
		if method == "rebase" && (len(parents) != 1 || parents[0].(map[string]any)["sha"] != base) {
			t.Errorf("a rebased commit sits on the old main, got %v", parents)
		}
	}
}

func TestMergeConflictIsRefused(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "conflict")
	num := openPR(t, tc, "conflict", "Conflicts")
	ghPut(tc, "/repos/twin-bot/conflict/contents/CHANGELOG.md", map[string]any{"message": "main edit", "content": "eAo="}).AssertStatus(201)
	pr := ghGet(tc, fmt.Sprintf("/repos/twin-bot/conflict/pulls/%d", num)).JSONMap()
	if pr["mergeable"] != false || pr["mergeable_state"] != "dirty" {
		t.Errorf("expected a dirty pull request, got %v %v", pr["mergeable"], pr["mergeable_state"])
	}
	ghPut(tc, fmt.Sprintf("/repos/twin-bot/conflict/pulls/%d/merge", num), nil).AssertStatus(405)
}

// --- Commit Status Tests ---

func TestCommitStatuses(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "status-repo")
	sha := mainSHA(tc, "status-repo")
	path := "/repos/twin-bot/status-repo/statuses/"

	ghPost(tc, path+"abc123", map[string]any{"state": "success"}).AssertStatus(422)
	ghPost(tc, path+sha, map[string]any{"state": "green"}).AssertStatus(422)

	st := ghPost(tc, path+sha, map[string]any{"state": "pending", "context": "ci/tests"}).AssertStatus(201).JSONMap()
	assertRequired(t, "status", st)
	ghPost(tc, path+sha, map[string]any{"state": "success", "context": "ci/tests"}).AssertStatus(201)
	ghPost(tc, path+sha, map[string]any{"state": "success", "context": "ci/lint"}).AssertStatus(201)

	// The branch name resolves to the same commit.
	m := ghGet(tc, "/repos/twin-bot/status-repo/commits/main/status").AssertStatus(200).JSONMap()
	assertRequired(t, "combined-commit-status", m)
	assertRequired(t, "simple-commit-status", m["statuses"].([]any)[0].(map[string]any))
	assertRequired(t, "minimal-repository", m["repository"].(map[string]any))
	if m["state"] != "success" || m["total_count"].(float64) != 2 || m["sha"] != sha {
		t.Errorf("only the newest status per context counts; got %v with %v statuses", m["state"], m["total_count"])
	}
	ghPost(tc, path+sha, map[string]any{"state": "failure", "context": "ci/lint"}).AssertStatus(201)
	if s := ghGet(tc, "/repos/twin-bot/status-repo/commits/"+sha+"/status").JSONMap()["state"]; s != "failure" {
		t.Errorf("a failing context fails the commit, got %v", s)
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

func TestReleasePublishAndLatest(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "rel")
	path := "/repos/twin-bot/rel/releases"
	admin := testutil.NewAdminClient(tc)

	v1 := ghPost(tc, path, map[string]any{"tag_name": "v1.0.0", "name": "v1.0.0", "body": "first"}).AssertStatus(201).JSONMap()
	assertRequired(t, "release", v1)
	ghPost(tc, path, map[string]any{"tag_name": "v1.0.0"}).AssertStatus(422)
	ghPost(tc, path, map[string]any{"name": "no tag"}).AssertStatus(422)

	// Publishing creates the tag at the target.
	tag := ghGet(tc, "/repos/twin-bot/rel/git/ref/tags/v1.0.0").AssertStatus(200).JSONMap()
	if tag["object"].(map[string]any)["sha"] != mainSHA(tc, "rel") {
		t.Errorf("the tag points at main, got %v", tag["object"])
	}

	admin.AdvanceTime("1m").AssertStatus(200)
	ghPost(tc, path, map[string]any{"tag_name": "v2.0.0-rc1", "prerelease": true}).AssertStatus(201)
	ghPost(tc, path, map[string]any{"tag_name": "v2.0.0", "draft": true}).AssertStatus(201)

	latest := ghGet(tc, path+"/latest").AssertStatus(200).JSONMap()
	if latest["tag_name"] != "v1.0.0" {
		t.Errorf("drafts and prereleases are never latest, got %v", latest["tag_name"])
	}
	// latest sorts by the date of the release's commit, not of the release:
	// a release made later for an older commit does not become latest.
	admin.AdvanceTime("1m").AssertStatus(200)
	ghPut(tc, "/repos/twin-bot/rel/contents/NEWS.md", map[string]any{"message": "news", "content": "eAo="}).AssertStatus(201)
	ghPost(tc, path, map[string]any{"tag_name": "v1.1.0"}).AssertStatus(201)
	admin.AdvanceTime("1m").AssertStatus(200)
	ghPost(tc, path, map[string]any{"tag_name": "v0.9.0", "target_commitish": tag["object"].(map[string]any)["sha"]}).AssertStatus(201)
	if got := ghGet(tc, path+"/latest").JSONMap()["tag_name"]; got != "v1.1.0" {
		t.Errorf("latest follows the newest commit, got %v", got)
	}
	if got := ghGet(tc, path+"/tags/v1.0.0").AssertStatus(200).JSONMap()["id"]; got != v1["id"] {
		t.Errorf("tag lookup returned %v", got)
	}

	// The upload URL points at the emulator and accepts the file body.
	upload := strings.TrimSuffix(v1["upload_url"].(string), "{?name,label}")
	if !strings.HasPrefix(upload, "http://127.0.0.1:") {
		t.Fatalf("upload_url should point at the emulator, got %s", upload)
	}
	asset := ghPost(tc, strings.TrimPrefix(upload, tc.BaseURL)+"?name=app.tar.gz", "binary").AssertStatus(201).JSONMap()
	assertRequired(t, "release-asset", asset)
	if len(ghGet(tc, fmt.Sprintf("%s/%d", path, int64(v1["id"].(float64)))).JSONMap()["assets"].([]any)) != 1 {
		t.Error("the asset is listed on its release")
	}
}

// --- Webhook Tests ---

// hookReceiver records the webhook deliveries it is sent.
type hookReceiver struct {
	mu   sync.Mutex
	got  []*http.Request
	body [][]byte
	srv  *httptest.Server
}

func newHookReceiver(t *testing.T, status int) *hookReceiver {
	hr := &hookReceiver{}
	hr.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		hr.mu.Lock()
		hr.got = append(hr.got, r)
		hr.body = append(hr.body, b)
		hr.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(hr.srv.Close)
	return hr
}

func (hr *hookReceiver) events() []string {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	var out []string
	for i, r := range hr.got {
		var p map[string]any
		_ = json.Unmarshal(hr.body[i], &p)
		e := r.Header.Get("X-GitHub-Event")
		if a, ok := p["action"].(string); ok {
			e += "." + a
		}
		out = append(out, e)
	}
	return out
}

func TestWebhooks(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "hook-repo")
	recv := newHookReceiver(t, 200)

	ghPost(tc, "/repos/twin-bot/hook-repo/hooks", map[string]any{"config": map[string]any{}}).AssertStatus(422)
	resp := ghPost(tc, "/repos/twin-bot/hook-repo/hooks", map[string]any{
		"events": []string{"issues", "push"},
		"config": map[string]any{"url": recv.srv.URL, "content_type": "json", "secret": "It's a Secret to Everybody"},
	})
	resp.AssertStatus(201)
	hook := resp.JSONMap()
	assertRequired(t, "hook", hook)
	if hook["config"].(map[string]any)["secret"] != "********" {
		t.Error("the secret is never echoed back")
	}
	hookID := int64(hook["id"].(float64))
	// The same config may be reused only for events that do not overlap.
	ghPost(tc, "/repos/twin-bot/hook-repo/hooks", map[string]any{"events": []string{"push"}, "config": map[string]any{"url": recv.srv.URL}}).AssertStatus(422)
	other := ghPost(tc, "/repos/twin-bot/hook-repo/hooks", map[string]any{"events": []string{"release"}, "config": map[string]any{"url": recv.srv.URL}}).AssertStatus(201).JSONMap()
	ghDelete(tc, fmt.Sprintf("/repos/twin-bot/hook-repo/hooks/%d", int64(other["id"].(float64)))).AssertStatus(204)

	ghPost(tc, "/repos/twin-bot/hook-repo/issues", map[string]any{"title": "hooked"}).AssertStatus(201)
	ghPut(tc, "/repos/twin-bot/hook-repo/contents/a.txt", map[string]any{"message": "a", "content": "YQo="}).AssertStatus(201)
	testutil.NewAdminClient(tc).FlushWebhooks().AssertStatus(200)

	// Two pings (one per hook created), then the issue and the push.
	if got := strings.Join(recv.events(), ","); got != "ping,ping,issues.opened,push" {
		t.Fatalf("expected two pings, issues.opened and push in order, got %s", got)
	}
	recv.mu.Lock()
	req, body := recv.got[2], recv.body[2]
	pushBody := recv.body[3]
	recv.mu.Unlock()

	// X-Hub-Signature-256 is GitHub's HMAC-SHA256 of the exact body.
	mac := hmac.New(sha256.New, []byte("It's a Secret to Everybody"))
	mac.Write(body)
	if want := "sha256=" + hex.EncodeToString(mac.Sum(nil)); req.Header.Get("X-Hub-Signature-256") != want {
		t.Errorf("signature %q, want %q", req.Header.Get("X-Hub-Signature-256"), want)
	}
	for _, hdr := range []string{"X-GitHub-Delivery", "X-GitHub-Hook-ID", "X-GitHub-Hook-Installation-Target-ID", "X-Hub-Signature"} {
		if req.Header.Get(hdr) == "" {
			t.Errorf("missing %s", hdr)
		}
	}
	if !strings.HasPrefix(req.Header.Get("User-Agent"), "GitHub-Hookshot/") || req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("unexpected agent or content type: %q %q", req.Header.Get("User-Agent"), req.Header.Get("Content-Type"))
	}
	var issuePayload map[string]any
	_ = json.Unmarshal(body, &issuePayload)
	for _, k := range []string{"action", "issue", "repository", "sender"} {
		if issuePayload[k] == nil {
			t.Errorf("issues payload lacks %s", k)
		}
	}
	var push map[string]any
	_ = json.Unmarshal(pushBody, &push)
	if push["ref"] != "refs/heads/main" || len(push["commits"].([]any)) != 1 || push["head_commit"] == nil {
		t.Errorf("unexpected push payload %v", push)
	}

	var deliveries []map[string]any
	ghGet(tc, fmt.Sprintf("/repos/twin-bot/hook-repo/hooks/%d/deliveries", hookID)).AssertStatus(200).JSON(&deliveries)
	if len(deliveries) != 3 || deliveries[0]["event"] != "push" || deliveries[0]["status_code"].(float64) != 200 {
		t.Errorf("deliveries are listed newest first, got %v", deliveries)
	}
	assertRequired(t, "hook-delivery-item", deliveries[0])

	ghDelete(tc, fmt.Sprintf("/repos/twin-bot/hook-repo/hooks/%d", hookID)).AssertStatus(204)
}

func TestWebhookFailureIsNotRetried(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "hook-fail")
	recv := newHookReceiver(t, 500)
	hook := ghPost(tc, "/repos/twin-bot/hook-fail/hooks", map[string]any{
		"events": []string{"issues"}, "config": map[string]any{"url": recv.srv.URL, "content_type": "form"},
	}).AssertStatus(201).JSONMap()
	testutil.NewAdminClient(tc).FlushWebhooks().AssertStatus(200)

	if n := len(recv.events()); n != 1 {
		t.Errorf("GitHub does not retry a failed delivery; expected 1 attempt, got %d", n)
	}
	recv.mu.Lock()
	form := string(recv.body[0])
	ctype := recv.got[0].Header.Get("Content-Type")
	recv.mu.Unlock()
	if !strings.HasPrefix(form, "payload=") || ctype != "application/x-www-form-urlencoded" {
		t.Errorf("content_type form sends payload=<json>, got %q as %q", form[:min(20, len(form))], ctype)
	}
	got := ghGet(tc, fmt.Sprintf("/repos/twin-bot/hook-fail/hooks/%d", int64(hook["id"].(float64)))).JSONMap()
	if lr := got["last_response"].(map[string]any); lr["code"].(float64) != 500 || lr["status"] != "failed" {
		t.Errorf("last_response should record the failure, got %v", lr)
	}
}

func TestPullRequestAndCheckRunEvents(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "hook-events")
	recv := newHookReceiver(t, 200)
	ghPost(tc, "/repos/twin-bot/hook-events/hooks", map[string]any{
		"events": []string{"pull_request", "check_run"}, "config": map[string]any{"url": recv.srv.URL, "content_type": "json"},
	}).AssertStatus(201)

	num := openPR(t, tc, "hook-events", "Evented")
	app := bearer(installationToken(t, tc))
	sha := ghGet(tc, fmt.Sprintf("/repos/twin-bot/hook-events/pulls/%d", num)).JSONMap()["head"].(map[string]any)["sha"]
	tc.DoWithHeaders("POST", "/repos/twin-bot/hook-events/check-runs", map[string]any{"name": "ci", "head_sha": sha, "conclusion": "success"}, app).AssertStatus(201)
	ghPut(tc, fmt.Sprintf("/repos/twin-bot/hook-events/pulls/%d/merge", num), nil).AssertStatus(200)
	testutil.NewAdminClient(tc).FlushWebhooks().AssertStatus(200)

	want := "ping,pull_request.opened,check_run.created,check_run.completed,pull_request.closed"
	if got := strings.Join(recv.events(), ","); got != want {
		t.Errorf("events = %s, want %s", got, want)
	}
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
	num := openPR(t, tc, "review-repo", "Review me")
	path := fmt.Sprintf("/repos/twin-bot/review-repo/pulls/%d/reviews", num)

	own := ghPost(tc, path, map[string]any{"body": "LGTM", "event": "APPROVE"})
	own.AssertStatus(422)
	if errs := own.JSONMap()["errors"].([]any); errs[0] != "Can not approve your own pull request" {
		t.Errorf("unexpected refusal %v", errs)
	}

	resp := tc.DoWithHeaders("POST", path, map[string]any{"body": "LGTM", "event": "APPROVE"}, reviewerHeaders(tc))
	resp.AssertStatus(200)
	review := resp.JSONMap()
	assertRequired(t, "pull-request-review", review)
	if review["state"] != "APPROVED" || review["user"].(map[string]any)["login"] != "reviewer" {
		t.Errorf("unexpected review %v by %v", review["state"], review["user"])
	}

	var reviews []map[string]any
	json.Unmarshal(ghGet(tc, path).Body, &reviews)
	if len(reviews) != 1 {
		t.Errorf("expected 1 review, got %d", len(reviews))
	}
	id := int64(review["id"].(float64))
	ghPut(tc, fmt.Sprintf("%s/%d", path, id), map[string]any{"body": "Still good"}).AssertStatus(200)
	ghPatch(tc, fmt.Sprintf("%s/%d", path, id), map[string]any{"body": "x"}).AssertStatus(404)
}

// --- PR Review Comment Tests ---

func TestPRReviewComments(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "rc-repo")

	num := openPR(t, tc, "rc-repo", "Comment me")

	resp := ghPost(tc, fmt.Sprintf("/repos/twin-bot/rc-repo/pulls/%d/comments", num), map[string]any{
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

// installationToken mints a token for the seeded App's installation.
func installationToken(t *testing.T, tc *testutil.TwinClient) string {
	t.Helper()
	jwt := appJWT("1000001", time.Now().Add(9*time.Minute))
	var installs []map[string]any
	tc.DoWithHeaders("GET", "/app/installations", nil, bearer(jwt)).AssertStatus(200).JSON(&installs)
	path := fmt.Sprintf("/app/installations/%d/access_tokens", int64(installs[0]["id"].(float64)))
	return tc.DoWithHeaders("POST", path, nil, bearer(jwt)).AssertStatus(201).JSONMap()["token"].(string)
}

func TestCheckRuns(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "check-repo")
	sha := mainSHA(tc, "check-repo")
	app := bearer(installationToken(t, tc))
	path := "/repos/twin-bot/check-repo/check-runs"

	// Check-run writes are GitHub App only.
	ghPost(tc, path, map[string]any{"name": "ci/test", "head_sha": sha}).AssertStatus(403)
	tc.DoWithHeaders("POST", path, map[string]any{"name": "ci/test", "head_sha": "abc123"}, app).AssertStatus(422)
	tc.DoWithHeaders("POST", path, map[string]any{"name": "ci/test", "head_sha": sha, "status": "completed"}, app).AssertStatus(422)

	resp := tc.DoWithHeaders("POST", path, map[string]any{"name": "ci/test", "head_sha": sha, "status": "in_progress"}, app)
	resp.AssertStatus(201)
	cr := resp.JSONMap()
	assertRequired(t, "check-run", cr)
	if cr["app"].(map[string]any)["slug"] != "wondertwin-app" || cr["completed_at"] != nil {
		t.Errorf("unexpected check run %v %v", cr["app"], cr["completed_at"])
	}
	crID := int64(cr["id"].(float64))

	done := tc.DoWithHeaders("PATCH", fmt.Sprintf("%s/%d", path, crID), map[string]any{"name": "", "conclusion": "success"}, app).AssertStatus(200).JSONMap()
	if done["name"] != "ci/test" {
		t.Errorf("an empty name on update keeps the name, got %v", done["name"])
	}
	if done["status"] != "completed" || done["completed_at"] == nil {
		t.Errorf("a conclusion completes the run, got %v %v", done["status"], done["completed_at"])
	}
	if ghGet(tc, fmt.Sprintf("%s/%d", path, crID)).AssertStatus(200).JSONMap()["conclusion"] != "success" {
		t.Error("expected conclusion=success")
	}

	m := ghGet(tc, "/repos/twin-bot/check-repo/commits/main/check-runs").AssertStatus(200).JSONMap()
	runs := m["check_runs"].([]any)
	if m["total_count"].(float64) != 1 || runs[0].(map[string]any)["conclusion"] != "success" {
		t.Errorf("expected the completed run, got %v", m)
	}
	suite := ghGet(tc, fmt.Sprintf("/repos/twin-bot/check-repo/check-suites/%d", int64(cr["check_suite"].(map[string]any)["id"].(float64)))).AssertStatus(200).JSONMap()
	if suite["status"] != "completed" || suite["conclusion"] != "success" {
		t.Errorf("the suite follows its runs, got %v %v", suite["status"], suite["conclusion"])
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

func commitWorkflow(tc *testutil.TwinClient, repo, file, yaml string) {
	ghPut(tc, "/repos/twin-bot/"+repo+"/contents/.github/workflows/"+file, map[string]any{
		"message": "add " + file, "content": base64.StdEncoding.EncodeToString([]byte(yaml)),
	}).AssertStatus(201)
}

func TestActionsWorkflowDispatch(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "actions-repo")
	commitWorkflow(tc, "actions-repo", "deploy.yml", "name: Deploy\non:\n  workflow_dispatch:\n    inputs:\n      env:\n        type: string\n")
	commitWorkflow(tc, "actions-repo", "ci.yml", "name: CI\non: [push]\n")

	wfs := ghGet(tc, "/repos/twin-bot/actions-repo/actions/workflows").AssertStatus(200).JSONMap()
	if wfs["total_count"].(float64) != 2 {
		t.Fatalf("committing workflow files registers them, got %v", wfs)
	}

	base := "/repos/twin-bot/actions-repo/actions/workflows/"
	ghPost(tc, base+"ci.yml/dispatches", map[string]any{"ref": "main"}).AssertStatus(422)
	ghPost(tc, base+"deploy.yml/dispatches", map[string]any{"ref": "nope"}).AssertStatus(422)
	ghPost(tc, base+"missing.yml/dispatches", map[string]any{"ref": "main"}).AssertStatus(404)

	// 2026-03-10: 200 with the run's ID and URLs, not 204.
	resp := ghPost(tc, base+"deploy.yml/dispatches", map[string]any{"ref": "main", "inputs": map[string]any{"env": "staging"}})
	resp.AssertStatus(200)
	d := resp.JSONMap()
	assertRequired(t, "workflow-dispatch-response", d)

	run := ghGet(tc, fmt.Sprintf("/repos/twin-bot/actions-repo/actions/runs/%d", int64(d["workflow_run_id"].(float64)))).AssertStatus(200).JSONMap()
	assertRequired(t, "workflow-run", run)
	if run["status"] != "completed" || run["conclusion"] != "success" || run["event"] != "workflow_dispatch" || run["head_sha"] != mainSHA(tc, "actions-repo") {
		t.Errorf("a dispatched run completes at once on the ref's head, got %v %v %v", run["status"], run["conclusion"], run["event"])
	}

	m := ghGet(tc, base+"deploy.yml/runs").AssertStatus(200).JSONMap()
	if m["total_count"].(float64) != 1 {
		t.Errorf("expected 1 run, got %v", m["total_count"])
	}
}

func TestActionsRunCancel(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "cancel-repo")
	commitWorkflow(tc, "cancel-repo", "deploy.yml", "on: workflow_dispatch\n")
	d := ghPost(tc, "/repos/twin-bot/cancel-repo/actions/workflows/deploy.yml/dispatches", map[string]any{"ref": "main"}).AssertStatus(200).JSONMap()
	runPath := fmt.Sprintf("/repos/twin-bot/cancel-repo/actions/runs/%d", int64(d["workflow_run_id"].(float64)))

	// The run is already complete, so there is nothing to cancel.
	ghPost(tc, runPath+"/cancel", nil).AssertStatus(409)
	ghPost(tc, runPath+"/rerun", nil).AssertStatus(201)
	if got := ghGet(tc, runPath).JSONMap()["run_attempt"]; got.(float64) != 2 {
		t.Errorf("a rerun is a second attempt, got %v", got)
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
	assertRequired(t, "git-ref", got)
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

func TestEscapedSlashesInPathParameters(t *testing.T) {
	_, tc := setupGitHub(t)
	createRepo(tc, "escaped")
	// Octokit escapes the slash inside {ref} and {path}.
	ghGet(tc, "/repos/twin-bot/escaped/git/ref/heads%2Fmain").AssertStatus(200)
	ghPut(tc, "/repos/twin-bot/escaped/contents/docs%2Fguide.md", map[string]any{"message": "doc", "content": "eAo="}).AssertStatus(201)
	file := ghGet(tc, "/repos/twin-bot/escaped/contents/docs/guide.md").AssertStatus(200).JSONMap()
	if file["path"] != "docs/guide.md" {
		t.Errorf("the escaped path is stored decoded, got %v", file["path"])
	}
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
	assertRequired(t, "file-commit", put)
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

	assertRequired(t, "short-branch", branches[0])
	file := ghGet(tc, "/repos/twin-bot/boot/contents/README.md").AssertStatus(200).JSONMap()
	assertRequired(t, "content-file", file)
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
