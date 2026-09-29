package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// --- Commit statuses ---

var statusStates = map[string]bool{"error": true, "failure": true, "pending": true, "success": true}

func (x renderer) status(cs store.CommitStatus) map[string]any {
	creator := x.user(cs.Creator.Login)
	return map[string]any{
		"url":         x.api("/repos/%s/%s/statuses/%s", cs.RepoOwner, cs.RepoName, cs.SHA),
		"avatar_url":  creator["avatar_url"],
		"id":          cs.ID,
		"node_id":     store.NodeID("SC", x.repoID(cs.RepoOwner, cs.RepoName), cs.ID),
		"state":       cs.State,
		"description": nullable(cs.Description),
		"target_url":  nullable(cs.TargetURL),
		"context":     cs.Context,
		"created_at":  cs.CreatedAt,
		"updated_at":  cs.UpdatedAt,
		"creator":     creator,
	}
}

// resolveSHA turns a ref into a commit SHA, writing GitHub's 422 when no
// commit matches.
func (h *Handler) resolveSHA(w http.ResponseWriter, owner, repo, ref string) (string, bool) {
	c, ok := h.store.ResolveRef(owner, repo, ref)
	if !ok || ref == "" {
		ghValidationErrors(w, "No commit found for SHA: "+ref)
		return "", false
	}
	return c.SHA, true
}

// CreateCommitStatus handles POST /repos/{owner}/{repo}/statuses/{sha}
func (h *Handler) CreateCommitStatus(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	if _, ok := h.store.GetRepo(owner, repo); !ok {
		ghError(w, 404, "Not Found")
		return
	}
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
	if !statusStates[req.State] {
		ghValidationErrors(w, "Validation Failed", map[string]any{
			"resource": "Status", "code": "custom", "field": "state", "message": "state is not included in the list",
		})
		return
	}
	sha, ok := h.resolveSHA(w, owner, repo, param(r, "sha"))
	if !ok {
		return
	}
	if req.Context == "" {
		req.Context = "default"
	}
	now := h.store.Now()
	cs := store.CommitStatus{
		ID: h.store.NewID(store.KindStatus), State: req.State, TargetURL: req.TargetURL,
		Description: req.Description, Context: req.Context, Creator: h.userRef(actor(r)),
		CreatedAt: now, UpdatedAt: now, RepoOwner: owner, RepoName: repo, SHA: sha,
	}
	h.store.Statuses.Set(h.store.Statuses.NextID(), cs)
	ghJSON(w, 201, h.rd(r).status(cs))
}

// statusesNewestFirst lists a commit's statuses, newest first.
func (h *Handler) statusesNewestFirst(owner, repo, sha string) []store.CommitStatus {
	all := h.store.ListRepoStatuses(owner, repo, sha)
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all
}

// ListCommitStatuses handles GET /repos/{owner}/{repo}/commits/{ref}/statuses
func (h *Handler) ListCommitStatuses(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	c, ok := h.store.ResolveRef(owner, repo, param(r, "ref"))
	out := []map[string]any{}
	if ok {
		x := h.rd(r)
		for _, s := range paginate(w, r, h.statusesNewestFirst(owner, repo, c.SHA)) {
			out = append(out, x.status(s))
		}
	}
	ghJSON(w, 200, out)
}

// GetCombinedStatus handles GET /repos/{owner}/{repo}/commits/{ref}/status.
// Only the newest status per context counts. The combined state is failure
// if any is error or failure, pending if any is pending or there are none,
// and success otherwise.
func (h *Handler) GetCombinedStatus(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	rp, ok := h.store.GetRepo(owner, repo)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ref := param(r, "ref")
	c, ok := h.store.ResolveRef(owner, repo, ref)
	if !ok {
		ghError(w, 404, "No commit found for SHA: "+ref)
		return
	}

	var latest []store.CommitStatus
	seen := map[string]bool{}
	for _, s := range h.statusesNewestFirst(owner, repo, c.SHA) {
		if !seen[s.Context] {
			seen[s.Context] = true
			latest = append(latest, s)
		}
	}
	state := "success"
	if len(latest) == 0 {
		state = "pending"
	}
	for _, s := range latest {
		switch s.State {
		case "error", "failure":
			state = "failure"
		case "pending":
			if state != "failure" {
				state = "pending"
			}
		}
	}

	x := h.rd(r)
	statuses := []map[string]any{}
	for _, s := range paginate(w, r, latest) {
		m := x.status(s)
		delete(m, "creator")
		m["required"] = false
		statuses = append(statuses, m)
	}
	ghJSON(w, 200, map[string]any{
		"state":       state,
		"statuses":    statuses,
		"sha":         c.SHA,
		"total_count": len(latest),
		"repository":  x.repoMinimal(*rp),
		"commit_url":  x.api("/repos/%s/%s/commits/%s", owner, repo, c.SHA),
		"url":         x.api("/repos/%s/%s/commits/%s/status", owner, repo, c.SHA),
	})
}

