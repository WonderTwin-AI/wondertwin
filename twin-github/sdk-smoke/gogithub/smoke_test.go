//go:build sdksmoke

// Package gogithub drives the GitHub app emulator's MVP use cases through
// google/go-github, a third-party Go client (GitHub's own libraries page
// lists it as community-maintained). It is test-only and sits outside the
// repository's Go module: run.sh copies it next to go-github.mod and
// go-github.sum, which pin the client, and runs it against a live binary.
//
// go-github sends X-GitHub-Api-Version: 2022-11-28 on every request by
// default. The community app emulator serves 2026-03-10 only and refuses
// that value with 400, so the client here sets 2026-03-10 explicitly.
package gogithub

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v92/github"
)

const apiVersion = "2026-03-10"

var (
	baseURL = strings.TrimRight(os.Getenv("GITHUB_EMULATOR_URL"), "/")
	ctx     = context.Background()
)

// versionTransport pins the API version on every request.
type versionTransport struct {
	version string
	next    http.RoundTripper
}

func (t versionTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if t.version != "" {
		r.Header.Set("X-GitHub-Api-Version", t.version)
	}
	return t.next.RoundTrip(r)
}

func client(t *testing.T, token, version string) *github.Client {
	t.Helper()
	api := baseURL + "/"
	opts := []github.ClientOptionsFunc{
		github.WithURLs(&api, &api),
		github.WithTransport(versionTransport{version: version, next: http.DefaultTransport}),
	}
	if token != "" {
		opts = append(opts, github.WithAuthToken(token))
	}
	c, err := github.NewClient(opts...)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return c
}

// admin calls the emulator's admin API, which has no GitHub equivalent.
func admin(t *testing.T, method, path string, body any) map[string]any {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, baseURL+path, rd)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin %s: %v", path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

const (
	author   = "ghp_smokeAuthor000000000000000000000000"
	reviewer = "ghp_smokeReviewer0000000000000000000000"
)

// fresh resets the emulator and registers two principals, so a use case
// that needs a second user (a reviewer) has one.
func fresh(t *testing.T) {
	t.Helper()
	admin(t, "POST", "/admin/reset", nil)
	admin(t, "POST", "/admin/state", map[string]any{
		"users": map[string]any{
			"smoke-author":   map[string]any{"id": 190009001, "login": "smoke-author", "type": "User"},
			"smoke-reviewer": map[string]any{"id": 190009002, "login": "smoke-reviewer", "type": "User"},
		},
		"tokens": map[string]any{
			author:   map[string]any{"token": author, "login": "smoke-author", "kind": "user"},
			reviewer: map[string]any{"token": reviewer, "login": "smoke-reviewer", "kind": "user"},
		},
	})
}

// result carries an SDK call's value and error, so a call can be checked
// inline: res(gh.Repositories.Get(...)).v(t).
type result[T any] struct {
	val T
	err error
}

func res[T any](v T, _ *github.Response, err error) result[T] { return result[T]{v, err} }

func (r result[T]) v(t *testing.T) T {
	t.Helper()
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	return r.val
}

func check(t *testing.T, ok bool, format string, a ...any) {
	t.Helper()
	if !ok {
		t.Errorf(format, a...)
	}
}

// record appends the case's outcome to $SMOKE_RESULTS for run.sh's table
// as soon as the case ends.
func record(t *testing.T, name string) {
	t.Cleanup(func() {
		status := "PASS"
		if t.Failed() {
			status = "FAIL"
		}
		f := os.Getenv("SMOKE_RESULTS")
		if f == "" {
			return
		}
		out, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		fmt.Fprintf(out, "go-github\t%s\t%s\n", name, status)
		_ = out.Close()
	})
}

