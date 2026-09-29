package api

import (
	"net/http"
	"strconv"

	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// --- Workflow Runs ---

// DeleteWorkflowRun handles DELETE /repos/{owner}/{repo}/actions/runs/{run_id}
func (h *Handler) DeleteWorkflowRun(w http.ResponseWriter, r *http.Request) {
	runID, _ := strconv.ParseInt(param(r, "run_id"), 10, 64)
	ids, _ := h.store.WorkflowRuns.FilterWithIDs(func(_ string, run store.WorkflowRun) bool {
		return run.ID == runID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.WorkflowRuns.Delete(ids[0])
	w.WriteHeader(204)
}

// RerunFailedJobs handles POST /repos/{owner}/{repo}/actions/runs/{run_id}/rerun-failed-jobs
func (h *Handler) RerunFailedJobs(w http.ResponseWriter, r *http.Request) {
	// Same behavior as rerun for the twin
	h.RerunWorkflow(w, r)
}

// --- Jobs ---

// ListRunJobs handles GET /repos/{owner}/{repo}/actions/runs/{run_id}/jobs
func (h *Handler) ListRunJobs(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	runID, _ := strconv.ParseInt(param(r, "run_id"), 10, 64)
	jobs := h.store.ListRunJobs(owner, repo, runID)
	ghJSON(w, 200, map[string]any{"total_count": len(jobs), "jobs": paginate(w, r, jobs)})
}

// GetJob handles GET /repos/{owner}/{repo}/actions/jobs/{job_id}
func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	jobID, _ := strconv.ParseInt(param(r, "job_id"), 10, 64)
	_, jobs := h.store.WorkflowJobs.FilterWithIDs(func(_ string, j store.WorkflowJob) bool {
		return j.ID == jobID
	})
	if len(jobs) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, jobs[0])
}

// --- Artifacts ---

// ListRunArtifacts handles GET /repos/{owner}/{repo}/actions/runs/{run_id}/artifacts
func (h *Handler) ListRunArtifacts(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	runID, _ := strconv.ParseInt(param(r, "run_id"), 10, 64)
	arts := h.store.ListRunArtifacts(owner, repo, runID)
	ghJSON(w, 200, map[string]any{"total_count": len(arts), "artifacts": paginate(w, r, arts)})
}

// ListRepoArtifacts handles GET /repos/{owner}/{repo}/actions/artifacts
func (h *Handler) ListRepoArtifacts(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	arts := h.store.ListRepoArtifacts(owner, repo)
	ghJSON(w, 200, map[string]any{"total_count": len(arts), "artifacts": paginate(w, r, arts)})
}

// GetArtifact handles GET /repos/{owner}/{repo}/actions/artifacts/{artifact_id}
func (h *Handler) GetArtifact(w http.ResponseWriter, r *http.Request) {
	artID, _ := strconv.ParseInt(param(r, "artifact_id"), 10, 64)
	_, arts := h.store.Artifacts.FilterWithIDs(func(_ string, a store.Artifact) bool {
		return a.ID == artID
	})
	if len(arts) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, arts[0])
}

// DeleteArtifact handles DELETE /repos/{owner}/{repo}/actions/artifacts/{artifact_id}
func (h *Handler) DeleteArtifact(w http.ResponseWriter, r *http.Request) {
	artID, _ := strconv.ParseInt(param(r, "artifact_id"), 10, 64)
	ids, _ := h.store.Artifacts.FilterWithIDs(func(_ string, a store.Artifact) bool {
		return a.ID == artID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.Artifacts.Delete(ids[0])
	w.WriteHeader(204)
}

// --- Secrets ---

// ListRepoSecrets handles GET /repos/{owner}/{repo}/actions/secrets
func (h *Handler) ListRepoSecrets(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	secrets := h.store.ListRepoSecrets(owner, repo)
	ghJSON(w, 200, map[string]any{"total_count": len(secrets), "secrets": paginate(w, r, secrets)})
}

// GetRepoSecret handles GET /repos/{owner}/{repo}/actions/secrets/{secret_name}
func (h *Handler) GetRepoSecret(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	name := param(r, "secret_name")

	secrets := h.store.Secrets.Filter(func(_ string, s store.Secret) bool {
		return s.RepoOwner == owner && s.RepoName == repo && s.Name == name
	})
	if len(secrets) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, secrets[0])
}

// CreateOrUpdateRepoSecret handles PUT /repos/{owner}/{repo}/actions/secrets/{secret_name}
func (h *Handler) CreateOrUpdateRepoSecret(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	name := param(r, "secret_name")

	now := h.store.Now()

	// Check if exists
	ids, _ := h.store.Secrets.FilterWithIDs(func(_ string, s store.Secret) bool {
		return s.RepoOwner == owner && s.RepoName == repo && s.Name == name
	})

	status := 201
	if len(ids) > 0 {
		status = 204
		h.store.Secrets.Delete(ids[0])
	}

	sec := store.Secret{
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
		RepoOwner: owner,
		RepoName:  repo,
	}
	id := h.store.Secrets.NextID()
	h.store.Secrets.Set(id, sec)
	w.WriteHeader(status)
}

// DeleteRepoSecret handles DELETE /repos/{owner}/{repo}/actions/secrets/{secret_name}
func (h *Handler) DeleteRepoSecret(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	name := param(r, "secret_name")

	ids, _ := h.store.Secrets.FilterWithIDs(func(_ string, s store.Secret) bool {
		return s.RepoOwner == owner && s.RepoName == repo && s.Name == name
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.Secrets.Delete(ids[0])
	w.WriteHeader(204)
}

// GetRepoPublicKey handles GET /repos/{owner}/{repo}/actions/secrets/public-key
func (h *Handler) GetRepoPublicKey(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{
		"key_id": "012345678912345678",
		"key":    "2Sg8iYjAxxmI2LvUXpJjkYrMxURPc8r+dB7TJyvv1234",
	})
}
