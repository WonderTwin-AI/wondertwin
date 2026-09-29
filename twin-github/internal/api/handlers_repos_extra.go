package api

import (
	"encoding/json"
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// ListUserRepos handles GET /user/repos
func (h *Handler) ListUserRepos(w http.ResponseWriter, r *http.Request) {
	me := actor(r)
	repos := h.store.Repos.Filter(func(_ string, rp store.Repository) bool {
		return rp.Owner.Login == me
	})
	h.writeRepos(w, r, repos)
}

// ListUserReposByUsername handles GET /users/{username}/repos
func (h *Handler) ListUserReposByUsername(w http.ResponseWriter, r *http.Request) {
	username := param(r, "username")
	repos := h.store.Repos.Filter(func(_ string, rp store.Repository) bool {
		return rp.Owner.Login == username && (!rp.Private || viewer(r) == username) && !hiddenFrom(r, rp)
	})
	h.writeRepos(w, r, repos)
}

// CreateOrgRepo handles POST /orgs/{org}/repos
func (h *Handler) CreateOrgRepo(w http.ResponseWriter, r *http.Request) {
	org := param(r, "org")

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Private     bool   `json:"private"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}

	if req.Name == "" {
		ghValidationError(w, "Repository", "name", "missing_field")
		return
	}
	if _, exists := h.store.GetRepo(org, req.Name); exists {
		repoExists(w, "https://docs.github.com/rest/repos/repos#create-an-organization-repository")
		return
	}

	if _, known := h.store.Orgs.Get(org); !known {
		h.store.Orgs.Set(org, store.Organization{ID: h.store.NewID(store.KindOrg), Login: org, Type: "Organization"})
	}

	now := h.store.Now()
	rp := store.Repository{
		ID:            h.store.NewID(store.KindRepo),
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
	ghJSON(w, 201, h.rd(r).repoFull(rp, actor(r)))
}

// ListForks handles GET /repos/{owner}/{repo}/forks
func (h *Handler) ListForks(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	forks := h.store.Repos.Filter(func(_ string, rp store.Repository) bool {
		return rp.Fork && rp.Name == repo && rp.Owner.Login != owner && !hiddenFrom(r, rp)
	})
	ghJSON(w, 200, paginate(w, r, forks))
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

// ListCommits handles GET /repos/{owner}/{repo}/commits: the history of sha
// (a branch, tag or SHA; the default branch when absent), newest first.
func (h *Handler) ListCommits(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	if _, ok := h.store.GetRepo(owner, repo); !ok {
		ghError(w, 404, "Not Found")
		return
	}
	if !h.store.HasCommits(owner, repo) {
		ghError(w, 409, "Git Repository is empty.")
		return
	}
	head, ok := h.store.ResolveRef(owner, repo, r.URL.Query().Get("sha"))
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	x := h.rd(r)
	out := []map[string]any{}
	for _, c := range paginate(w, r, h.store.History(owner, repo, head.SHA, "")) {
		out = append(out, x.commit(c))
	}
	ghJSON(w, 200, out)
}

// GetCommit handles GET /repos/{owner}/{repo}/commits/{ref} (high-level)
func (h *Handler) GetCommit(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	ref := param(r, "ref")
	c, ok := h.store.ResolveRef(owner, repo, ref)
	if !ok {
		ghValidationErrors(w, "No commit found for SHA: "+ref)
		return
	}
	ghJSON(w, 200, h.rd(r).commit(c))
}

// CompareCommits handles GET /repos/{owner}/{repo}/compare/{basehead}
func (h *Handler) CompareCommits(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{
		"status":        "ahead",
		"ahead_by":      1,
		"behind_by":     0,
		"total_commits": 1,
		"commits":       []any{},
		"files":         []any{},
	})
}

// ListAllUsers handles GET /users
func (h *Handler) ListAllUsers(w http.ResponseWriter, r *http.Request) {
	users := h.store.Users.List()
	ghJSON(w, 200, paginate(w, r, users))
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
	ghJSON(w, 200, paginate(w, r, orgs))
}

// ListAuthUserIssues handles GET /issues
func (h *Handler) ListAuthUserIssues(w http.ResponseWriter, r *http.Request) {
	me := actor(r)
	x := h.rd(r)
	out := []map[string]any{}
	for _, i := range paginate(w, r, h.store.Issues.Filter(func(_ string, i store.Issue) bool {
		if i.State != "open" {
			return false
		}
		for _, a := range i.Assignees {
			if a.Login == me {
				return true
			}
		}
		return false
	})) {
		m := x.issue(i)
		if rp, ok := h.store.GetRepo(i.RepoOwner, i.RepoName); ok {
			m["repository"] = x.repo(*rp, me)
		}
		out = append(out, m)
	}
	ghJSON(w, 200, out)
}

// ListRepoIssueComments handles GET /repos/{owner}/{repo}/issues/comments (all comments in repo)
func (h *Handler) ListRepoIssueComments(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	comments := h.store.Comments.Filter(func(_ string, c store.Comment) bool {
		return c.RepoOwner == owner && c.RepoName == repo
	})
	x := h.rd(r)
	out := []map[string]any{}
	for _, c := range paginate(w, r, comments) {
		out = append(out, x.comment(c))
	}
	ghJSON(w, 200, out)
}

// writeRepos renders a page of repositories.
func (h *Handler) writeRepos(w http.ResponseWriter, r *http.Request, repos []store.Repository) {
	x := h.rd(r)
	out := []map[string]any{}
	for _, rp := range paginate(w, r, repos) {
		out = append(out, x.repo(rp, viewer(r)))
	}
	ghJSON(w, 200, out)
}

// hiddenFrom reports whether rp is private and the caller is anonymous.
func hiddenFrom(r *http.Request, rp store.Repository) bool {
	return rp.Private && principalFrom(r).Kind == principalAnonymous
}