func TestMain(m *testing.M) {
	if baseURL == "" {
		fmt.Fprintln(os.Stderr, "GITHUB_EMULATOR_URL is not set")
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func bootstrap(t *testing.T, gh *github.Client, name string) (owner string, commit string) {
	t.Helper()
	repo := res(gh.Repositories.Create(ctx, "", &github.Repository{Name: github.Ptr(name)})).v(t)
	owner = repo.GetOwner().GetLogin()
	put := res(gh.Repositories.CreateFile(ctx, owner, name, "README.md", &github.RepositoryContentFileOptions{
		Message: github.Ptr("init"), Content: []byte("# hello\n"),
	})).v(t)
	return owner, put.GetSHA()
}

func TestSmoke(t *testing.T) {
	t.Run("repo-bootstrap", func(t *testing.T) {
		record(t, "repo-bootstrap")
		fresh(t)
		gh := client(t, author, apiVersion)
		repo := res(gh.Repositories.Create(ctx, "", &github.Repository{Name: github.Ptr("boot")})).v(t)
		check(t, repo.GetID() > 1_000_000_000 && strings.HasPrefix(repo.GetNodeID(), "R_"), "unexpected id %d / node_id %s", repo.GetID(), repo.GetNodeID())
		put := res(gh.Repositories.CreateFile(ctx, "smoke-author", "boot", "README.md", &github.RepositoryContentFileOptions{
			Message: github.Ptr("init"), Content: []byte("# hello\n"),
		})).v(t)
		branches := res(gh.Repositories.ListBranches(ctx, "smoke-author", "boot", nil)).v(t)
		check(t, len(branches) == 1 && branches[0].GetCommit().GetSHA() == put.GetSHA(), "main should be at the new commit")
		file, _, _, err := gh.Repositories.GetContents(ctx, "smoke-author", "boot", "README.md", nil)
		if err != nil {
			t.Fatal(err)
		}
		text, _ := file.GetContent()
		check(t, text == "# hello\n" && file.GetSHA() == put.Content.GetSHA(), "read back %q", text)
		got := res(gh.Repositories.Get(ctx, "smoke-author", "boot")).v(t)
		check(t, got.GetID() == repo.GetID(), "repository id changed")
		_, _, err = gh.Repositories.Create(ctx, "", &github.Repository{Name: github.Ptr("boot")})
		var er *github.ErrorResponse
		check(t, errors.As(err, &er) && er.Response.StatusCode == 422, "a duplicate name is refused with 422, got %v", err)
	})

	t.Run("issue-triage", func(t *testing.T) {
		record(t, "issue-triage")
		fresh(t)
		gh := client(t, author, apiVersion)
		owner, _ := bootstrap(t, gh, "triage")
		res(gh.Issues.CreateLabel(ctx, owner, "triage", github.CreateIssueLabelRequest{Name: "triage", Color: github.Ptr("fbca04")})).v(t)
		for i := 0; i < 3; i++ {
			res(gh.Issues.Create(ctx, owner, "triage", github.CreateIssueRequest{Title: fmt.Sprintf("older %d", i)})).v(t)
		}
		issue := res(gh.Issues.Create(ctx, owner, "triage", github.CreateIssueRequest{Title: "Build fails on main"})).v(t)
		res(gh.Issues.AddLabelsToIssue(ctx, owner, "triage", issue.GetNumber(), []string{"triage"})).v(t)
		res(gh.Issues.CreateComment(ctx, owner, "triage", issue.GetNumber(), github.IssueCommentRequest{Body: "Reproduced."})).v(t)

		var all []*github.Issue
		opt := &github.IssueListByRepoOptions{ListOptions: github.ListOptions{PerPage: 2}}
		pages := 0
		for {
			page, resp, err := gh.Issues.ListByRepo(ctx, owner, "triage", opt)
			if err != nil {
				t.Fatal(err)
			}
			all, pages = append(all, page...), pages+1
			if resp.NextPage == 0 {
				break
			}
			opt.ListOptions.Page = resp.NextPage
		}
		check(t, len(all) == 4 && pages == 2, "expected 4 issues over 2 pages, got %d over %d", len(all), pages)

		res(gh.Issues.Update(ctx, owner, "triage", issue.GetNumber(), github.UpdateIssueRequest{State: github.Ptr("closed"), StateReason: github.Ptr("completed")})).v(t)
		got := res(gh.Issues.Get(ctx, owner, "triage", issue.GetNumber())).v(t)
		check(t, got.GetState() == "closed" && len(got.Labels) == 1 && got.Labels[0].GetName() == "triage" && got.GetComments() == 1,
			"issue reads back %s with %d labels and %d comments", got.GetState(), len(got.Labels), got.GetComments())
		check(t, got.GetID() > 1<<31, "issue IDs exceed int32, got %d", got.GetID())
	})

	t.Run("pr-lifecycle", func(t *testing.T) {
		record(t, "pr-lifecycle")
		fresh(t)
		gh := client(t, author, apiVersion)
		owner, _ := bootstrap(t, gh, "prs")
		main := res(gh.Git.GetRef(ctx, owner, "prs", "heads/main")).v(t)
		res(gh.Git.CreateRef(ctx, owner, "prs", github.CreateRef{Ref: "refs/heads/feature", SHA: main.GetObject().GetSHA()})).v(t)
		put := res(gh.Repositories.CreateFile(ctx, owner, "prs", "CHANGELOG.md", &github.RepositoryContentFileOptions{
			Message: github.Ptr("add changelog"), Content: []byte("# Changelog\n"), Branch: github.Ptr("feature"),
		})).v(t)
		pr := res(gh.PullRequests.Create(ctx, owner, "prs", github.CreatePullRequest{
			Title: github.Ptr("Add changelog"), Head: "feature", Base: "main",
		})).v(t)
		check(t, pr.GetHead().GetSHA() == put.GetSHA(), "head sha is the branch commit")

		_, _, err := gh.PullRequests.CreateReview(ctx, owner, "prs", pr.GetNumber(), &github.PullRequestReviewRequest{Event: github.Ptr("APPROVE")})
		var er *github.ErrorResponse
		check(t, errors.As(err, &er) && er.Response.StatusCode == 422, "an author cannot approve their own pull request, got %v", err)
		rv := client(t, reviewer, apiVersion)
		review := res(rv.PullRequests.CreateReview(ctx, owner, "prs", pr.GetNumber(), &github.PullRequestReviewRequest{Event: github.Ptr("APPROVE")})).v(t)
		check(t, review.GetState() == "APPROVED", "review state %s", review.GetState())

		mr := res(gh.PullRequests.Merge(ctx, owner, "prs", pr.GetNumber(), "", &github.PullRequestOptions{MergeMethod: "squash"})).v(t)
		check(t, mr.GetMerged(), "merge result")
		merged, _, err := gh.PullRequests.IsMerged(ctx, owner, "prs", pr.GetNumber())
		check(t, err == nil && merged, "IsMerged: %v %v", merged, err)
		got := res(gh.PullRequests.Get(ctx, owner, "prs", pr.GetNumber())).v(t)
		check(t, got.GetState() == "closed" && got.GetMerged(), "pull request reads back %s merged=%v", got.GetState(), got.GetMerged())
	})

	t.Run("ci-status-and-checks", func(t *testing.T) {
		record(t, "ci-status-and-checks")
		fresh(t)
		gh := client(t, author, apiVersion)
		owner, sha := bootstrap(t, gh, "ci")
		res(gh.Repositories.CreateStatus(ctx, owner, "ci", sha, github.RepoStatus{State: github.Ptr("success"), Context: github.Ptr("ci/unit")})).v(t)
		combined := res(gh.Repositories.GetCombinedStatus(ctx, owner, "ci", sha, nil)).v(t)
		check(t, combined.GetState() == "success", "combined state %s", combined.GetState())

		_, _, err := gh.Checks.CreateCheckRun(ctx, owner, "ci", github.CreateCheckRunOptions{Name: "ci/integration", HeadSHA: sha})
		var er *github.ErrorResponse
		check(t, errors.As(err, &er) && er.Response.StatusCode == 403, "check runs are GitHub App only, got %v", err)

		inst := client(t, installationToken(t), apiVersion)
		run := res(inst.Checks.CreateCheckRun(ctx, owner, "ci", github.CreateCheckRunOptions{
			Name: "ci/integration", HeadSHA: sha, Status: github.Ptr("in_progress"),
		})).v(t)
		res(inst.Checks.UpdateCheckRun(ctx, owner, "ci", run.GetID(), github.UpdateCheckRunOptions{
			Name: "ci/integration", Conclusion: github.Ptr("success"),
		})).v(t)
		list := res(gh.Checks.ListCheckRunsForRef(ctx, owner, "ci", sha, nil)).v(t)
		check(t, list.GetTotal() == 1 && list.CheckRuns[0].GetConclusion() == "success" && list.CheckRuns[0].GetApp().GetSlug() != "",
			"expected one completed run with its app, got %d", list.GetTotal())
	})

	t.Run("app-installation-token", func(t *testing.T) {
		record(t, "app-installation-token")
		fresh(t)
		// The seeded App is installed on the default user's account; an
		// unregistered well-formed token acts as that user.
		bootstrap(t, client(t, "ghp_defaultUser000000000000000000000000", apiVersion), "app-repo")
		app := client(t, appJWT(t), apiVersion)
		me := res(app.Apps.Get(ctx, "")).v(t)
		installs := res(app.Apps.ListInstallations(ctx, nil)).v(t)
		check(t, len(installs) == 1 && installs[0].GetAppID() == me.GetID(), "expected the app's installation")
		tok := res(app.Apps.CreateInstallationToken(ctx, installs[0].GetID(), nil)).v(t)
		check(t, strings.HasPrefix(tok.GetToken(), "ghs_") && tok.GetExpiresAt().After(time.Now()), "token %q", tok.GetToken())
		repos := res(client(t, tok.GetToken(), apiVersion).Apps.ListRepos(ctx, nil)).v(t)
		check(t, repos.GetTotalCount() == 1 && repos.Repositories[0].GetName() == "app-repo", "installation repositories: %d", repos.GetTotalCount())
	})

	t.Run("release-publish", func(t *testing.T) {
		record(t, "release-publish")
		fresh(t)
		gh := client(t, author, apiVersion)
		owner, _ := bootstrap(t, gh, "rel")
		rel := res(gh.Repositories.CreateRelease(ctx, owner, "rel", github.CreateReleaseRequest{TagName: "v1.0.0", Name: github.Ptr("v1.0.0")})).v(t)
		latest := res(gh.Repositories.GetLatestRelease(ctx, owner, "rel")).v(t)
		byTag := res(gh.Repositories.GetReleaseByTag(ctx, owner, "rel", "v1.0.0")).v(t)
		check(t, latest.GetID() == rel.GetID() && byTag.GetID() == rel.GetID(), "latest and by-tag return the release")

		f, err := os.CreateTemp(t.TempDir(), "asset-*.txt")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.WriteString("artifact")
		_, _ = f.Seek(0, 0)
		asset := res(gh.Repositories.UploadReleaseAsset(ctx, owner, "rel", rel.GetID(), &github.UploadOptions{Name: "artifact.txt"}, f)).v(t)
		check(t, asset.GetSize() == 8, "asset size %d", asset.GetSize())
	})

	t.Run("api-version-negotiation", func(t *testing.T) {
		record(t, "api-version-negotiation")
		fresh(t)
		gh := client(t, author, apiVersion)
		versions := res(gh.Meta.ListAPIVersions(ctx)).v(t)
		check(t, len(versions) == 1 && versions[0] == apiVersion, "versions %v", versions)
		owner, _ := bootstrap(t, gh, "ver")
		_, resp, err := gh.Repositories.Get(ctx, owner, "ver")
		check(t, err == nil && resp.Header.Get("X-GitHub-Api-Version-Selected") == apiVersion, "echo header %q", resp.Header.Get("X-GitHub-Api-Version-Selected"))
		limits, _, err := gh.RateLimit.Get(ctx)
		check(t, err == nil && limits.GetCore().Limit == 5000, "rate limit core %v", err)
	})

	t.Run("workflow-dispatch", func(t *testing.T) {
		record(t, "workflow-dispatch")
		fresh(t)
		gh := client(t, author, apiVersion)
		owner, _ := bootstrap(t, gh, "wf")
		res(gh.Repositories.CreateFile(ctx, owner, "wf", ".github/workflows/deploy.yml", &github.RepositoryContentFileOptions{
			Message: github.Ptr("add workflow"), Content: []byte("name: Deploy\non:\n  workflow_dispatch:\n"),
		})).v(t)
		details := res(gh.Actions.CreateWorkflowDispatchEventByFileName(ctx, owner, "wf", "deploy.yml", github.CreateWorkflowDispatchEventRequest{Ref: "main"})).v(t)
		run := res(gh.Actions.GetWorkflowRunByID(ctx, owner, "wf", details.GetWorkflowRunID())).v(t)
		check(t, run.GetStatus() == "completed" && run.GetConclusion() == "success" && run.GetEvent() == "workflow_dispatch",
			"run %s/%s", run.GetStatus(), run.GetConclusion())
		runs := res(gh.Actions.ListWorkflowRunsByFileName(ctx, owner, "wf", "deploy.yml", nil)).v(t)
		check(t, runs.GetTotalCount() == 1, "runs %d", runs.GetTotalCount())
	})

	t.Run("webhook-delivery", func(t *testing.T) {
		record(t, "webhook-delivery")
		fresh(t)
		const secret = "smoke-secret"
		var mu sync.Mutex
		var events []any
		var bad []error
		recv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			payload, err := github.ValidatePayload(r, []byte(secret))
			if err == nil {
				var evt any
				evt, err = github.ParseWebHook(github.WebHookType(r), payload)
				mu.Lock()
				events = append(events, evt)
				mu.Unlock()
			}
			if err != nil {
				mu.Lock()
				bad = append(bad, err)
				mu.Unlock()
			}
		}))
		defer recv.Close()

		gh := client(t, author, apiVersion)
		owner, sha := bootstrap(t, gh, "hooks")
		hook := res(gh.Repositories.CreateHook(ctx, owner, "hooks", &github.Hook{
			Events: []string{"issues", "pull_request", "push", "check_run"},
			Config: &github.HookConfig{URL: github.Ptr(recv.URL), ContentType: github.Ptr("json"), Secret: github.Ptr(secret)},
		})).v(t)
		res(gh.Issues.Create(ctx, owner, "hooks", github.CreateIssueRequest{Title: "hook me"})).v(t)
		res(gh.Git.CreateRef(ctx, owner, "hooks", github.CreateRef{Ref: "refs/heads/topic", SHA: sha})).v(t)
		res(gh.Repositories.CreateFile(ctx, owner, "hooks", "x.txt", &github.RepositoryContentFileOptions{
			Message: github.Ptr("x"), Content: []byte("x\n"), Branch: github.Ptr("topic"),
		})).v(t)
		res(gh.PullRequests.Create(ctx, owner, "hooks", github.CreatePullRequest{Title: github.Ptr("t"), Head: "topic", Base: "main"})).v(t)
		inst := client(t, installationToken(t), apiVersion)
		res(inst.Checks.CreateCheckRun(ctx, owner, "hooks", github.CreateCheckRunOptions{Name: "c", HeadSHA: sha, Conclusion: github.Ptr("success")})).v(t)
		admin(t, "POST", "/admin/webhooks/flush", nil)

		mu.Lock()
		defer mu.Unlock()
		check(t, len(bad) == 0, "deliveries failed validation: %v", bad)
		seen := map[string]bool{}
		for _, e := range events {
			switch ev := e.(type) {
			case *github.PingEvent:
				seen["ping"] = ev.GetHookID() == hook.GetID()
			case *github.IssuesEvent:
				seen["issues."+ev.GetAction()] = true
			case *github.PushEvent:
				seen["push"] = true
			case *github.PullRequestEvent:
				seen["pull_request."+ev.GetAction()] = true
			case *github.CheckRunEvent:
				seen["check_run."+ev.GetAction()] = true
			}
		}
		for _, want := range []string{"ping", "issues.opened", "push", "pull_request.opened", "check_run.created", "check_run.completed"} {
			check(t, seen[want], "no signed %s delivery was parsed (got %v)", want, seen)
		}
		ds := res(gh.Repositories.ListHookDeliveries(ctx, owner, "hooks", hook.GetID(), &github.ListCursorOptions{PerPage: 50})).v(t)
		check(t, len(ds) == len(events), "delivery log has %d entries for %d deliveries", len(ds), len(events))
	})

	t.Run("error-bad-token", func(t *testing.T) {
		record(t, "error-bad-token")
		fresh(t)
		_, _, err := client(t, "not-a-github-token", apiVersion).Users.Get(ctx, "")
		var er *github.ErrorResponse
		check(t, errors.As(err, &er) && er.Response.StatusCode == 401 && er.Message == "Bad credentials", "got %v", err)
	})

	t.Run("error-missing-resource", func(t *testing.T) {
		record(t, "error-missing-resource")
		fresh(t)
		_, _, err := client(t, author, apiVersion).Repositories.Get(ctx, "nobody", "nothing")
		var er *github.ErrorResponse
		check(t, errors.As(err, &er) && er.Response.StatusCode == 404 && er.Message == "Not Found", "got %v", err)
	})

	t.Run("error-unsupported-version", func(t *testing.T) {
		record(t, "error-unsupported-version")
		fresh(t)
		// No override: go-github's own default, 2022-11-28.
		_, _, err := client(t, author, "").Repositories.Get(ctx, "smoke-author", "anything")
		var er *github.ErrorResponse
		check(t, errors.As(err, &er) && er.Response.StatusCode == 400, "go-github's default 2022-11-28 is refused with 400, got %v", err)
		// GitHub's version 400 carries errors as a string, not the documented
		// array, so go-github cannot decode the body; the status is what a
		// go-github caller sees.
		_, _, err = client(t, author, "2024-01-01").Repositories.Get(ctx, "smoke-author", "anything")
		check(t, errors.As(err, &er) && er.Response.StatusCode == 400, "an unknown version is refused with 400, got %v", err)
	})
}

// appID reads the seeded App's ID from the emulator's admin state.
func appID(t *testing.T) string {
	t.Helper()
	apps, _ := admin(t, "GET", "/admin/state", nil)["apps"].(map[string]any)
	for _, a := range apps {
		if app, ok := a.(map[string]any); ok {
			if id, ok := app["id"].(float64); ok {
				return strconv.FormatInt(int64(id), 10)
			}
		}
	}
	t.Fatal("no app in state")
	return ""
}

// appJWT signs an RS256 app JWT with a throwaway key. The emulator checks
// the claims, not the signature.
func appJWT(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	now := time.Now()
	signing := enc(map[string]string{"alg": "RS256", "typ": "JWT"}) + "." +
		enc(map[string]any{"iat": now.Add(-30 * time.Second).Unix(), "exp": now.Add(9 * time.Minute).Unix(), "iss": appID(t)})
	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func installationToken(t *testing.T) string {
	t.Helper()
	app := client(t, appJWT(t), apiVersion)
	installs := res(app.Apps.ListInstallations(ctx, nil)).v(t)
	return res(app.Apps.CreateInstallationToken(ctx, installs[0].GetID(), nil)).v(t).GetToken()
}
