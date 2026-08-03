package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// CreateCommitStatus handles POST /repos/{owner}/{repo}/statuses/{sha}
func (h *Handler) CreateCommitStatus(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	sha := chi.URLParam(r, "sha")

	var req struct {
		State       string `json:"state"`
		TargetURL   string `json:"target_url"`
		Description string `json:"description"`
		Context     string `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}

	now := h.store.Now()
	cs := store.CommitStatus{
		ID:          h.store.NextID(),
		State:       req.State,
		TargetURL:   req.TargetURL,
		Description: req.Description,
		Context:     req.Context,
		Creator:     store.User{ID: 1, Login: "twin-bot", Type: "User"},
		CreatedAt:   now,
		UpdatedAt:   now,
		RepoOwner:   owner,
		RepoName:    repo,
		SHA:         sha,
	}

	id := h.store.Statuses.NextID()
	h.store.Statuses.Set(id, cs)
	ghJSON(w, 201, cs)
}

// ListCommitStatuses handles GET /repos/{owner}/{repo}/commits/{ref}/statuses
func (h *Handler) ListCommitStatuses(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	ref := chi.URLParam(r, "ref")

	statuses := h.store.ListRepoStatuses(owner, repo, ref)
	ghJSON(w, 200, statuses)
}

// GetCombinedStatus handles GET /repos/{owner}/{repo}/commits/{ref}/status
func (h *Handler) GetCombinedStatus(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	ref := chi.URLParam(r, "ref")

	statuses := h.store.ListRepoStatuses(owner, repo, ref)

	// Compute combined state
	combined := "pending"
	if len(statuses) > 0 {
		allSuccess := true
		hasFailure := false
		for _, s := range statuses {
			if s.State == "failure" || s.State == "error" {
				hasFailure = true
				allSuccess = false
			} else if s.State != "success" {
				allSuccess = false
			}
		}
		if hasFailure {
			combined = "failure"
		} else if allSuccess {
			combined = "success"
		}
	}

	ghJSON(w, 200, map[string]any{
		"state":       combined,
		"statuses":    statuses,
		"sha":         ref,
		"total_count": len(statuses),
		"repository": map[string]any{
			"full_name": owner + "/" + repo,
		},
	})
}

// ListBranches handles GET /repos/{owner}/{repo}/branches
func (h *Handler) ListBranches(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	branches := h.store.ListRepoBranches(owner, repo)
	ghJSON(w, 200, branches)
}

// GetBranch handles GET /repos/{owner}/{repo}/branches/{branch}
func (h *Handler) GetBranch(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	branchName := chi.URLParam(r, "branch")

	branches := h.store.ListRepoBranches(owner, repo)
	for _, b := range branches {
		if b.Name == branchName {
			ghJSON(w, 200, b)
			return
		}
	}
	ghError(w, 404, "Branch not found")
}

// ListReleases handles GET /repos/{owner}/{repo}/releases
func (h *Handler) ListReleases(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	releases := h.store.ListRepoReleases(owner, repo)
	ghJSON(w, 200, releases)
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
		ID:              h.store.NextID(),
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
