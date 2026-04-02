package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// RateLimit handles GET /rate_limit
func (h *Handler) RateLimit(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{
		"resources": map[string]any{
			"core":    map[string]any{"limit": 5000, "remaining": 4999, "reset": 0, "used": 1},
			"search":  map[string]any{"limit": 30, "remaining": 30, "reset": 0, "used": 0},
			"graphql": map[string]any{"limit": 5000, "remaining": 5000, "reset": 0, "used": 0},
		},
		"rate": map[string]any{"limit": 5000, "remaining": 4999, "reset": 0, "used": 1},
	})
}

// GetAuthenticatedUser handles GET /user
func (h *Handler) GetAuthenticatedUser(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, store.User{
		ID:    1,
		Login: "twin-bot",
		Type:  "User",
		Name:  "Twin Bot",
		Email: "bot@wondertwin.dev",
	})
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
			ID:      h.store.NextID(),
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

	owner := "twin-bot"
	now := h.store.Now()
	rp := store.Repository{
		ID:            h.store.NextID(),
		Name:          req.Name,
		FullName:      owner + "/" + req.Name,
		Description:   req.Description,
		Private:       req.Private,
		DefaultBranch: "main",
		HasIssues:     true,
		HasProjects:   true,
		HasWiki:       true,
		Owner:         store.User{ID: 1, Login: owner, Type: "User"},
		HTMLURL:       fmt.Sprintf("%s/%s/%s", h.store.BaseURL(), owner, req.Name),
		CreatedAt:     now,
		UpdatedAt:     now,
		PushedAt:      now,
	}

	h.store.Repos.Set(store.RepoKey(owner, req.Name), rp)

	// Create default branch if auto_init
	if req.AutoInit {
		bID := h.store.Branches.NextID()
		h.store.Branches.Set(bID, store.Branch{
			Name:      "main",
			Commit:    store.BranchCommit{SHA: store.DefaultSHA()},
			RepoOwner: owner,
			RepoName:  req.Name,
		})
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