// --- Check runs and suites ---

var (
	checkStatuses    = map[string]bool{"queued": true, "in_progress": true, "completed": true, "waiting": true, "requested": true, "pending": true}
	checkConclusions = map[string]bool{"action_required": true, "cancelled": true, "failure": true, "neutral": true, "success": true, "skipped": true, "stale": true, "timed_out": true}
)

// checkApp returns the App behind an installation token. Check-run writes
// are GitHub App only.
func (h *Handler) checkApp(w http.ResponseWriter, r *http.Request) (store.App, bool) {
	p := principalFrom(r)
	if p.Kind == principalInstallation {
		if inst, ok := h.store.GetInstallation(p.InstallationID); ok {
			if app, found := h.store.GetApp(inst.AppID); found {
				return app, true
			}
		}
	}
	ghError(w, 403, "You must authenticate via a GitHub App.")
	return store.App{}, false
}

// suiteFor finds or creates the check suite for an app on a commit.
func (h *Handler) suiteFor(owner, repo, sha string, appID int64) store.CheckSuite {
	for _, cs := range h.store.CheckSuites.Filter(func(_ string, cs store.CheckSuite) bool {
		return cs.RepoOwner == owner && cs.RepoName == repo && cs.HeadSHA == sha && cs.AppID == appID
	}) {
		return cs
	}
	branch := ""
	for _, b := range h.store.ListRepoBranches(owner, repo) {
		if b.Commit.SHA == sha {
			branch = b.Name
		}
	}
	now := h.store.Now()
	cs := store.CheckSuite{
		ID: h.store.NewID(store.KindCheckSuite), HeadSHA: sha, HeadBranch: branch, Status: "queued",
		CreatedAt: now, UpdatedAt: now, AppID: appID, RepoOwner: owner, RepoName: repo,
	}
	h.store.CheckSuites.Set(strconv.FormatInt(cs.ID, 10), cs)
	return cs
}

// refreshSuite derives a suite's status and conclusion from its runs.
func (h *Handler) refreshSuite(id int64) {
	cs, ok := h.store.CheckSuites.Get(strconv.FormatInt(id, 10))
	if !ok {
		return
	}
	runs := h.store.CheckRuns.Filter(func(_ string, cr store.CheckRun) bool { return cr.SuiteID == id })
	cs.Status, cs.Conclusion = "completed", "success"
	rank := map[string]int{"success": 0, "skipped": 0, "neutral": 1, "stale": 2, "cancelled": 3, "timed_out": 4, "action_required": 5, "failure": 6}
	for _, cr := range runs {
		if cr.Status != "completed" {
			cs.Status, cs.Conclusion = "in_progress", ""
			break
		}
		if rank[cr.Conclusion] > rank[cs.Conclusion] {
			cs.Conclusion = cr.Conclusion
		}
	}
	cs.UpdatedAt = h.store.Now()
	h.store.CheckSuites.Set(strconv.FormatInt(id, 10), cs)
}

// pullsForSHA lists the open pull requests whose head is sha, in the
// pull-request-minimal shape check runs carry.
func (x renderer) pullsForSHA(owner, repo, sha string) []map[string]any {
	out := []map[string]any{}
	repoRef := map[string]any{"id": x.repoID(owner, repo), "url": x.api("/repos/%s/%s", owner, repo), "name": repo}
	for _, p := range x.h.store.ListRepoPRs(owner, repo, "open") {
		st := x.h.prState(p)
		if st.headSHA != sha {
			continue
		}
		out = append(out, map[string]any{
			"id": p.ID, "number": p.Number, "url": x.api("/repos/%s/%s/pulls/%d", owner, repo, p.Number),
			"head": map[string]any{"ref": p.Head.Ref, "sha": st.headSHA, "repo": repoRef},
			"base": map[string]any{"ref": p.Base.Ref, "sha": st.baseSHA, "repo": repoRef},
		})
	}
	return out
}

