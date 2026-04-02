package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// ListUserRepos handles GET /user/repos
func (h *Handler) ListUserRepos(w http.ResponseWriter, r *http.Request) {
	repos := h.store.Repos.Filter(func(_ string, rp store.Repository) bool {
		return rp.Owner.Login == "twin-bot"
	})
	ghJSON(w, 200, repos)
}

// ListUserReposByUsername handles GET /users/{username}/repos
func (h *Handler) ListUserReposByUsername(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	repos := h.store.Repos.Filter(func(_ string, rp store.Repository) bool {
		return rp.Owner.Login == username
	})
	ghJSON(w, 200, repos)
}

// ListOrgReposFromRepos handles POST /orgs/{org}/repos
func (h *Handler) CreateOrgRepo(w http.ResponseWriter, r *http.Request) {
	org := chi.URLParam(r, "org")

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Private     bool   `json:"private"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}

	now := h.store.Now()
	rp := store.Repository{
		ID:            h.store.NextID(),
		Name:          req.Name,
		FullName:      org + "/" + req.Name,
		Description:   req.Description,
		Private:       req.Private,
		DefaultBranch: "main",
		HasIssues:     true,
		Owner:         store.User{Login: org, Type: "Organization"},
		HTMLURL:       h.store.BaseURL() + "/" + org + "/" + req.Name,
		CreatedAt:     now,
		UpdatedAt:     now,
		PushedAt:      now,
	}
	h.store.Repos.Set(store.RepoKey(org, req.Name), rp)
	ghJSON(w, 201, rp)
}

// ListForks handles GET /repos/{owner}/{repo}/forks
func (h *Handler) ListForks(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	forks := h.store.Repos.Filter(func(_ string, rp store.Repository) bool {
		return rp.Fork && rp.Name == repo && rp.Owner.Login != owner
	})
	ghJSON(w, 200, forks)
}

// ListContributors handles GET /repos/{owner}/{repo}/contributors
func (h *Handler) ListContributors(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []store.User{
		{ID: 1, Login: "twin-bot", Type: "User"},
	})
}

// ListLanguages handles GET /repos/{owner}/{repo}/languages
func (h *Handler) ListLanguages(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]int{"Go": 10000})
}

// ListTags handles GET /repos/{owner}/{repo}/tags
func (h *Handler) ListTags(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []any{})
}

// ListCommits handles GET /repos/{owner}/{repo}/commits
func (h *Handler) ListCommits(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []any{})
}

// GetCommit handles GET /repos/{owner}/{repo}/commits/{ref} (high-level)
func (h *Handler) GetCommit(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	ghJSON(w, 200, map[string]any{
		"sha": ref,
		"commit": map[string]any{
			"message": "simulated commit",
			"author":  store.GitSignature{Name: "twin-bot", Email: "bot@wondertwin.dev", Date: h.store.Now()},
		},
		"author":    store.User{ID: 1, Login: "twin-bot", Type: "User"},
		"committer": store.User{ID: 1, Login: "twin-bot", Type: "User"},
	})
}

// CompareCommits handles GET /repos/{owner}/{repo}/compare/{basehead}
func (h *Handler) CompareCommits(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{
		"status":       "ahead",
		"ahead_by":     1,
		"behind_by":    0,
		"total_commits": 1,
		"commits":      []any{},
		"files":        []any{},
	})
}

// ListAllUsers handles GET /users
func (h *Handler) ListAllUsers(w http.ResponseWriter, r *http.Request) {
	users := h.store.Users.List()
	ghJSON(w, 200, users)
}

// ListUserEmails handles GET /user/emails
func (h *Handler) ListUserEmails(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []map[string]any{
		{"email": "bot@wondertwin.dev", "verified": true, "primary": true, "visibility": "public"},
	})
}

// ListUserKeys handles GET /user/keys
func (h *Handler) ListUserKeys(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []any{})
}

// ListUserOrgs handles GET /user/orgs
func (h *Handler) ListUserOrgs(w http.ResponseWriter, r *http.Request) {
	orgs := h.store.Orgs.List()
	ghJSON(w, 200, orgs)
}

// ListAuthUserIssues handles GET /issues
func (h *Handler) ListAuthUserIssues(w http.ResponseWriter, r *http.Request) {
	issues := h.store.Issues.List()
	ghJSON(w, 200, issues)
}

// ListRepoIssueComments handles GET /repos/{owner}/{repo}/issues/comments (all comments in repo)
func (h *Handler) ListRepoIssueComments(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	comments := h.store.Comments.Filter(func(_ string, c store.Comment) bool {
		return c.RepoOwner == owner && c.RepoName == repo
	})
	ghJSON(w, 200, comments)
}
