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
		r.Get("/user", h.GetAuthenticatedUser)
		r.Patch("/user", h.UpdateAuthenticatedUser)

		// Users
		r.Get("/users/{username}", h.GetUser)

		// Repos
		r.Get("/repos/{owner}/{repo}", h.GetRepo)
		r.Post("/user/repos", h.CreateRepo)
		r.Patch("/repos/{owner}/{repo}", h.UpdateRepo)
		r.Delete("/repos/{owner}/{repo}", h.DeleteRepo)

		// Branches
		r.Get("/repos/{owner}/{repo}/branches", h.ListBranches)
		r.Get("/repos/{owner}/{repo}/branches/{branch}", h.GetBranch)

		// Issues
		r.Get("/repos/{owner}/{repo}/issues", h.ListIssues)
		r.Post("/repos/{owner}/{repo}/issues", h.CreateIssue)
		r.Get("/repos/{owner}/{repo}/issues/{issue_number}", h.GetIssue)
		r.Patch("/repos/{owner}/{repo}/issues/{issue_number}", h.UpdateIssue)

		// Issue comments
		r.Get("/repos/{owner}/{repo}/issues/{issue_number}/comments", h.ListIssueComments)
		r.Post("/repos/{owner}/{repo}/issues/{issue_number}/comments", h.CreateIssueComment)
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
		r.Delete("/repos/{owner}/{repo}/issues/{issue_number}/labels/{name}", h.RemoveIssueLabel)

		// Pull Requests
		r.Get("/repos/{owner}/{repo}/pulls", h.ListPullRequests)
		r.Post("/repos/{owner}/{repo}/pulls", h.CreatePullRequest)
		r.Get("/repos/{owner}/{repo}/pulls/{pull_number}", h.GetPullRequest)
		r.Patch("/repos/{owner}/{repo}/pulls/{pull_number}", h.UpdatePullRequest)
		r.Put("/repos/{owner}/{repo}/pulls/{pull_number}/merge", h.MergePullRequest)
		r.Get("/repos/{owner}/{repo}/pulls/{pull_number}/merge", h.CheckPRMerged)

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

		// Webhooks
		r.Get("/repos/{owner}/{repo}/hooks", h.ListWebhooks)
		r.Post("/repos/{owner}/{repo}/hooks", h.CreateWebhook)
		r.Get("/repos/{owner}/{repo}/hooks/{hook_id}", h.GetWebhook)
		r.Patch("/repos/{owner}/{repo}/hooks/{hook_id}", h.UpdateWebhook)
		r.Delete("/repos/{owner}/{repo}/hooks/{hook_id}", h.DeleteWebhook)
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
