package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// labelNames accepts the forms GitHub takes for a label list: strings, or
// objects with a name.
func labelNames(raw []json.RawMessage) []string {
	var out []string
	for _, r := range raw {
		var s string
		if json.Unmarshal(r, &s) == nil {
			out = append(out, s)
			continue
		}
		var o struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(r, &o) == nil && o.Name != "" {
			out = append(out, o.Name)
		}
	}
	return out
}

// ensureLabel returns the repository label called name, creating it with
// GitHub's default colour if it does not exist yet, as GitHub does when an
// issue is labelled with a new name.
func (h *Handler) ensureLabel(owner, repo, name string) store.Label {
	if l, _, ok := h.store.GetLabel(owner, repo, name); ok {
		return *l
	}
	l := store.Label{ID: h.store.NewID(store.KindLabel), Name: name, Color: "ededed", RepoOwner: owner, RepoName: repo}
	h.store.Labels.Set(h.store.Labels.NextID(), l)
	return l
}

func (h *Handler) addLabels(owner, repo string, have []store.Label, names []string) []store.Label {
	for _, n := range names {
		dup := false
		for _, l := range have {
			if strings.EqualFold(l.Name, n) {
				dup = true
			}
		}
		if !dup {
			have = append(have, h.ensureLabel(owner, repo, n))
		}
	}
	return have
}

func (h *Handler) usersNamed(logins []string) []store.User {
	out := make([]store.User, 0, len(logins))
	for _, l := range logins {
		out = append(out, h.userRef(l))
	}
	return out
}

