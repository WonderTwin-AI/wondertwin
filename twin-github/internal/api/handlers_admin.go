package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// AdminListRepos handles GET /admin/repos
func (h *Handler) AdminListRepos(w http.ResponseWriter, r *http.Request) {
	repos := h.store.Repos.List()
	twincore.JSON(w, 200, map[string]any{"repos": repos, "total": len(repos)})
}

// AdminListIssues handles GET /admin/issues
func (h *Handler) AdminListIssues(w http.ResponseWriter, r *http.Request) {
	issues := h.store.Issues.List()
	twincore.JSON(w, 200, map[string]any{"issues": issues, "total": len(issues)})
}

// AdminListPRs handles GET /admin/pulls
func (h *Handler) AdminListPRs(w http.ResponseWriter, r *http.Request) {
	prs := h.store.PullRequests.List()
	twincore.JSON(w, 200, map[string]any{"pull_requests": prs, "total": len(prs)})
}