func (x renderer) checkRun(cr store.CheckRun) map[string]any {
	owner, repo := cr.RepoOwner, cr.RepoName
	u := x.api("/repos/%s/%s/check-runs/%d", owner, repo, cr.ID)
	out := store.CheckRunOutput{}
	if cr.Output != nil {
		out = *cr.Output
	}
	var app any
	if a, ok := x.h.store.GetApp(cr.AppID); ok {
		app = x.app(a)
	}
	return map[string]any{
		"id":           cr.ID,
		"head_sha":     cr.HeadSHA,
		"node_id":      store.NodeID("CR", x.repoID(owner, repo), cr.ID),
		"external_id":  cr.ExternalID,
		"url":          u,
		"html_url":     x.web("/%s/%s/runs/%d", owner, repo, cr.ID),
		"details_url":  nullable(cr.DetailsURL),
		"status":       cr.Status,
		"conclusion":   nullable(cr.Conclusion),
		"started_at":   nullable(cr.StartedAt),
		"completed_at": nullable(cr.CompletedAt),
		"output": map[string]any{
			"title": nullable(out.Title), "summary": nullable(out.Summary), "text": nullable(out.Text),
			"annotations_count": cr.Annotations, "annotations_url": u + "/annotations",
		},
		"name":          cr.Name,
		"check_suite":   map[string]any{"id": cr.SuiteID},
		"app":           app,
		"pull_requests": x.pullsForSHA(owner, repo, cr.HeadSHA),
	}
}

func (x renderer) checkSuite(cs store.CheckSuite) map[string]any {
	owner, repo := cs.RepoOwner, cs.RepoName
	var app any
	if a, ok := x.h.store.GetApp(cs.AppID); ok {
		app = x.app(a)
	}
	runs := len(x.h.store.CheckRuns.Filter(func(_ string, cr store.CheckRun) bool { return cr.SuiteID == cs.ID }))
	var head any
	if c, ok := x.h.store.GetCommit(owner, repo, cs.HeadSHA); ok {
		head = map[string]any{"id": c.SHA, "tree_id": c.TreeSHA, "message": c.Message, "timestamp": c.AuthorDate,
			"author": map[string]any{"name": c.AuthorName, "email": c.AuthorEmail}, "committer": map[string]any{"name": c.CommitterName, "email": c.CommitterEmail}}
	}
	rp, _ := x.h.store.GetRepo(owner, repo)
	var repository any
	if rp != nil {
		repository = x.repoMinimal(*rp)
	}
	return map[string]any{
		"id": cs.ID, "node_id": store.NodeID("CS", x.repoID(owner, repo), cs.ID), "head_branch": nullable(cs.HeadBranch),
		"head_sha": cs.HeadSHA, "status": cs.Status, "conclusion": nullable(cs.Conclusion),
		"url": x.api("/repos/%s/%s/check-suites/%d", owner, repo, cs.ID), "before": nil, "after": cs.HeadSHA,
		"pull_requests": x.pullsForSHA(owner, repo, cs.HeadSHA), "app": app, "repository": repository,
		"created_at": cs.CreatedAt, "updated_at": cs.UpdatedAt, "rerequestable": true, "runs_rerequestable": true,
		"latest_check_runs_count": runs, "check_runs_url": x.api("/repos/%s/%s/check-suites/%d/check-runs", owner, repo, cs.ID),
		"head_commit": head,
	}
}

type checkRunRequest struct {
	Name        *string               `json:"name"`
	HeadSHA     string                `json:"head_sha"`
	DetailsURL  *string               `json:"details_url"`
	ExternalID  *string               `json:"external_id"`
	Status      *string               `json:"status"`
	Conclusion  *string               `json:"conclusion"`
	StartedAt   *string               `json:"started_at"`
	CompletedAt *string               `json:"completed_at"`
	Output      *store.CheckRunOutput `json:"output"`
}

// applyCheckRun applies a create or update request. A conclusion completes
// the run; a completed run needs a conclusion.
func (h *Handler) applyCheckRun(w http.ResponseWriter, cr *store.CheckRun, req checkRunRequest) bool {
	if req.Name != nil {
		cr.Name = *req.Name
	}
	if req.DetailsURL != nil {
		cr.DetailsURL = *req.DetailsURL
	}
	if req.ExternalID != nil {
		cr.ExternalID = *req.ExternalID
	}
	if req.Output != nil {
		cr.Output = req.Output
	}
	if req.StartedAt != nil {
		cr.StartedAt = *req.StartedAt
	}
	if req.Status != nil {
		if !checkStatuses[*req.Status] {
			ghValidationError(w, "CheckRun", "status", "invalid")
			return false
		}
		cr.Status = *req.Status
	}
	if req.Conclusion != nil {
		if !checkConclusions[*req.Conclusion] {
			ghValidationError(w, "CheckRun", "conclusion", "invalid")
			return false
		}
		cr.Conclusion = *req.Conclusion
		cr.Status = "completed"
	}
	if cr.Status == "completed" && cr.Conclusion == "" {
		ghValidationError(w, "CheckRun", "conclusion", "missing_field")
		return false
	}
	if cr.Status == "completed" {
		if req.CompletedAt != nil {
			cr.CompletedAt = *req.CompletedAt
		} else if cr.CompletedAt == "" {
			cr.CompletedAt = h.store.Now()
		}
	}
	return true
}