// ListIssues handles GET /repos/{owner}/{repo}/issues. GitHub lists pull
// requests here too, and sorts by creation time, newest first, by default.
func (h *Handler) ListIssues(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	if _, ok := h.store.GetRepo(owner, repo); !ok {
		ghError(w, 404, "Not Found")
		return
	}
	q := r.URL.Query()
	state := q.Get("state")
	if state == "" {
		state = "open"
	}
	x := h.rd(r)

	type row struct {
		created, updated string
		comments         int
		render           func() map[string]any
	}
	var rows []row
	wantLabels := splitList(q.Get("labels"))
	for _, i := range h.store.ListRepoIssues(owner, repo, state) {
		if !hasLabels(i.Labels, wantLabels) || (q.Get("creator") != "" && i.User.Login != q.Get("creator")) {
			continue
		}
		if q.Get("since") != "" && i.UpdatedAt < q.Get("since") {
			continue
		}
		i := i
		rows = append(rows, row{i.CreatedAt, i.UpdatedAt, i.Comments, func() map[string]any { return x.issue(i) }})
	}
	for _, p := range h.store.ListRepoPRs(owner, repo, state) {
		if !hasLabels(p.Labels, wantLabels) || (q.Get("creator") != "" && p.User.Login != q.Get("creator")) {
			continue
		}
		p := p
		rows = append(rows, row{p.CreatedAt, p.UpdatedAt, p.Comments, func() map[string]any { return x.pullAsIssue(p) }})
	}

	key := func(i int) string { return rows[i].created }
	switch q.Get("sort") {
	case "updated":
		key = func(i int) string { return rows[i].updated }
	case "comments":
		key = func(i int) string { return strconv.Itoa(1_000_000 + rows[i].comments) }
	}
	asc := q.Get("direction") == "asc"
	idx := make([]int, len(rows))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		ka, kb := key(idx[a]), key(idx[b])
		if ka == kb {
			return (idx[a] < idx[b]) == asc
		}
		return (ka < kb) == asc
	})
	out := []map[string]any{}
	for _, i := range paginate(w, r, idx) {
		out = append(out, rows[i].render())
	}
	ghJSON(w, 200, out)
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func hasLabels(have []store.Label, want []string) bool {
	for _, w := range want {
		found := false
		for _, l := range have {
			if strings.EqualFold(l.Name, w) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

type issueRequest struct {
	Title       *json.RawMessage  `json:"title"`
	Body        *string           `json:"body"`
	State       *string           `json:"state"`
	StateReason *string           `json:"state_reason"`
	Labels      []json.RawMessage `json:"labels"`
	Assignees   []string          `json:"assignees"`
	Milestone   *json.RawMessage  `json:"milestone"`
}

func rawTitle(raw *json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var s string
	if json.Unmarshal(*raw, &s) == nil {
		return s
	}
	var n json.Number
	if json.Unmarshal(*raw, &n) == nil {
		return n.String()
	}
	return ""
}

func (h *Handler) milestoneFor(owner, repo string, raw *json.RawMessage) (*store.Milestone, bool) {
	if raw == nil || string(*raw) == "null" {
		return nil, true
	}
	var n int
	if json.Unmarshal(*raw, &n) != nil {
		var s string
		if json.Unmarshal(*raw, &s) != nil {
			return nil, false
		}
		n, _ = strconv.Atoi(s)
	}
	for _, m := range h.store.ListRepoMilestones(owner, repo) {
		if m.Number == n {
			m := m
			return &m, true
		}
	}
	return nil, false
}

// CreateIssue handles POST /repos/{owner}/{repo}/issues. The 2026-03-10
// version has no singular assignee parameter; assignees is the only form.
func (h *Handler) CreateIssue(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	rp, ok := h.store.GetRepo(owner, repo)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	if !rp.HasIssues {
		ghError(w, 410, "Issues are disabled for this repo")
		return
	}

	var req issueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	title := rawTitle(req.Title)
	if title == "" {
		ghValidationErrors(w, "Invalid request.\n\n\"title\" wasn't supplied.")
		return
	}
	ms, ok := h.milestoneFor(owner, repo, req.Milestone)
	if !ok {
		ghValidationError(w, "Issue", "milestone", "invalid")
		return
	}

	now := h.store.Now()
	num := h.store.NextIssueNumber(owner, repo)
	issue := store.Issue{
		ID:        h.store.NewID(store.KindIssue),
		Number:    num,
		Title:     title,
		State:     "open",
		User:      h.userRef(actor(r)),
		Assignees: h.usersNamed(req.Assignees),
		Milestone: ms,
		CreatedAt: now,
		UpdatedAt: now,
		RepoOwner: owner,
		RepoName:  repo,
	}
	if req.Body != nil {
		issue.Body = *req.Body
	}
	issue.Labels = h.addLabels(owner, repo, nil, labelNames(req.Labels))

	h.store.Issues.Set(h.store.Issues.NextID(), issue)
	h.onIssue(r, "opened", issue)
	ghJSON(w, 201, h.rd(r).issue(issue))
}

// GetIssue handles GET /repos/{owner}/{repo}/issues/{issue_number}. A pull
// request number returns the pull request in its issue form.
func (h *Handler) GetIssue(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))

	if issue, _, ok := h.store.GetIssue(owner, repo, num); ok {
		ghJSON(w, 200, h.rd(r).issue(*issue))
		return
	}
	if pr, _, ok := h.store.GetPR(owner, repo, num); ok {
		ghJSON(w, 200, h.rd(r).pullAsIssue(*pr))
		return
	}
	ghError(w, 404, "Not Found")
}

// UpdateIssue handles PATCH /repos/{owner}/{repo}/issues/{issue_number}
func (h *Handler) UpdateIssue(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))

	issue, id, ok := h.store.GetIssue(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	var req issueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	var actions []string
	if req.Title != nil {
		issue.Title = rawTitle(req.Title)
		actions = append(actions, "edited")
	}
	if req.Body != nil {
		issue.Body = *req.Body
		actions = append(actions, "edited")
	}
	if req.State != nil && *req.State != issue.State {
		switch *req.State {
		case "closed":
			issue.State = "closed"
			issue.ClosedAt = h.store.Now()
			issue.ClosedBy = actor(r)
			issue.StateReason = "completed"
			actions = append(actions, "closed")
		case "open":
			issue.State = "open"
			issue.ClosedAt = ""
			issue.ClosedBy = ""
			issue.StateReason = "reopened"
			actions = append(actions, "reopened")
		default:
			ghValidationError(w, "Issue", "state", "invalid")
			return
		}
	}
	if req.StateReason != nil {
		switch *req.StateReason {
		case "completed", "not_planned", "reopened", "duplicate":
			issue.StateReason = *req.StateReason
		default:
			ghValidationError(w, "Issue", "state_reason", "invalid")
			return
		}
	}
	if req.Labels != nil {
		issue.Labels = h.addLabels(owner, repo, nil, labelNames(req.Labels))
	}
	if req.Assignees != nil {
		issue.Assignees = h.usersNamed(req.Assignees)
	}
	if req.Milestone != nil {
		ms, found := h.milestoneFor(owner, repo, req.Milestone)
		if !found {
			ghValidationError(w, "Issue", "milestone", "invalid")
			return
		}
		issue.Milestone = ms
	}
	issue.UpdatedAt = h.store.Now()

	h.store.Issues.Set(id, *issue)
	for _, a := range dedupe(actions) {
		h.onIssue(r, a, *issue)
	}
	ghJSON(w, 200, h.rd(r).issue(*issue))
}

func dedupe(s []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// ListIssueComments handles GET /repos/{owner}/{repo}/issues/{issue_number}/comments
func (h *Handler) ListIssueComments(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))

	if !h.issueOrPRExists(owner, repo, num) {
		ghError(w, 404, "Not Found")
		return
	}
	x := h.rd(r)
	out := []map[string]any{}
	for _, c := range paginate(w, r, h.store.ListIssueComments(owner, repo, num)) {
		out = append(out, x.comment(c))
	}
	ghJSON(w, 200, out)
}

