package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// --- Workflows ---

// ListWorkflows handles GET /repos/{owner}/{repo}/actions/workflows
func (h *Handler) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	wfs := h.store.ListRepoWorkflows(owner, repo)
	ghJSON(w, 200, map[string]any{"total_count": len(wfs), "workflows": wfs})
}

// GetWorkflow handles GET /repos/{owner}/{repo}/actions/workflows/{workflow_id}
func (h *Handler) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	wfID, _ := strconv.ParseInt(chi.URLParam(r, "workflow_id"), 10, 64)
	_, wfs := h.store.Workflows.FilterWithIDs(func(_ string, wf store.Workflow) bool {
		return wf.ID == wfID
	})
	if len(wfs) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, wfs[0])
}

// TriggerWorkflow handles POST /repos/{owner}/{repo}/actions/workflows/{workflow_id}/dispatches
func (h *Handler) TriggerWorkflow(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	wfID, _ := strconv.ParseInt(chi.URLParam(r, "workflow_id"), 10, 64)

	var req struct {
		Ref    string         `json:"ref"`
		Inputs map[string]any `json:"inputs,omitempty"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Create a run for the workflow
	now := h.store.Now()
	run := store.WorkflowRun{
		ID:         h.store.NextID(),
		WorkflowID: wfID,
		Name:       "workflow_dispatch",
		HeadBranch: req.Ref,
		HeadSHA:    store.MakeSHA(req.Ref + now),
		Status:     "queued",
		Event:      "workflow_dispatch",
		RunNumber:  h.store.NextRunNumber(),
		RunAttempt: 1,
		Actor:      store.User{ID: 1, Login: "twin-bot", Type: "User"},
		HTMLURL:    fmt.Sprintf("%s/%s/%s/actions/runs/%d", h.store.BaseURL(), owner, repo, h.store.NextID()),
		CreatedAt:  now,
		UpdatedAt:  now,
		RepoOwner:  owner,
		RepoName:   repo,
	}
	id := h.store.WorkflowRuns.NextID()
	h.store.WorkflowRuns.Set(id, run)

	w.WriteHeader(204)
}

// --- Workflow Runs ---

// ListWorkflowRuns handles GET /repos/{owner}/{repo}/actions/runs
func (h *Handler) ListWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	runs := h.store.ListWorkflowRuns(owner, repo, 0)
	ghJSON(w, 200, map[string]any{"total_count": len(runs), "workflow_runs": runs})
}

// ListWorkflowRunsForWorkflow handles GET /repos/{owner}/{repo}/actions/workflows/{workflow_id}/runs
func (h *Handler) ListWorkflowRunsForWorkflow(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	wfID, _ := strconv.ParseInt(chi.URLParam(r, "workflow_id"), 10, 64)
	runs := h.store.ListWorkflowRuns(owner, repo, wfID)
	ghJSON(w, 200, map[string]any{"total_count": len(runs), "workflow_runs": runs})
}

// GetWorkflowRun handles GET /repos/{owner}/{repo}/actions/runs/{run_id}
func (h *Handler) GetWorkflowRun(w http.ResponseWriter, r *http.Request) {
	runID, _ := strconv.ParseInt(chi.URLParam(r, "run_id"), 10, 64)
	_, runs := h.store.WorkflowRuns.FilterWithIDs(func(_ string, run store.WorkflowRun) bool {
		return run.ID == runID
	})
	if len(runs) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, runs[0])
}

// DeleteWorkflowRun handles DELETE /repos/{owner}/{repo}/actions/runs/{run_id}
func (h *Handler) DeleteWorkflowRun(w http.ResponseWriter, r *http.Request) {
	runID, _ := strconv.ParseInt(chi.URLParam(r, "run_id"), 10, 64)
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

// CancelWorkflowRun handles POST /repos/{owner}/{repo}/actions/runs/{run_id}/cancel
func (h *Handler) CancelWorkflowRun(w http.ResponseWriter, r *http.Request) {
	runID, _ := strconv.ParseInt(chi.URLParam(r, "run_id"), 10, 64)
	ids, runs := h.store.WorkflowRuns.FilterWithIDs(func(_ string, run store.WorkflowRun) bool {
		return run.ID == runID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	run := runs[0]
	run.Status = "completed"
	run.Conclusion = "cancelled"
	run.UpdatedAt = h.store.Now()
	h.store.WorkflowRuns.Set(ids[0], run)
	w.WriteHeader(202)
}

// RerunWorkflow handles POST /repos/{owner}/{repo}/actions/runs/{run_id}/rerun
func (h *Handler) RerunWorkflow(w http.ResponseWriter, r *http.Request) {
	runID, _ := strconv.ParseInt(chi.URLParam(r, "run_id"), 10, 64)
	ids, runs := h.store.WorkflowRuns.FilterWithIDs(func(_ string, run store.WorkflowRun) bool {
		return run.ID == runID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	run := runs[0]
	run.Status = "queued"
	run.Conclusion = ""
	run.RunAttempt++
	run.UpdatedAt = h.store.Now()
	h.store.WorkflowRuns.Set(ids[0], run)
	w.WriteHeader(201)
}

// RerunFailedJobs handles POST /repos/{owner}/{repo}/actions/runs/{run_id}/rerun-failed-jobs
func (h *Handler) RerunFailedJobs(w http.ResponseWriter, r *http.Request) {
	// Same behavior as rerun for the twin
	h.RerunWorkflow(w, r)
}

// --- Jobs ---

// ListRunJobs handles GET /repos/{owner}/{repo}/actions/runs/{run_id}/jobs
func (h *Handler) ListRunJobs(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	runID, _ := strconv.ParseInt(chi.URLParam(r, "run_id"), 10, 64)
	jobs := h.store.ListRunJobs(owner, repo, runID)
	ghJSON(w, 200, map[string]any{"total_count": len(jobs), "jobs": jobs})
}

// GetJob handles GET /repos/{owner}/{repo}/actions/jobs/{job_id}
func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	jobID, _ := strconv.ParseInt(chi.URLParam(r, "job_id"), 10, 64)
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
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	runID, _ := strconv.ParseInt(chi.URLParam(r, "run_id"), 10, 64)
	arts := h.store.ListRunArtifacts(owner, repo, runID)
	ghJSON(w, 200, map[string]any{"total_count": len(arts), "artifacts": arts})
}

// ListRepoArtifacts handles GET /repos/{owner}/{repo}/actions/artifacts
func (h *Handler) ListRepoArtifacts(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	arts := h.store.ListRepoArtifacts(owner, repo)
	ghJSON(w, 200, map[string]any{"total_count": len(arts), "artifacts": arts})
}

// GetArtifact handles GET /repos/{owner}/{repo}/actions/artifacts/{artifact_id}
func (h *Handler) GetArtifact(w http.ResponseWriter, r *http.Request) {
	artID, _ := strconv.ParseInt(chi.URLParam(r, "artifact_id"), 10, 64)
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
	artID, _ := strconv.ParseInt(chi.URLParam(r, "artifact_id"), 10, 64)
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
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	secrets := h.store.ListRepoSecrets(owner, repo)
	ghJSON(w, 200, map[string]any{"total_count": len(secrets), "secrets": secrets})
}

// GetRepoSecret handles GET /repos/{owner}/{repo}/actions/secrets/{secret_name}
func (h *Handler) GetRepoSecret(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	name := chi.URLParam(r, "secret_name")

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
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	name := chi.URLParam(r, "secret_name")

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
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	name := chi.URLParam(r, "secret_name")

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