// CreateCheckRun handles POST /repos/{owner}/{repo}/check-runs
func (h *Handler) CreateCheckRun(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	if _, ok := h.store.GetRepo(owner, repo); !ok {
		ghError(w, 404, "Not Found")
		return
	}
	app, ok := h.checkApp(w, r)
	if !ok {
		return
	}
	var req checkRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if req.Name == nil || *req.Name == "" {
		ghValidationError(w, "CheckRun", "name", "missing_field")
		return
	}
	c, found := h.store.GetCommit(owner, repo, req.HeadSHA)
	if !found || len(req.HeadSHA) != 40 {
		ghValidationErrors(w, "No commit found for SHA: "+req.HeadSHA)
		return
	}
	cr := store.CheckRun{
		ID: h.store.NewID(store.KindCheckRun), HeadSHA: c.SHA, Status: "queued",
		StartedAt: h.store.Now(), AppID: app.ID, RepoOwner: owner, RepoName: repo,
	}
	if !h.applyCheckRun(w, &cr, req) {
		return
	}
	cr.SuiteID = h.suiteFor(owner, repo, c.SHA, app.ID).ID
	h.store.CheckRuns.Set(strconv.FormatInt(cr.ID, 10), cr)
	h.refreshSuite(cr.SuiteID)
	h.onCheckRun(r, "created", cr)
	if cr.Status == "completed" {
		h.onCheckRun(r, "completed", cr)
	}
	ghJSON(w, 201, h.rd(r).checkRun(cr))
}

func (h *Handler) checkRunFromPath(w http.ResponseWriter, r *http.Request) (store.CheckRun, bool) {
	cr, ok := h.store.CheckRuns.Get(param(r, "check_run_id"))
	if !ok || cr.RepoOwner != param(r, "owner") || cr.RepoName != param(r, "repo") {
		ghError(w, 404, "Not Found")
		return cr, false
	}
	return cr, true
}

// GetCheckRun handles GET /repos/{owner}/{repo}/check-runs/{check_run_id}
func (h *Handler) GetCheckRun(w http.ResponseWriter, r *http.Request) {
	if cr, ok := h.checkRunFromPath(w, r); ok {
		ghJSON(w, 200, h.rd(r).checkRun(cr))
	}
}

// UpdateCheckRun handles PATCH /repos/{owner}/{repo}/check-runs/{check_run_id}.
// Only the App that created the run may update it.
func (h *Handler) UpdateCheckRun(w http.ResponseWriter, r *http.Request) {
	cr, ok := h.checkRunFromPath(w, r)
	if !ok {
		return
	}
	app, ok := h.checkApp(w, r)
	if !ok {
		return
	}
	if app.ID != cr.AppID {
		ghError(w, 403, "Invalid app_id `"+strconv.FormatInt(app.ID, 10)+"` - check run can only be modified by the GitHub App that created it.")
		return
	}
	var req checkRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	wasCompleted := cr.Status == "completed"
	if !h.applyCheckRun(w, &cr, req) {
		return
	}
	h.store.CheckRuns.Set(strconv.FormatInt(cr.ID, 10), cr)
	h.refreshSuite(cr.SuiteID)
	if cr.Status == "completed" && !wasCompleted {
		h.onCheckRun(r, "completed", cr)
	}
	ghJSON(w, 200, h.rd(r).checkRun(cr))
}