func (h *Handler) issueOrPRExists(owner, repo string, num int) bool {
	if _, _, ok := h.store.GetIssue(owner, repo, num); ok {
		return true
	}
	_, _, ok := h.store.GetPR(owner, repo, num)
	return ok
}

// CreateIssueComment handles POST /repos/{owner}/{repo}/issues/{issue_number}/comments
func (h *Handler) CreateIssueComment(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))

	if !h.issueOrPRExists(owner, repo, num) {
		ghError(w, 404, "Not Found")
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if req.Body == "" {
		ghValidationError(w, "IssueComment", "body", "missing_field")
		return
	}

	now := h.store.Now()
	comment := store.Comment{
		ID:          h.store.NewID(store.KindComment),
		Body:        req.Body,
		User:        h.userRef(actor(r)),
		CreatedAt:   now,
		UpdatedAt:   now,
		RepoOwner:   owner,
		RepoName:    repo,
		IssueNumber: num,
	}
	h.store.Comments.Set(h.store.Comments.NextID(), comment)

	if iss, issID, ok := h.store.GetIssue(owner, repo, num); ok {
		iss.Comments++
		iss.UpdatedAt = now
		h.store.Issues.Set(issID, *iss)
	} else if pr, prID, ok := h.store.GetPR(owner, repo, num); ok {
		pr.Comments++
		pr.UpdatedAt = now
		h.store.PullRequests.Set(prID, *pr)
	}
	ghJSON(w, 201, h.rd(r).comment(comment))
}

func (h *Handler) findComment(id int64) (store.Comment, string, bool) {
	ids, comments := h.store.Comments.FilterWithIDs(func(_ string, c store.Comment) bool { return c.ID == id })
	if len(ids) == 0 {
		return store.Comment{}, "", false
	}
	return comments[0], ids[0], true
}

// GetIssueComment handles GET /repos/{owner}/{repo}/issues/comments/{comment_id}
func (h *Handler) GetIssueComment(w http.ResponseWriter, r *http.Request) {
	commentID, _ := strconv.ParseInt(chi.URLParam(r, "comment_id"), 10, 64)
	c, _, ok := h.findComment(commentID)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, h.rd(r).comment(c))
}

// UpdateIssueComment handles PATCH /repos/{owner}/{repo}/issues/comments/{comment_id}
func (h *Handler) UpdateIssueComment(w http.ResponseWriter, r *http.Request) {
	commentID, _ := strconv.ParseInt(chi.URLParam(r, "comment_id"), 10, 64)
	c, id, ok := h.findComment(commentID)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Body == "" {
		ghValidationError(w, "IssueComment", "body", "missing_field")
		return
	}
	c.Body = req.Body
	c.UpdatedAt = h.store.Now()
	h.store.Comments.Set(id, c)
	ghJSON(w, 200, h.rd(r).comment(c))
}

// DeleteIssueComment handles DELETE /repos/{owner}/{repo}/issues/comments/{comment_id}
func (h *Handler) DeleteIssueComment(w http.ResponseWriter, r *http.Request) {
	commentID, _ := strconv.ParseInt(chi.URLParam(r, "comment_id"), 10, 64)
	c, id, ok := h.findComment(commentID)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.Comments.Delete(id)
	if iss, issID, found := h.store.GetIssue(c.RepoOwner, c.RepoName, c.IssueNumber); found && iss.Comments > 0 {
		iss.Comments--
		h.store.Issues.Set(issID, *iss)
	}
	w.WriteHeader(204)
}

// ListLabels handles GET /repos/{owner}/{repo}/labels
func (h *Handler) ListLabels(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	if _, ok := h.store.GetRepo(owner, repo); !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, h.rd(r).labels(owner, repo, paginate(w, r, h.store.ListRepoLabels(owner, repo))))
}

// CreateLabel handles POST /repos/{owner}/{repo}/labels
func (h *Handler) CreateLabel(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	if _, ok := h.store.GetRepo(owner, repo); !ok {
		ghError(w, 404, "Not Found")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if req.Name == "" {
		ghValidationError(w, "Label", "name", "missing_field")
		return
	}
	if _, _, exists := h.store.GetLabel(owner, repo, req.Name); exists {
		ghValidationError(w, "Label", "name", "already_exists")
		return
	}
	color := strings.TrimPrefix(req.Color, "#")
	if color == "" {
		color = "ededed"
	}

	label := store.Label{
		ID:          h.store.NewID(store.KindLabel),
		Name:        req.Name,
		Color:       color,
		Description: req.Description,
		RepoOwner:   owner,
		RepoName:    repo,
	}
	h.store.Labels.Set(h.store.Labels.NextID(), label)
	ghJSON(w, 201, h.rd(r).label(owner, repo, label))
}

// GetLabel handles GET /repos/{owner}/{repo}/labels/{name}
func (h *Handler) GetLabel(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	label, _, ok := h.store.GetLabel(owner, repo, chi.URLParam(r, "name"))
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, h.rd(r).label(owner, repo, *label))
}

