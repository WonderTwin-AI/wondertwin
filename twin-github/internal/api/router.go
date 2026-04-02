// Package api implements the GitHub REST API-compatible HTTP handlers for the twin.
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/quirks"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// Handler holds GitHub API state.
type Handler struct {
	store   *store.MemoryStore
	mw      *twincore.Middleware
	emitter *telemetry.Emitter
	quirks  *quirks.Engine
}

// NewHandler creates a new GitHub API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware, em *telemetry.Emitter, qe *quirks.Engine) *Handler {
	return &Handler{store: s, mw: mw, emitter: em, quirks: qe}
}

// Routes mounts the GitHub REST API-compatible routes.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.bearerAuthMiddleware)
		r.Use(h.mw.FaultInjection)
		r.Use(quirks.Middleware(h.quirks))
		r.Use(telemetry.Middleware(h.emitter))

		// Meta
		r.Get("/rate_limit", h.RateLimit)
		r.Get("/emojis", h.ListEmojis)
		r.Get("/gitignore/templates", h.ListGitignoreTemplates)
		r.Get("/licenses", h.ListLicenses)
		r.Get("/user", h.GetAuthenticatedUser)
		r.Patch("/user", h.UpdateAuthenticatedUser)
		r.Get("/user/repos", h.ListUserRepos)
		r.Get("/user/emails", h.ListUserEmails)
		r.Get("/user/keys", h.ListUserKeys)
		r.Get("/user/orgs", h.ListUserOrgs)
		r.Get("/issues", h.ListAuthUserIssues)

		// Users
		r.Get("/users", h.ListAllUsers)
		r.Get("/users/{username}", h.GetUser)
		r.Get("/users/{username}/repos", h.ListUserReposByUsername)

		// GitHub App
		r.Get("/app", h.GetApp)
		r.Get("/app/installations", h.ListAppInstallations)
		r.Post("/app/installations/{installation_id}/access_tokens", h.CreateInstallationAccessToken)

		// Repos
		r.Get("/repos/{owner}/{repo}", h.GetRepo)
		r.Post("/user/repos", h.CreateRepo)
		r.Post("/orgs/{org}/repos", h.CreateOrgRepo)
		r.Patch("/repos/{owner}/{repo}", h.UpdateRepo)
		r.Delete("/repos/{owner}/{repo}", h.DeleteRepo)
		r.Get("/repos/{owner}/{repo}/contributors", h.ListContributors)
		r.Get("/repos/{owner}/{repo}/languages", h.ListLanguages)
		r.Get("/repos/{owner}/{repo}/tags", h.ListTags)
		r.Post("/repos/{owner}/{repo}/dispatches", h.RepoDispatch)

		// Branches
		r.Get("/repos/{owner}/{repo}/branches", h.ListBranches)
		r.Get("/repos/{owner}/{repo}/branches/{branch}", h.GetBranch)
		r.Get("/repos/{owner}/{repo}/branches/{branch}/protection", h.GetBranchProtection)
		r.Put("/repos/{owner}/{repo}/branches/{branch}/protection", h.UpdateBranchProtection)
		r.Delete("/repos/{owner}/{repo}/branches/{branch}/protection", h.DeleteBranchProtection)
		r.Post("/repos/{owner}/{repo}/branches/{branch}/rename", h.RenameBranch)

		// Commits
		r.Get("/repos/{owner}/{repo}/commits", h.ListCommits)
		r.Get("/repos/{owner}/{repo}/commits/{ref}", h.GetCommit)
		r.Get("/repos/{owner}/{repo}/compare/{basehead}", h.CompareCommits)

		// Issues
		r.Get("/repos/{owner}/{repo}/issues", h.ListIssues)
		r.Post("/repos/{owner}/{repo}/issues", h.CreateIssue)
		r.Get("/repos/{owner}/{repo}/issues/{issue_number}", h.GetIssue)
		r.Patch("/repos/{owner}/{repo}/issues/{issue_number}", h.UpdateIssue)

		// Issue comments
		r.Get("/repos/{owner}/{repo}/issues/{issue_number}/comments", h.ListIssueComments)
		r.Post("/repos/{owner}/{repo}/issues/{issue_number}/comments", h.CreateIssueComment)
		r.Get("/repos/{owner}/{repo}/issues/comments", h.ListRepoIssueComments)
		r.Get("/repos/{owner}/{repo}/issues/comments/{comment_id}", h.GetIssueComment)
		r.Patch("/repos/{owner}/{repo}/issues/comments/{comment_id}", h.UpdateIssueComment)
		r.Delete("/repos/{owner}/{repo}/issues/comments/{comment_id}", h.DeleteIssueComment)

		// Labels
		r.Get("/repos/{owner}/{repo}/labels", h.ListLabels)
		r.Post("/repos/{owner}/{repo}/labels", h.CreateLabel)
		r.Get("/repos/{owner}/{repo}/labels/{name}", h.GetLabel)
		r.Patch("/repos/{owner}/{repo}/labels/{name}", h.UpdateLabel)
		r.Delete("/repos/{owner}/{repo}/labels/{name}", h.DeleteLabel)
		r.Get("/repos/{owner}/{repo}/issues/{issue_number}/labels", h.ListIssueLabels)
		r.Post("/repos/{owner}/{repo}/issues/{issue_number}/labels", h.AddIssueLabels)
		r.Put("/repos/{owner}/{repo}/issues/{issue_number}/labels", h.SetIssueLabels)
		r.Delete("/repos/{owner}/{repo}/issues/{issue_number}/labels", h.RemoveAllIssueLabels)
		r.Delete("/repos/{owner}/{repo}/issues/{issue_number}/labels/{name}", h.RemoveIssueLabel)

		// Pull Requests
		r.Get("/repos/{owner}/{repo}/pulls", h.ListPullRequests)
		r.Post("/repos/{owner}/{repo}/pulls", h.CreatePullRequest)
		r.Get("/repos/{owner}/{repo}/pulls/{pull_number}", h.GetPullRequest)
		r.Patch("/repos/{owner}/{repo}/pulls/{pull_number}", h.UpdatePullRequest)
		r.Get("/repos/{owner}/{repo}/pulls/{pull_number}/commits", h.ListPRCommits)
		r.Get("/repos/{owner}/{repo}/pulls/{pull_number}/files", h.ListPRFiles)
		r.Get("/repos/{owner}/{repo}/pulls/{pull_number}/requested_reviewers", h.ListRequestedReviewers)
		r.Post("/repos/{owner}/{repo}/pulls/{pull_number}/requested_reviewers", h.RequestReviewers)
		r.Delete("/repos/{owner}/{repo}/pulls/{pull_number}/requested_reviewers", h.RemoveRequestedReviewers)
		r.Put("/repos/{owner}/{repo}/pulls/{pull_number}/merge", h.MergePullRequest)
		r.Get("/repos/{owner}/{repo}/pulls/{pull_number}/merge", h.CheckPRMerged)
		r.Put("/repos/{owner}/{repo}/pulls/{pull_number}/update-branch", h.UpdatePRBranch)

		// Commit statuses
		r.Post("/repos/{owner}/{repo}/statuses/{sha}", h.CreateCommitStatus)
		r.Get("/repos/{owner}/{repo}/commits/{ref}/statuses", h.ListCommitStatuses)
		r.Get("/repos/{owner}/{repo}/commits/{ref}/status", h.GetCombinedStatus)

		// Releases
		r.Get("/repos/{owner}/{repo}/releases", h.ListReleases)
		r.Post("/repos/{owner}/{repo}/releases", h.CreateRelease)
		r.Get("/repos/{owner}/{repo}/releases/{release_id}", h.GetRelease)
		r.Get("/repos/{owner}/{repo}/releases/latest", h.GetLatestRelease)
		r.Delete("/repos/{owner}/{repo}/releases/{release_id}", h.DeleteRelease)

		// Milestones
		r.Get("/repos/{owner}/{repo}/milestones", h.ListMilestones)
		r.Post("/repos/{owner}/{repo}/milestones", h.CreateMilestone)
		r.Get("/repos/{owner}/{repo}/milestones/{milestone_number}", h.GetMilestone)
		r.Patch("/repos/{owner}/{repo}/milestones/{milestone_number}", h.UpdateMilestone)
		r.Delete("/repos/{owner}/{repo}/milestones/{milestone_number}", h.DeleteMilestone)

		// PR Reviews
		r.Get("/repos/{owner}/{repo}/pulls/{pull_number}/reviews", h.ListPRReviews)
		r.Post("/repos/{owner}/{repo}/pulls/{pull_number}/reviews", h.CreatePRReview)
		r.Get("/repos/{owner}/{repo}/pulls/{pull_number}/reviews/{review_id}", h.GetPRReview)
		r.Patch("/repos/{owner}/{repo}/pulls/{pull_number}/reviews/{review_id}", h.UpdatePRReview)
		r.Post("/repos/{owner}/{repo}/pulls/{pull_number}/reviews/{review_id}/events", h.SubmitPRReview)
		r.Put("/repos/{owner}/{repo}/pulls/{pull_number}/reviews/{review_id}/dismissals", h.DismissPRReview)

		// PR Review Comments
		r.Get("/repos/{owner}/{repo}/pulls/{pull_number}/comments", h.ListPRReviewComments)
		r.Post("/repos/{owner}/{repo}/pulls/{pull_number}/comments", h.CreatePRReviewComment)
		r.Post("/repos/{owner}/{repo}/pulls/{pull_number}/comments/{comment_id}/replies", h.ReplyToPRReviewComment)
		r.Get("/repos/{owner}/{repo}/pulls/comments", h.ListAllPRReviewComments)
		r.Get("/repos/{owner}/{repo}/pulls/comments/{comment_id}", h.GetPRReviewComment)
		r.Patch("/repos/{owner}/{repo}/pulls/comments/{comment_id}", h.UpdatePRReviewComment)
		r.Delete("/repos/{owner}/{repo}/pulls/comments/{comment_id}", h.DeletePRReviewComment)

		// Check Runs
		r.Post("/repos/{owner}/{repo}/check-runs", h.CreateCheckRun)
		r.Get("/repos/{owner}/{repo}/check-runs/{check_run_id}", h.GetCheckRun)
		r.Patch("/repos/{owner}/{repo}/check-runs/{check_run_id}", h.UpdateCheckRun)
		r.Get("/repos/{owner}/{repo}/commits/{ref}/check-runs", h.ListCheckRunsForRef)
		r.Get("/repos/{owner}/{repo}/check-suites/{check_suite_id}/check-runs", h.ListCheckRunsInSuite)
		r.Get("/repos/{owner}/{repo}/check-runs/{check_run_id}/annotations", h.ListCheckRunAnnotations)
		r.Post("/repos/{owner}/{repo}/check-runs/{check_run_id}/rerequest", h.RerequestCheckRun)

		// Check Suites
		r.Post("/repos/{owner}/{repo}/check-suites", h.CreateCheckSuite)
		r.Get("/repos/{owner}/{repo}/check-suites/{check_suite_id}", h.GetCheckSuite)
		r.Get("/repos/{owner}/{repo}/commits/{ref}/check-suites", h.ListCheckSuitesForRef)
		r.Post("/repos/{owner}/{repo}/check-suites/{check_suite_id}/rerequest", h.RerequestCheckSuite)
		r.Patch("/repos/{owner}/{repo}/check-suites/preferences", h.UpdateCheckSuitePreferences)

		// Contents
		r.Get("/repos/{owner}/{repo}/contents/*", h.GetContents)
		r.Put("/repos/{owner}/{repo}/contents/*", h.CreateOrUpdateContents)
		r.Delete("/repos/{owner}/{repo}/contents/*", h.DeleteContents)
		r.Get("/repos/{owner}/{repo}/readme", h.GetReadme)
		r.Get("/repos/{owner}/{repo}/readme/{dir}", h.GetReadmeForDir)
		r.Get("/repos/{owner}/{repo}/tarball/{ref}", h.DownloadTarball)
		r.Get("/repos/{owner}/{repo}/zipball/{ref}", h.DownloadZipball)

		// Organizations
		r.Get("/orgs/{org}", h.GetOrg)
		r.Patch("/orgs/{org}", h.UpdateOrg)
		r.Get("/orgs/{org}/members", h.ListOrgMembers)
		r.Get("/orgs/{org}/members/{username}", h.CheckOrgMembership)
		r.Delete("/orgs/{org}/members/{username}", h.RemoveOrgMember)
		r.Get("/orgs/{org}/public_members", h.ListOrgPublicMembers)
		r.Get("/orgs/{org}/repos", h.ListOrgRepos)

		// Teams
		r.Get("/orgs/{org}/teams", h.ListOrgTeams)
		r.Post("/orgs/{org}/teams", h.CreateTeam)
		r.Get("/orgs/{org}/teams/{team_slug}", h.GetTeam)
		r.Patch("/orgs/{org}/teams/{team_slug}", h.UpdateTeam)
		r.Delete("/orgs/{org}/teams/{team_slug}", h.DeleteTeam)
		r.Get("/orgs/{org}/teams/{team_slug}/repos", h.ListTeamRepos)
		r.Get("/orgs/{org}/teams/{team_slug}/members", h.ListTeamMembers)
		r.Get("/orgs/{org}/teams/{team_slug}/teams", h.ListChildTeams)

		// Deploy Keys
		r.Get("/repos/{owner}/{repo}/keys", h.ListDeployKeys)
		r.Post("/repos/{owner}/{repo}/keys", h.CreateDeployKey)
		r.Get("/repos/{owner}/{repo}/keys/{key_id}", h.GetDeployKey)
		r.Delete("/repos/{owner}/{repo}/keys/{key_id}", h.DeleteDeployKey)

		// Deployments
		r.Get("/repos/{owner}/{repo}/deployments", h.ListDeployments)
		r.Post("/repos/{owner}/{repo}/deployments", h.CreateDeployment)
		r.Get("/repos/{owner}/{repo}/deployments/{deployment_id}", h.GetDeployment)
		r.Delete("/repos/{owner}/{repo}/deployments/{deployment_id}", h.DeleteDeployment)
		r.Get("/repos/{owner}/{repo}/deployments/{deployment_id}/statuses", h.ListDeploymentStatuses)
		r.Post("/repos/{owner}/{repo}/deployments/{deployment_id}/statuses", h.CreateDeploymentStatus)
		r.Get("/repos/{owner}/{repo}/deployments/{deployment_id}/statuses/{status_id}", h.GetDeploymentStatus)

		// Reactions
		r.Get("/repos/{owner}/{repo}/issues/{issue_number}/reactions", h.ListIssueReactions)
		r.Post("/repos/{owner}/{repo}/issues/{issue_number}/reactions", h.CreateIssueReaction)
		r.Delete("/repos/{owner}/{repo}/issues/{issue_number}/reactions/{reaction_id}", h.DeleteIssueReaction)
		r.Get("/repos/{owner}/{repo}/issues/comments/{comment_id}/reactions", h.ListCommentReactions)
		r.Post("/repos/{owner}/{repo}/issues/comments/{comment_id}/reactions", h.CreateCommentReaction)
		r.Delete("/repos/{owner}/{repo}/issues/comments/{comment_id}/reactions/{reaction_id}", h.DeleteCommentReaction)
		r.Get("/repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions", h.ListPRCommentReactions)
		r.Post("/repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions", h.CreatePRCommentReaction)
		r.Delete("/repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions/{reaction_id}", h.DeletePRCommentReaction)

		// Collaborators
		r.Get("/repos/{owner}/{repo}/collaborators", h.ListCollaborators)
		r.Get("/repos/{owner}/{repo}/collaborators/{username}", h.CheckCollaborator)
		r.Put("/repos/{owner}/{repo}/collaborators/{username}", h.AddCollaborator)
		r.Delete("/repos/{owner}/{repo}/collaborators/{username}", h.RemoveCollaborator)
		r.Get("/repos/{owner}/{repo}/collaborators/{username}/permission", h.GetCollaboratorPermission)

		// Topics
		r.Get("/repos/{owner}/{repo}/topics", h.ListRepoTopics)
		r.Put("/repos/{owner}/{repo}/topics", h.ReplaceRepoTopics)

		// Forks
		r.Get("/repos/{owner}/{repo}/forks", h.ListForks)
		r.Post("/repos/{owner}/{repo}/forks", h.CreateFork)

		// Releases extra
		r.Patch("/repos/{owner}/{repo}/releases/{release_id}", h.UpdateRelease)
		r.Get("/repos/{owner}/{repo}/releases/tags/{tag}", h.GetReleaseByTag)

		// Release Assets
		r.Get("/repos/{owner}/{repo}/releases/{release_id}/assets", h.ListReleaseAssets)
		r.Post("/repos/{owner}/{repo}/releases/{release_id}/assets", h.UploadReleaseAsset)
		r.Get("/repos/{owner}/{repo}/releases/assets/{asset_id}", h.GetReleaseAsset)
		r.Patch("/repos/{owner}/{repo}/releases/assets/{asset_id}", h.UpdateReleaseAsset)
		r.Delete("/repos/{owner}/{repo}/releases/assets/{asset_id}", h.DeleteReleaseAsset)

		// Actions — Workflows
		r.Get("/repos/{owner}/{repo}/actions/workflows", h.ListWorkflows)
		r.Get("/repos/{owner}/{repo}/actions/workflows/{workflow_id}", h.GetWorkflow)
		r.Post("/repos/{owner}/{repo}/actions/workflows/{workflow_id}/dispatches", h.TriggerWorkflow)

		// Actions — Runs
		r.Get("/repos/{owner}/{repo}/actions/runs", h.ListWorkflowRuns)
		r.Get("/repos/{owner}/{repo}/actions/workflows/{workflow_id}/runs", h.ListWorkflowRunsForWorkflow)
		r.Get("/repos/{owner}/{repo}/actions/runs/{run_id}", h.GetWorkflowRun)
		r.Delete("/repos/{owner}/{repo}/actions/runs/{run_id}", h.DeleteWorkflowRun)
		r.Post("/repos/{owner}/{repo}/actions/runs/{run_id}/cancel", h.CancelWorkflowRun)
		r.Post("/repos/{owner}/{repo}/actions/runs/{run_id}/rerun", h.RerunWorkflow)
		r.Post("/repos/{owner}/{repo}/actions/runs/{run_id}/rerun-failed-jobs", h.RerunFailedJobs)
		r.Get("/repos/{owner}/{repo}/actions/runs/{run_id}/logs", h.GetWorkflowRunLogs)
		r.Get("/repos/{owner}/{repo}/actions/runs/{run_id}/timing", h.GetWorkflowRunTiming)
		r.Post("/repos/{owner}/{repo}/actions/runs/{run_id}/approve", h.ApproveWorkflowRun)
		r.Get("/repos/{owner}/{repo}/actions/runs/{run_id}/approvals", h.ListWorkflowRunApprovals)

		// Actions — Jobs
		r.Get("/repos/{owner}/{repo}/actions/runs/{run_id}/jobs", h.ListRunJobs)
		r.Get("/repos/{owner}/{repo}/actions/jobs/{job_id}", h.GetJob)
		r.Get("/repos/{owner}/{repo}/actions/jobs/{job_id}/logs", h.GetJobLogs)
		r.Post("/repos/{owner}/{repo}/actions/jobs/{job_id}/rerun", h.RerunJob)

		// Actions — Artifacts
		r.Get("/repos/{owner}/{repo}/actions/runs/{run_id}/artifacts", h.ListRunArtifacts)
		r.Get("/repos/{owner}/{repo}/actions/artifacts", h.ListRepoArtifacts)
		r.Get("/repos/{owner}/{repo}/actions/artifacts/{artifact_id}", h.GetArtifact)
		r.Delete("/repos/{owner}/{repo}/actions/artifacts/{artifact_id}", h.DeleteArtifact)
		r.Get("/repos/{owner}/{repo}/actions/artifacts/{artifact_id}/{archive_format}", h.DownloadArtifact)

		// Actions — Secrets
		r.Get("/repos/{owner}/{repo}/actions/secrets", h.ListRepoSecrets)
		r.Get("/repos/{owner}/{repo}/actions/secrets/public-key", h.GetRepoPublicKey)
		r.Get("/repos/{owner}/{repo}/actions/secrets/{secret_name}", h.GetRepoSecret)
		r.Put("/repos/{owner}/{repo}/actions/secrets/{secret_name}", h.CreateOrUpdateRepoSecret)
		r.Delete("/repos/{owner}/{repo}/actions/secrets/{secret_name}", h.DeleteRepoSecret)

		// Org-level Actions Secrets
		r.Get("/orgs/{org}/actions/secrets", h.ListOrgSecrets)
		r.Get("/orgs/{org}/actions/secrets/public-key", h.GetOrgPublicKey)
		r.Get("/orgs/{org}/actions/secrets/{secret_name}", h.GetOrgSecret)
		r.Put("/orgs/{org}/actions/secrets/{secret_name}", h.CreateOrUpdateOrgSecret)
		r.Delete("/orgs/{org}/actions/secrets/{secret_name}", h.DeleteOrgSecret)

		// Git Data — Refs
		r.Get("/repos/{owner}/{repo}/git/ref/*", h.GetGitRef)
		r.Get("/repos/{owner}/{repo}/git/matching-refs/*", h.ListMatchingRefs)
		r.Post("/repos/{owner}/{repo}/git/refs", h.CreateGitRef)
		r.Patch("/repos/{owner}/{repo}/git/refs/*", h.UpdateGitRef)
		r.Delete("/repos/{owner}/{repo}/git/refs/*", h.DeleteGitRef)

		// Git Data — Commits, Trees, Blobs, Tags
		r.Get("/repos/{owner}/{repo}/git/commits/{commit_sha}", h.GetGitCommit)
		r.Post("/repos/{owner}/{repo}/git/commits", h.CreateGitCommit)
		r.Get("/repos/{owner}/{repo}/git/trees/{tree_sha}", h.GetGitTree)
		r.Post("/repos/{owner}/{repo}/git/trees", h.CreateGitTree)
		r.Get("/repos/{owner}/{repo}/git/blobs/{file_sha}", h.GetGitBlob)
		r.Post("/repos/{owner}/{repo}/git/blobs", h.CreateGitBlob)
		r.Get("/repos/{owner}/{repo}/git/tags/{tag_sha}", h.GetGitTag)
		r.Post("/repos/{owner}/{repo}/git/tags", h.CreateGitTag)

		// Search
		r.Get("/search/issues", h.SearchIssues)
		r.Get("/search/repositories", h.SearchRepos)
		r.Get("/search/users", h.SearchUsers)
		r.Get("/search/code", h.SearchCode)
		r.Get("/search/commits", h.SearchCommits)
		r.Get("/search/topics", h.SearchTopics)
		r.Get("/search/labels", h.SearchLabels)

		// Webhooks
		r.Get("/repos/{owner}/{repo}/hooks", h.ListWebhooks)
		r.Post("/repos/{owner}/{repo}/hooks", h.CreateWebhook)
		r.Get("/repos/{owner}/{repo}/hooks/{hook_id}", h.GetWebhook)
		r.Patch("/repos/{owner}/{repo}/hooks/{hook_id}", h.UpdateWebhook)
		r.Delete("/repos/{owner}/{repo}/hooks/{hook_id}", h.DeleteWebhook)
		r.Post("/repos/{owner}/{repo}/hooks/{hook_id}/pings", h.PingWebhook)
		r.Post("/repos/{owner}/{repo}/hooks/{hook_id}/tests", h.TestWebhook)
		r.Get("/repos/{owner}/{repo}/hooks/{hook_id}/deliveries", h.ListWebhookDeliveries)
	})

	// Admin extras (no auth required)
	r.Get("/admin/repos", h.AdminListRepos)
	r.Get("/admin/issues", h.AdminListIssues)
	r.Get("/admin/pulls", h.AdminListPRs)
}

// bearerAuthMiddleware validates GitHub-style Bearer token auth.
func (h *Handler) bearerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			ghError(w, http.StatusUnauthorized, "Requires authentication")
			return
		}

		// Accept "Bearer <token>" or legacy "token <token>"
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth {
			token = strings.TrimPrefix(auth, "token ")
		}
		if token == auth || token == "" {
			ghError(w, http.StatusUnauthorized, "Bad credentials")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ghJSON writes a successful JSON response with GitHub-standard headers.
func ghJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-RateLimit-Limit", "5000")
	w.Header().Set("X-RateLimit-Remaining", "4999")
	w.Header().Set("X-RateLimit-Used", "1")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ghError writes a GitHub-style error response.
func ghError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"message":           message,
		"documentation_url": "https://docs.github.com/rest",
	})
}

// ghValidationError writes a 422 validation error.
func ghValidationError(w http.ResponseWriter, resource, field, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	json.NewEncoder(w).Encode(map[string]any{
		"message":           "Validation Failed",
		"documentation_url": "https://docs.github.com/rest",
		"errors": []map[string]any{
			{"resource": resource, "field": field, "code": code},
		},
	})
}
