package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

var workflowName = regexp.MustCompile(`(?m)^name:\s*["']?([^"'\n#]+?)["']?\s*(#.*)?$`)

func isWorkflowFile(p string) bool {
	return strings.HasPrefix(p, ".github/workflows/") && strings.Count(p, "/") == 2 &&
		(strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml"))
}

// syncWorkflows registers the workflow files on the default branch, which is
// how GitHub discovers workflows: committing .github/workflows/ci.yml
// creates the ci.yml workflow. Workflows keep their ID when the file changes.
func (h *Handler) syncWorkflows(owner, repo string) {
	head, ok := h.store.ResolveRef(owner, repo, "")
	if !ok {
		return
	}
	for p, sha := range head.Files {
		if !isWorkflowFile(p) {
			continue
		}
		key := store.RepoKey(owner, repo) + "#" + p
		content, _ := h.store.GetBlob(owner, repo, sha)
		name := p
		if m := workflowName.FindSubmatch(content); m != nil {
			name = strings.TrimSpace(string(m[1]))
		}
		wf, exists := h.store.Workflows.Get(key)
		if !exists {
			now := h.store.Now()
			wf = store.Workflow{ID: h.store.NewID(store.KindWorkflow), Path: p, State: "active", CreatedAt: now,
				RepoOwner: owner, RepoName: repo}
			wf.UpdatedAt = now
		}
		if wf.Name != name {
			wf.Name, wf.UpdatedAt = name, h.store.Now()
		}
		h.store.Workflows.Set(key, wf)
	}
}

// workflowFromPath resolves {workflow_id}, which may be the numeric ID or
// the workflow's file name.
func (h *Handler) workflowFromPath(w http.ResponseWriter, r *http.Request) (store.Workflow, bool) {
	owner, repo := chi.URLParam(r, "owner"), chi.URLParam(r, "repo")
	h.syncWorkflows(owner, repo)
	ref := chi.URLParam(r, "workflow_id")
	for _, wf := range h.store.ListRepoWorkflows(owner, repo) {
		if strconv.FormatInt(wf.ID, 10) == ref || pathBase(wf.Path) == ref {
			return wf, true
		}
	}
	ghError(w, 404, "Not Found")
	return store.Workflow{}, false
}

func (x renderer) workflow(wf store.Workflow) map[string]any {
	return map[string]any{
		"id":         wf.ID,
		"node_id":    store.NodeID("W", x.repoID(wf.RepoOwner, wf.RepoName), wf.ID),
		"name":       wf.Name,
		"path":       wf.Path,
		"state":      wf.State,
		"created_at": wf.CreatedAt,
		"updated_at": wf.UpdatedAt,
		"url":        x.api("/repos/%s/%s/actions/workflows/%d", wf.RepoOwner, wf.RepoName, wf.ID),
		"html_url":   x.web("/%s/%s/blob/main/%s", wf.RepoOwner, wf.RepoName, wf.Path),
		"badge_url":  x.web("/%s/%s/workflows/%s/badge.svg", wf.RepoOwner, wf.RepoName, wf.Name),
	}
}

func (x renderer) workflowRun(run store.WorkflowRun) map[string]any {
	owner, repo := run.RepoOwner, run.RepoName
	u := x.api("/repos/%s/%s/actions/runs/%d", owner, repo, run.ID)
	var headCommit any
	if c, ok := x.h.store.GetCommit(owner, repo, run.HeadSHA); ok {
		headCommit = map[string]any{
			"id": c.SHA, "tree_id": c.TreeSHA, "message": c.Message, "timestamp": c.AuthorDate,
			"author":    map[string]any{"name": c.AuthorName, "email": c.AuthorEmail},
			"committer": map[string]any{"name": c.CommitterName, "email": c.CommitterEmail},
		}
	}
	var repository any
	if rp, ok := x.h.store.GetRepo(owner, repo); ok {
		repository = x.repoMinimal(*rp)
	}
	var conclusion any
	if run.Conclusion != "" {
		conclusion = run.Conclusion
	}
	return map[string]any{
		"id":                   run.ID,
		"name":                 run.Name,
		"node_id":              store.NodeID("WFR", x.repoID(owner, repo), run.ID),
		"check_suite_id":       run.SuiteID,
		"check_suite_node_id":  store.NodeID("CS", x.repoID(owner, repo), run.SuiteID),
		"head_branch":          nullable(run.HeadBranch),
		"head_sha":             run.HeadSHA,
		"path":                 run.Path,
		"display_title":        run.Name,
		"run_number":           run.RunNumber,
		"run_attempt":          run.RunAttempt,
		"referenced_workflows": []any{},
		"event":                run.Event,
		"status":               run.Status,
		"conclusion":           conclusion,
		"workflow_id":          run.WorkflowID,
		"url":                  u,
		"html_url":             x.web("/%s/%s/actions/runs/%d", owner, repo, run.ID),
		"pull_requests":        []any{},
		"created_at":           run.CreatedAt,
		"updated_at":           run.UpdatedAt,
		"actor":                x.user(run.Actor.Login),
		"triggering_actor":     x.user(run.Actor.Login),
		"run_started_at":       run.StartedAt,
		"jobs_url":             u + "/jobs",
		"logs_url":             u + "/logs",
		"check_suite_url":      x.api("/repos/%s/%s/check-suites/%d", owner, repo, run.SuiteID),
		"artifacts_url":        u + "/artifacts",
		"cancel_url":           u + "/cancel",
		"rerun_url":            u + "/rerun",
		"previous_attempt_url": nil,
		"workflow_url":         x.api("/repos/%s/%s/actions/workflows/%d", owner, repo, run.WorkflowID),
		"head_commit":          headCommit,
		"repository":           repository,
		"head_repository":      repository,
	}
}

// ListWorkflows handles GET /repos/{owner}/{repo}/actions/workflows
func (h *Handler) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	owner, repo := chi.URLParam(r, "owner"), chi.URLParam(r, "repo")
	h.syncWorkflows(owner, repo)
	wfs := h.store.ListRepoWorkflows(owner, repo)
	sort.Slice(wfs, func(i, j int) bool { return wfs[i].ID < wfs[j].ID })
	x := h.rd(r)
	out := []map[string]any{}
	for _, wf := range paginate(w, r, wfs) {
		out = append(out, x.workflow(wf))
	}
	ghJSON(w, 200, map[string]any{"total_count": len(wfs), "workflows": out})
}

