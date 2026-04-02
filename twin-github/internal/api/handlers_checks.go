package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// CreateCheckRun handles POST /repos/{owner}/{repo}/check-runs
func (h *Handler) CreateCheckRun(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	var req struct {
		Name       string           `json:"name"`
		HeadSHA    string           `json:"head_sha"`
		Status     string           `json:"status"`
		Conclusion string           `json:"conclusion,omitempty"`
		DetailsURL string           `json:"details_url,omitempty"`
		ExternalID string           `json:"external_id,omitempty"`
		Output     *store.CheckRunOutput `json:"output,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}

	if req.Status == "" {
		req.Status = "queued"
	}

	now := h.store.Now()
	cr := store.CheckRun{
		ID:         h.store.NextID(),
		Name:       req.Name,
		HeadSHA:    req.HeadSHA,
		Status:     req.Status,
		Conclusion: req.Conclusion,
		StartedAt:  now,
		DetailsURL: req.DetailsURL,
		ExternalID: req.ExternalID,
		Output:     req.Output,
		HTMLURL:    fmt.Sprintf("%s/%s/%s/runs/%d", h.store.BaseURL(), owner, repo, h.store.NextID()),
		RepoOwner:  owner,
		RepoName:   repo,
	}

	if req.Status == "completed" {
		cr.CompletedAt = now
	}

	id := h.store.CheckRuns.NextID()
	h.store.CheckRuns.Set(id, cr)
	ghJSON(w, 201, cr)
}

// GetCheckRun handles GET /repos/{owner}/{repo}/check-runs/{check_run_id}
func (h *Handler) GetCheckRun(w http.ResponseWriter, r *http.Request) {
	crID, _ := strconv.ParseInt(chi.URLParam(r, "check_run_id"), 10, 64)

	_, runs := h.store.CheckRuns.FilterWithIDs(func(_ string, cr store.CheckRun) bool {
		return cr.ID == crID
	})
	if len(runs) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, runs[0])
}

// UpdateCheckRun handles PATCH /repos/{owner}/{repo}/check-runs/{check_run_id}
func (h *Handler) UpdateCheckRun(w http.ResponseWriter, r *http.Request) {
	crID, _ := strconv.ParseInt(chi.URLParam(r, "check_run_id"), 10, 64)

	ids, runs := h.store.CheckRuns.FilterWithIDs(func(_ string, cr store.CheckRun) bool {
		return cr.ID == crID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}

	var req map[string]any
	json.NewDecoder(r.Body).Decode(&req)

	cr := runs[0]
	if status, ok := req["status"].(string); ok {
		cr.Status = status
	}
	if conclusion, ok := req["conclusion"].(string); ok {
		cr.Conclusion = conclusion
	}
	if cr.Status == "completed" && cr.CompletedAt == "" {
		cr.CompletedAt = h.store.Now()
	}

	h.store.CheckRuns.Set(ids[0], cr)
	ghJSON(w, 200, cr)
}

// ListCheckRunsForRef handles GET /repos/{owner}/{repo}/commits/{ref}/check-runs
func (h *Handler) ListCheckRunsForRef(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	ref := chi.URLParam(r, "ref")

	runs := h.store.ListCheckRunsForRef(owner, repo, ref)
	ghJSON(w, 200, map[string]any{
		"total_count": len(runs),
		"check_runs":  runs,
	})
}

// CreateCheckSuite handles POST /repos/{owner}/{repo}/check-suites
func (h *Handler) CreateCheckSuite(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	var req struct {
		HeadSHA    string `json:"head_sha"`
		HeadBranch string `json:"head_branch,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}

	now := h.store.Now()
	cs := store.CheckSuite{
		ID:         h.store.NextID(),
		HeadSHA:    req.HeadSHA,
		HeadBranch: req.HeadBranch,
		Status:     "queued",
		CreatedAt:  now,
		UpdatedAt:  now,
		RepoOwner:  owner,
		RepoName:   repo,
	}

	id := h.store.CheckSuites.NextID()
	h.store.CheckSuites.Set(id, cs)
	ghJSON(w, 201, cs)
}

// GetCheckSuite handles GET /repos/{owner}/{repo}/check-suites/{check_suite_id}
func (h *Handler) GetCheckSuite(w http.ResponseWriter, r *http.Request) {
	csID, _ := strconv.ParseInt(chi.URLParam(r, "check_suite_id"), 10, 64)

	_, suites := h.store.CheckSuites.FilterWithIDs(func(_ string, cs store.CheckSuite) bool {
		return cs.ID == csID
	})
	if len(suites) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, suites[0])
}

// ListCheckSuitesForRef handles GET /repos/{owner}/{repo}/commits/{ref}/check-suites
func (h *Handler) ListCheckSuitesForRef(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	ref := chi.URLParam(r, "ref")

	suites := h.store.CheckSuites.Filter(func(_ string, cs store.CheckSuite) bool {
		return cs.RepoOwner == owner && cs.RepoName == repo && cs.HeadSHA == ref
	})
	ghJSON(w, 200, map[string]any{
		"total_count":  len(suites),
		"check_suites": suites,
	})
}
