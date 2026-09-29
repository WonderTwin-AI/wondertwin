package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// RateLimit handles GET /rate_limit. The 2026-03-10 shape has no top-level
// "rate" object (changeset remove_rate_limit_rate); resources.core replaces it.
func (h *Handler) RateLimit(w http.ResponseWriter, r *http.Request) {
	reset := h.store.Clock.Now().Add(time.Hour).Unix()
	core := 5000
	if principalFrom(r).Kind == principalAnonymous {
		core = 60
	}
	bucket := func(limit, used int) map[string]any {
		return map[string]any{"limit": limit, "remaining": limit - used, "reset": reset, "used": used}
	}
	ghJSON(w, 200, map[string]any{
		"resources": map[string]any{
			"core":                        bucket(core, 1),
			"search":                      bucket(30, 0),
			"graphql":                     bucket(5000, 0),
			"code_search":                 bucket(10, 0),
			"integration_manifest":        bucket(5000, 0),
			"source_import":               bucket(100, 0),
			"actions_runner_registration": bucket(10000, 0),
			"scim":                        bucket(15000, 0),
			"dependency_snapshots":        bucket(100, 0),
			"dependency_sbom":             bucket(100, 0),
			"code_scanning_autofix":       bucket(10, 0),
			"code_scanning_upload":        bucket(1000, 0),
			"audit_log":                   bucket(1750, 0),
			"audit_log_streaming":         bucket(15, 0),
		},
	})
}

// ListVersions handles GET /versions: the calendar versions served, newest
// first. Community serves one.
func (h *Handler) ListVersions(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []string{APIVersion})
}

var zen = []string{
	"Keep it logically awesome.",
	"Design for failure.",
	"Speak like a human.",
	"Approachable is better than simple.",
	"Mind your words, they are important.",
	"Non-blocking is better than blocking.",
	"Favor focus over features.",
	"Avoid administrative distraction.",
	"Anything added dilutes everything else.",
	"Half measures are as bad as nothing at all.",
	"Responsive is better than fast.",
	"It's not fully shipped until it's fast.",
	"Practicality beats purity.",
	"Encourage flow.",
}

// GetZen handles GET /zen, which answers in plain text.
func (h *Handler) GetZen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain;charset=utf-8")
	w.WriteHeader(200)
	_, _ = w.Write([]byte(zen[h.store.Clock.Now().Unix()%int64(len(zen))]))
}

// GetRoot handles GET /, the hypermedia index. The 2026-03-10 shape drops
// authorizations_url and hub_url.
func (h *Handler) GetRoot(w http.ResponseWriter, r *http.Request) {
	o := origin(r)
	ghJSON(w, 200, map[string]any{
		"current_user_url":                     o + "/user",
		"current_user_authorizations_html_url": "https://github.com/settings/connections/applications{/client_id}",
		"code_search_url":                      o + "/search/code?q={query}{&page,per_page,sort,order}",
		"commit_search_url":                    o + "/search/commits?q={query}{&page,per_page,sort,order}",
		"emails_url":                           o + "/user/emails",
		"emojis_url":                           o + "/emojis",
		"events_url":                           o + "/events",
		"feeds_url":                            o + "/feeds",
		"followers_url":                        o + "/user/followers",
		"following_url":                        o + "/user/following{/target}",
		"gists_url":                            o + "/gists{/gist_id}",
		"issue_search_url":                     o + "/search/issues?q={query}{&page,per_page,sort,order}",
		"issues_url":                           o + "/issues",
		"keys_url":                             o + "/user/keys",
		"label_search_url":                     o + "/search/labels?q={query}&repository_id={repository_id}{&page,per_page}",
		"notifications_url":                    o + "/notifications",
		"organization_url":                     o + "/orgs/{org}",
		"organization_repositories_url":        o + "/orgs/{org}/repos{?type,page,per_page,sort}",
		"organization_teams_url":               o + "/orgs/{org}/teams",
		"public_gists_url":                     o + "/gists/public",
		"rate_limit_url":                       o + "/rate_limit",
		"repository_url":                       o + "/repos/{owner}/{repo}",
		"repository_search_url":                o + "/search/repositories?q={query}{&page,per_page,sort,order}",
		"current_user_repositories_url":        o + "/user/repos{?type,page,per_page,sort}",
		"starred_url":                          o + "/user/starred{/owner}{/repo}",
		"starred_gists_url":                    o + "/gists/starred",
		"topic_search_url":                     o + "/search/topics?q={query}{&page,per_page}",
		"user_url":                             o + "/users/{user}",
		"user_organizations_url":               o + "/user/orgs",
		"user_repositories_url":                o + "/users/{user}/repos{?type,page,per_page,sort}",
		"user_search_url":                      o + "/search/users?q={query}{&page,per_page}",
	})
}