// GetWorkflow handles GET /repos/{owner}/{repo}/actions/workflows/{workflow_id}
func (h *Handler) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	if wf, ok := h.workflowFromPath(w, r); ok {
		ghJSON(w, 200, h.rd(r).workflow(wf))
	}
}

// TriggerWorkflow handles POST
// /repos/{owner}/{repo}/actions/workflows/{workflow_id}/dispatches. In
// 2026-03-10 the response is 200 with the run's ID and URLs. The community
// app emulator does not execute workflows: the run it creates is already
// completed with conclusion success.
func (h *Handler) TriggerWorkflow(w http.ResponseWriter, r *http.Request) {
	owner, repo := chi.URLParam(r, "owner"), chi.URLParam(r, "repo")
	wf, ok := h.workflowFromPath(w, r)
	if !ok {
		return
	}
	var req struct {
		Ref    string         `json:"ref"`
		Inputs map[string]any `json:"inputs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if req.Ref == "" {
		ghValidationErrors(w, "Invalid request.\n\n\"ref\" wasn't supplied.")
		return
	}
	head, found := h.store.ResolveRef(owner, repo, req.Ref)
	_, isBranch := h.store.GetBranch(owner, repo, strings.TrimPrefix(req.Ref, "refs/heads/"))
	_, isTag := h.store.GetTagRef(owner, repo, "refs/tags/"+strings.TrimPrefix(req.Ref, "refs/tags/"))
	if !found || (!isBranch && !isTag) {
		ghValidationErrors(w, "No ref found for: "+req.Ref)
		return
	}
	content, _ := h.store.GetBlob(owner, repo, head.Files[wf.Path])
	if !strings.Contains(string(content), "workflow_dispatch") {
		ghValidationErrors(w, "Workflow does not have 'workflow_dispatch' trigger")
		return
	}

	runs := h.store.ListWorkflowRuns(owner, repo, wf.ID)
	now := h.store.Now()
	run := store.WorkflowRun{
		ID: h.store.NewID(store.KindRun), Name: wf.Name, WorkflowID: wf.ID,
		HeadBranch: strings.TrimPrefix(strings.TrimPrefix(req.Ref, "refs/heads/"), "refs/tags/"), HeadSHA: head.SHA,
		Status: "completed", Conclusion: "success", Event: "workflow_dispatch",
		RunNumber: len(runs) + 1, RunAttempt: 1, Actor: h.userRef(actor(r)),
		CreatedAt: now, UpdatedAt: now, StartedAt: now, Path: wf.Path,
		SuiteID: h.store.NewID(store.KindCheckSuite), Inputs: req.Inputs,
		RepoOwner: owner, RepoName: repo,
	}
	h.store.WorkflowRuns.Set(strconv.FormatInt(run.ID, 10), run)
	x := h.rd(r)
	ghJSON(w, 200, map[string]any{
		"workflow_run_id": run.ID,
		"run_url":         x.api("/repos/%s/%s/actions/runs/%d", owner, repo, run.ID),
		"html_url":        x.web("/%s/%s/actions/runs/%d", owner, repo, run.ID),
	})
}

// writeRuns renders a filtered run list, newest first.
func (h *Handler) writeRuns(w http.ResponseWriter, r *http.Request, runs []store.WorkflowRun) {
	q := r.URL.Query()
	var kept []store.WorkflowRun
	for _, run := range runs {
		if (q.Get("branch") != "" && run.HeadBranch != q.Get("branch")) ||
			(q.Get("event") != "" && run.Event != q.Get("event")) ||
			(q.Get("actor") != "" && run.Actor.Login != q.Get("actor")) ||
			(q.Get("head_sha") != "" && run.HeadSHA != q.Get("head_sha")) ||
			(q.Get("status") != "" && run.Status != q.Get("status") && run.Conclusion != q.Get("status")) {
			continue
		}
		kept = append(kept, run)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].ID > kept[j].ID })
	x := h.rd(r)
	out := []map[string]any{}
	for _, run := range paginate(w, r, kept) {
		out = append(out, x.workflowRun(run))
	}
	ghJSON(w, 200, map[string]any{"total_count": len(kept), "workflow_runs": out})
}

// ListWorkflowRuns handles GET /repos/{owner}/{repo}/actions/runs
func (h *Handler) ListWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	h.writeRuns(w, r, h.store.ListWorkflowRuns(chi.URLParam(r, "owner"), chi.URLParam(r, "repo"), 0))
}

// ListWorkflowRunsForWorkflow handles GET /repos/{owner}/{repo}/actions/workflows/{workflow_id}/runs
func (h *Handler) ListWorkflowRunsForWorkflow(w http.ResponseWriter, r *http.Request) {
	wf, ok := h.workflowFromPath(w, r)
	if !ok {
		return
	}
	h.writeRuns(w, r, h.store.ListWorkflowRuns(wf.RepoOwner, wf.RepoName, wf.ID))
}

func (h *Handler) runFromPath(w http.ResponseWriter, r *http.Request) (store.WorkflowRun, string, bool) {
	runID, _ := strconv.ParseInt(chi.URLParam(r, "run_id"), 10, 64)
	ids, runs := h.store.WorkflowRuns.FilterWithIDs(func(_ string, run store.WorkflowRun) bool {
		return run.ID == runID && run.RepoOwner == chi.URLParam(r, "owner") && run.RepoName == chi.URLParam(r, "repo")
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return store.WorkflowRun{}, "", false
	}
	return runs[0], ids[0], true
}

// GetWorkflowRun handles GET /repos/{owner}/{repo}/actions/runs/{run_id}
func (h *Handler) GetWorkflowRun(w http.ResponseWriter, r *http.Request) {
	if run, _, ok := h.runFromPath(w, r); ok {
		ghJSON(w, 200, h.rd(r).workflowRun(run))
	}
}

// CancelWorkflowRun handles POST /repos/{owner}/{repo}/actions/runs/{run_id}/cancel
func (h *Handler) CancelWorkflowRun(w http.ResponseWriter, r *http.Request) {
	run, id, ok := h.runFromPath(w, r)
	if !ok {
		return
	}
	if run.Status == "completed" {
		ghError(w, 409, "Cannot cancel a workflow run that is completed.")
		return
	}
	run.Status, run.Conclusion, run.UpdatedAt = "completed", "cancelled", h.store.Now()
	h.store.WorkflowRuns.Set(id, run)
	ghJSON(w, 202, map[string]any{})
}

// RerunWorkflow handles POST /repos/{owner}/{repo}/actions/runs/{run_id}/rerun.
// Like a dispatched run, the new attempt is already complete.
func (h *Handler) RerunWorkflow(w http.ResponseWriter, r *http.Request) {
	run, id, ok := h.runFromPath(w, r)
	if !ok {
		return
	}
	if run.Status != "completed" {
		ghError(w, 403, "This workflow is already running")
		return
	}
	now := h.store.Now()
	run.RunAttempt++
	run.Status, run.Conclusion, run.UpdatedAt, run.StartedAt = "completed", "success", now, now
	h.store.WorkflowRuns.Set(id, run)
	ghJSON(w, 201, map[string]any{})
}
