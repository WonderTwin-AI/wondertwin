package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// ListBranches handles GET /repos/{owner}/{repo}/branches
func (h *Handler) ListBranches(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	branches := h.store.ListRepoBranches(owner, repo)
	if r.URL.Query().Get("protected") == "true" {
		var protected []store.Branch
		for _, b := range branches {
			if b.Protected {
				protected = append(protected, b)
			}
		}
		branches = protected
	}
	x := h.rd(r)
	out := make([]map[string]any, 0, len(branches))
	for _, b := range paginate(w, r, branches) {
		out = append(out, x.branch(b))
	}
	ghJSON(w, 200, out)
}

// GetBranch handles GET /repos/{owner}/{repo}/branches/{branch}
func (h *Handler) GetBranch(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	branchName := chi.URLParam(r, "branch")

	b, ok := h.store.GetBranch(owner, repo, branchName)
	if !ok {
		ghError(w, 404, "Branch not found")
		return
	}
	x := h.rd(r)
	out := x.branch(b)
	if c, found := h.store.GetCommit(owner, repo, b.Commit.SHA); found {
		out["commit"] = x.commit(c)
	}
	out["_links"] = map[string]any{
		"self": x.api("/repos/%s/%s/branches/%s", owner, repo, b.Name),
		"html": x.web("/%s/%s/tree/%s", owner, repo, b.Name),
	}
	out["protection"] = map[string]any{"enabled": b.Protected, "required_status_checks": map[string]any{"enforcement_level": "off", "contexts": []string{}, "checks": []any{}}}
	ghJSON(w, 200, out)
}

// ListReleases handles GET /repos/{owner}/{repo}/releases
func (h *Handler) ListReleases(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	releases := h.store.ListRepoReleases(owner, repo)
	ghJSON(w, 200, paginate(w, r, releases))
}

// CreateRelease handles POST /repos/{owner}/{repo}/releases
func (h *Handler) CreateRelease(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	var req struct {
		TagName         string `json:"tag_name"`
		Name            string `json:"name"`
		Body            string `json:"body"`
		Draft           bool   `json:"draft"`
		Prerelease      bool   `json:"prerelease"`
		TargetCommitish string `json:"target_commitish"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}

	now := h.store.Now()
	rel := store.Release{
		ID:              h.store.NewID(store.KindRelease),
		TagName:         req.TagName,
		Name:            req.Name,
		Body:            req.Body,
		Draft:           req.Draft,
		Prerelease:      req.Prerelease,
		TargetCommitish: req.TargetCommitish,
		Author:          store.User{ID: 1, Login: "twin-bot", Type: "User"},
		HTMLURL:         h.store.BaseURL() + "/" + owner + "/" + repo + "/releases/tag/" + req.TagName,
		CreatedAt:       now,
		PublishedAt:     now,
		RepoOwner:       owner,
		RepoName:        repo,
	}

	id := h.store.Releases.NextID()
	h.store.Releases.Set(id, rel)
	ghJSON(w, 201, rel)
}

// GetRelease handles GET /repos/{owner}/{repo}/releases/{release_id}
func (h *Handler) GetRelease(w http.ResponseWriter, r *http.Request) {
	releaseID, _ := strconv.ParseInt(chi.URLParam(r, "release_id"), 10, 64)

	_, releases := h.store.Releases.FilterWithIDs(func(_ string, rel store.Release) bool {
		return rel.ID == releaseID
	})
	if len(releases) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, releases[0])
}

// GetLatestRelease handles GET /repos/{owner}/{repo}/releases/latest
func (h *Handler) GetLatestRelease(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	releases := h.store.ListRepoReleases(owner, repo)
	if len(releases) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, releases[len(releases)-1])
}

// DeleteRelease handles DELETE /repos/{owner}/{repo}/releases/{release_id}
func (h *Handler) DeleteRelease(w http.ResponseWriter, r *http.Request) {
	releaseID, _ := strconv.ParseInt(chi.URLParam(r, "release_id"), 10, 64)

	ids, _ := h.store.Releases.FilterWithIDs(func(_ string, rel store.Release) bool {
		return rel.ID == releaseID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.Releases.Delete(ids[0])
	w.WriteHeader(204)
}