// writeCheckRuns renders a filtered, paginated check-run list. filter=latest
// (the default) keeps the newest run per name.
func (h *Handler) writeCheckRuns(w http.ResponseWriter, r *http.Request, runs []store.CheckRun) {
	q := r.URL.Query()
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].ID > runs[j].ID })
	var kept []store.CheckRun
	seen := map[string]bool{}
	for _, cr := range runs {
		if n := q.Get("check_name"); n != "" && cr.Name != n {
			continue
		}
		if st := q.Get("status"); st != "" && cr.Status != st {
			continue
		}
		if q.Get("filter") != "all" {
			if seen[cr.Name] {
				continue
			}
			seen[cr.Name] = true
		}
		kept = append(kept, cr)
	}
	x := h.rd(r)
	out := []map[string]any{}
	for _, cr := range paginate(w, r, kept) {
		out = append(out, x.checkRun(cr))
	}
	ghJSON(w, 200, map[string]any{"total_count": len(kept), "check_runs": out})
}

// ListCheckRunsForRef handles GET /repos/{owner}/{repo}/commits/{ref}/check-runs
func (h *Handler) ListCheckRunsForRef(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	ref := param(r, "ref")
	c, ok := h.store.ResolveRef(owner, repo, ref)
	if !ok {
		ghValidationErrors(w, "No commit found for SHA: "+ref)
		return
	}
	h.writeCheckRuns(w, r, h.store.ListCheckRunsForRef(owner, repo, c.SHA))
}

// ListCheckRunsInSuite handles GET /repos/{owner}/{repo}/check-suites/{check_suite_id}/check-runs
func (h *Handler) ListCheckRunsInSuite(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(param(r, "check_suite_id"), 10, 64)
	h.writeCheckRuns(w, r, h.store.CheckRuns.Filter(func(_ string, cr store.CheckRun) bool { return cr.SuiteID == id }))
}

// ListCheckRunAnnotations handles GET /repos/{owner}/{repo}/check-runs/{check_run_id}/annotations
func (h *Handler) ListCheckRunAnnotations(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.checkRunFromPath(w, r); ok {
		ghJSON(w, 200, []any{})
	}
}

// RerequestCheckRun handles POST /repos/{owner}/{repo}/check-runs/{check_run_id}/rerequest
func (h *Handler) RerequestCheckRun(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.checkRunFromPath(w, r); ok {
		ghJSON(w, 201, map[string]any{})
	}
}

// CreateCheckSuite handles POST /repos/{owner}/{repo}/check-suites
func (h *Handler) CreateCheckSuite(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	app, ok := h.checkApp(w, r)
	if !ok {
		return
	}
	var req struct {
		HeadSHA string `json:"head_sha"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if _, found := h.store.GetCommit(owner, repo, req.HeadSHA); !found {
		ghValidationErrors(w, "No commit found for SHA: "+req.HeadSHA)
		return
	}
	existed := len(h.store.CheckSuites.Filter(func(_ string, cs store.CheckSuite) bool {
		return cs.RepoOwner == owner && cs.RepoName == repo && cs.HeadSHA == req.HeadSHA && cs.AppID == app.ID
	})) > 0
	cs := h.suiteFor(owner, repo, req.HeadSHA, app.ID)
	status := 201
	if existed {
		status = 200
	}
	ghJSON(w, status, h.rd(r).checkSuite(cs))
}

// GetCheckSuite handles GET /repos/{owner}/{repo}/check-suites/{check_suite_id}
func (h *Handler) GetCheckSuite(w http.ResponseWriter, r *http.Request) {
	cs, ok := h.store.CheckSuites.Get(param(r, "check_suite_id"))
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, h.rd(r).checkSuite(cs))
}

// RerequestCheckSuite handles POST /repos/{owner}/{repo}/check-suites/{check_suite_id}/rerequest
func (h *Handler) RerequestCheckSuite(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.store.CheckSuites.Get(param(r, "check_suite_id")); !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 201, map[string]any{})
}

// ListCheckSuitesForRef handles GET /repos/{owner}/{repo}/commits/{ref}/check-suites
func (h *Handler) ListCheckSuitesForRef(w http.ResponseWriter, r *http.Request) {
	owner := param(r, "owner")
	repo := param(r, "repo")
	ref := param(r, "ref")
	c, ok := h.store.ResolveRef(owner, repo, ref)
	if !ok {
		ghValidationErrors(w, "No commit found for SHA: "+ref)
		return
	}
	suites := h.store.CheckSuites.Filter(func(_ string, cs store.CheckSuite) bool {
		return cs.RepoOwner == owner && cs.RepoName == repo && cs.HeadSHA == c.SHA
	})
	x := h.rd(r)
	out := []map[string]any{}
	for _, cs := range paginate(w, r, suites) {
		out = append(out, x.checkSuite(cs))
	}
	ghJSON(w, 200, map[string]any{"total_count": len(suites), "check_suites": out})
}