// UpdateLabel handles PATCH /repos/{owner}/{repo}/labels/{name}
func (h *Handler) UpdateLabel(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	label, id, ok := h.store.GetLabel(owner, repo, chi.URLParam(r, "name"))
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	if newName, ok := req["new_name"].(string); ok && newName != "" {
		label.Name = newName
	}
	if color, ok := req["color"].(string); ok {
		label.Color = strings.TrimPrefix(color, "#")
	}
	if desc, ok := req["description"].(string); ok {
		label.Description = desc
	}
	h.store.Labels.Set(id, *label)
	ghJSON(w, 200, h.rd(r).label(owner, repo, *label))
}

// DeleteLabel handles DELETE /repos/{owner}/{repo}/labels/{name}
func (h *Handler) DeleteLabel(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	_, id, ok := h.store.GetLabel(owner, repo, chi.URLParam(r, "name"))
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.Labels.Delete(id)
	w.WriteHeader(204)
}

// issueLabels reads and writes the labels of an issue or a pull request.
func (h *Handler) issueLabels(owner, repo string, num int) ([]store.Label, func([]store.Label), bool) {
	if issue, id, ok := h.store.GetIssue(owner, repo, num); ok {
		return issue.Labels, func(ls []store.Label) {
			issue.Labels = ls
			issue.UpdatedAt = h.store.Now()
			h.store.Issues.Set(id, *issue)
		}, true
	}
	if pr, id, ok := h.store.GetPR(owner, repo, num); ok {
		return pr.Labels, func(ls []store.Label) {
			pr.Labels = ls
			pr.UpdatedAt = h.store.Now()
			h.store.PullRequests.Set(id, *pr)
		}, true
	}
	return nil, nil, false
}

// decodeLabelBody accepts {"labels": [...]} or a bare array.
func decodeLabelBody(r *http.Request) []string {
	var raw json.RawMessage
	if json.NewDecoder(r.Body).Decode(&raw) != nil {
		return nil
	}
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		return labelNames(arr)
	}
	var obj struct {
		Labels []json.RawMessage `json:"labels"`
	}
	_ = json.Unmarshal(raw, &obj)
	return labelNames(obj.Labels)
}

// ListIssueLabels handles GET /repos/{owner}/{repo}/issues/{issue_number}/labels
func (h *Handler) ListIssueLabels(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))
	labels, _, ok := h.issueLabels(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, h.rd(r).labels(owner, repo, paginate(w, r, labels)))
}

// AddIssueLabels handles POST /repos/{owner}/{repo}/issues/{issue_number}/labels
func (h *Handler) AddIssueLabels(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))
	labels, set, ok := h.issueLabels(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	labels = h.addLabels(owner, repo, labels, decodeLabelBody(r))
	set(labels)
	if issue, _, found := h.store.GetIssue(owner, repo, num); found {
		h.onIssue(r, "labeled", *issue)
	}
	ghJSON(w, 200, h.rd(r).labels(owner, repo, labels))
}

// SetIssueLabels handles PUT /repos/{owner}/{repo}/issues/{issue_number}/labels
func (h *Handler) SetIssueLabels(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))
	_, set, ok := h.issueLabels(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	labels := h.addLabels(owner, repo, nil, decodeLabelBody(r))
	set(labels)
	ghJSON(w, 200, h.rd(r).labels(owner, repo, labels))
}

// RemoveAllIssueLabels handles DELETE /repos/{owner}/{repo}/issues/{issue_number}/labels
func (h *Handler) RemoveAllIssueLabels(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))
	_, set, ok := h.issueLabels(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	set(nil)
	w.WriteHeader(204)
}

// RemoveIssueLabel handles DELETE /repos/{owner}/{repo}/issues/{issue_number}/labels/{name}
func (h *Handler) RemoveIssueLabel(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))
	name := chi.URLParam(r, "name")

	labels, set, ok := h.issueLabels(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	kept := make([]store.Label, 0, len(labels))
	found := false
	for _, l := range labels {
		if strings.EqualFold(l.Name, name) {
			found = true
			continue
		}
		kept = append(kept, l)
	}
	if !found {
		ghError(w, 404, "Label does not exist")
		return
	}
	set(kept)
	ghJSON(w, 200, h.rd(r).labels(owner, repo, kept))
}
