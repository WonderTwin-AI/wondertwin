package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// ListDeployKeys handles GET /repos/{owner}/{repo}/keys
func (h *Handler) ListDeployKeys(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	keys := h.store.ListRepoDeployKeys(owner, repo)
	ghJSON(w, 200, keys)
}

// CreateDeployKey handles POST /repos/{owner}/{repo}/keys
func (h *Handler) CreateDeployKey(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	var req struct {
		Title    string `json:"title"`
		Key      string `json:"key"`
		ReadOnly bool   `json:"read_only"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}

	dk := store.DeployKey{
		ID:        h.store.NextID(),
		Key:       req.Key,
		Title:     req.Title,
		ReadOnly:  req.ReadOnly,
		CreatedAt: h.store.Now(),
		Verified:  true,
		RepoOwner: owner,
		RepoName:  repo,
	}
	id := h.store.DeployKeys.NextID()
	h.store.DeployKeys.Set(id, dk)
	ghJSON(w, 201, dk)
}

// GetDeployKey handles GET /repos/{owner}/{repo}/keys/{key_id}
func (h *Handler) GetDeployKey(w http.ResponseWriter, r *http.Request) {
	keyID, _ := strconv.ParseInt(chi.URLParam(r, "key_id"), 10, 64)

	_, keys := h.store.DeployKeys.FilterWithIDs(func(_ string, dk store.DeployKey) bool {
		return dk.ID == keyID
	})
	if len(keys) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, keys[0])
}

// DeleteDeployKey handles DELETE /repos/{owner}/{repo}/keys/{key_id}
func (h *Handler) DeleteDeployKey(w http.ResponseWriter, r *http.Request) {
	keyID, _ := strconv.ParseInt(chi.URLParam(r, "key_id"), 10, 64)

	ids, _ := h.store.DeployKeys.FilterWithIDs(func(_ string, dk store.DeployKey) bool {
		return dk.ID == keyID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.DeployKeys.Delete(ids[0])
	w.WriteHeader(204)
}

// ListDeployments handles GET /repos/{owner}/{repo}/deployments
func (h *Handler) ListDeployments(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	deployments := h.store.ListRepoDeployments(owner, repo)
	ghJSON(w, 200, deployments)
}

// CreateDeployment handles POST /repos/{owner}/{repo}/deployments
func (h *Handler) CreateDeployment(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	var req struct {
		Ref         string `json:"ref"`
		Task        string `json:"task"`
		Environment string `json:"environment"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if req.Task == "" {
		req.Task = "deploy"
	}
	if req.Environment == "" {
		req.Environment = "production"
	}

	now := h.store.Now()
	d := store.Deployment{
		ID:          h.store.NextID(),
		Ref:         req.Ref,
		Task:        req.Task,
		Environment: req.Environment,
		Description: req.Description,
		Creator:     store.User{ID: 1, Login: "twin-bot", Type: "User"},
		SHA:         store.MakeSHA(req.Ref),
		CreatedAt:   now,
		UpdatedAt:   now,
		RepoOwner:   owner,
		RepoName:    repo,
	}
	id := h.store.Deployments.NextID()
	h.store.Deployments.Set(id, d)
	ghJSON(w, 201, d)
}

// GetDeployment handles GET /repos/{owner}/{repo}/deployments/{deployment_id}
func (h *Handler) GetDeployment(w http.ResponseWriter, r *http.Request) {
	deployID, _ := strconv.ParseInt(chi.URLParam(r, "deployment_id"), 10, 64)

	_, deps := h.store.Deployments.FilterWithIDs(func(_ string, d store.Deployment) bool {
		return d.ID == deployID
	})
	if len(deps) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, deps[0])
}

// ListDeploymentStatuses handles GET /repos/{owner}/{repo}/deployments/{deployment_id}/statuses
func (h *Handler) ListDeploymentStatuses(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	deployID, _ := strconv.ParseInt(chi.URLParam(r, "deployment_id"), 10, 64)

	statuses := h.store.ListDeploymentStatuses(owner, repo, deployID)
	ghJSON(w, 200, statuses)
}

// CreateDeploymentStatus handles POST /repos/{owner}/{repo}/deployments/{deployment_id}/statuses
func (h *Handler) CreateDeploymentStatus(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	deployID, _ := strconv.ParseInt(chi.URLParam(r, "deployment_id"), 10, 64)

	var req struct {
		State          string `json:"state"`
		Description    string `json:"description"`
		Environment    string `json:"environment"`
		EnvironmentURL string `json:"environment_url"`
		LogURL         string `json:"log_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}

	now := h.store.Now()
	ds := store.DeploymentStatus{
		ID:             h.store.NextID(),
		State:          req.State,
		Description:    req.Description,
		Environment:    req.Environment,
		EnvironmentURL: req.EnvironmentURL,
		LogURL:         req.LogURL,
		Creator:        store.User{ID: 1, Login: "twin-bot", Type: "User"},
		CreatedAt:      now,
		UpdatedAt:      now,
		RepoOwner:      owner,
		RepoName:       repo,
		DeploymentID:   deployID,
	}
	id := h.store.DeployStatuses.NextID()
	h.store.DeployStatuses.Set(id, ds)
	ghJSON(w, 201, ds)
}