// GetAuthenticatedUser handles GET /user
func (h *Handler) GetAuthenticatedUser(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r)
	if p.Kind != principalUser {
		ghError(w, http.StatusForbidden, "Resource not accessible by integration")
		return
	}
	u, ok := h.store.Users.Get(p.Login)
	if !ok {
		u = store.User{ID: h.store.NewID(store.KindUser), Login: p.Login, Type: "User"}
		h.store.Users.Set(p.Login, u)
	}
	ghJSON(w, 200, u)
}

// UpdateAuthenticatedUser handles PATCH /user
func (h *Handler) UpdateAuthenticatedUser(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, store.User{
		ID:    1,
		Login: "twin-bot",
		Type:  "User",
	})
}

// GetUser handles GET /users/{username}
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	u, ok := h.store.Users.Get(username)
	if !ok {
		// Return a generated user
		u = store.User{
			ID:      h.store.NewID(store.KindUser),
			Login:   username,
			Type:    "User",
			HTMLURL: h.store.BaseURL() + "/" + username,
		}
	}
	ghJSON(w, 200, u)
}

// GetRepo handles GET /repos/{owner}/{repo}
func (h *Handler) GetRepo(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	rp, ok := h.store.GetRepo(owner, repo)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, rp)
}

// CreateRepo handles POST /user/repos
func (h *Handler) CreateRepo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Private     bool   `json:"private"`
		AutoInit    bool   `json:"auto_init"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if req.Name == "" {
		ghValidationError(w, "Repository", "name", "missing_field")
		return
	}

	owner := actor(r)
	if _, exists := h.store.GetRepo(owner, req.Name); exists {
		repoExists(w, "https://docs.github.com/rest/repos/repos#create-a-repository-for-the-authenticated-user")
		return
	}
	now := h.store.Now()
	rp := store.Repository{
		ID:            h.store.NewID(store.KindRepo),
		Name:          req.Name,
		FullName:      owner + "/" + req.Name,
		Description:   req.Description,
		Private:       req.Private,
		DefaultBranch: "main",
		HasIssues:     true,
		HasProjects:   true,
		HasWiki:       true,
		Owner:         h.userRef(owner),
		HTMLURL:       fmt.Sprintf("%s/%s/%s", h.store.BaseURL(), owner, req.Name),
		CreatedAt:     now,
		UpdatedAt:     now,
		PushedAt:      now,
	}

	h.store.Repos.Set(store.RepoKey(owner, req.Name), rp)

	// auto_init makes an initial commit holding a README, as GitHub does.
	if req.AutoInit {
		readme := "# " + req.Name + "\n"
		if req.Description != "" {
			readme += req.Description + "\n"
		}
		sig := h.signature(r, contentsRequest{})
		_, _ = h.store.CommitChange(owner, req.Name, "main", "README.md", []byte(readme), "Initial commit", sig)
	}

	ghJSON(w, 201, rp)
}

// UpdateRepo handles PATCH /repos/{owner}/{repo}
func (h *Handler) UpdateRepo(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	rp, ok := h.store.GetRepo(owner, repo)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	var req map[string]any
	json.NewDecoder(r.Body).Decode(&req)

	if desc, ok := req["description"].(string); ok {
		rp.Description = desc
	}
	if priv, ok := req["private"].(bool); ok {
		rp.Private = priv
	}
	if archived, ok := req["archived"].(bool); ok {
		rp.Archived = archived
	}
	rp.UpdatedAt = h.store.Now()

	h.store.Repos.Set(store.RepoKey(owner, repo), *rp)
	ghJSON(w, 200, rp)
}

// DeleteRepo handles DELETE /repos/{owner}/{repo}
func (h *Handler) DeleteRepo(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	key := store.RepoKey(owner, repo)
	if _, ok := h.store.Repos.Get(key); !ok {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.Repos.Delete(key)
	w.WriteHeader(204)
}

// userRef returns the stored user for login, creating it on first sight so
// that every login the emulator mentions has one stable ID.
func (h *Handler) userRef(login string) store.User {
	if u, ok := h.store.Users.Get(login); ok {
		return u
	}
	u := store.User{ID: h.store.NewID(store.KindUser), Login: login, Type: "User"}
	h.store.Users.Set(login, u)
	return u
}
