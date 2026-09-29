package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// ghErrorList writes an error whose errors member is a list of strings, the
// shape GitHub uses for review and merge refusals.
func ghErrorList(w http.ResponseWriter, status int, message string, errs ...string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"message": message, "errors": errs, "documentation_url": docsURL, "status": strconv.Itoa(status),
	})
}

func (h *Handler) pullFromPath(w http.ResponseWriter, r *http.Request) (*store.PullRequest, string, bool) {
	num, _ := strconv.Atoi(chi.URLParam(r, "pull_number"))
	pr, id, ok := h.store.GetPR(chi.URLParam(r, "owner"), chi.URLParam(r, "repo"), num)
	if !ok {
		ghError(w, 404, "Not Found")
	}
	return pr, id, ok
}

// mergeBase is the newest commit of head's history that base also reaches.
func (h *Handler) mergeBase(owner, repo, base, head string) string {
	for _, c := range h.store.History(owner, repo, head, "") {
		if h.store.IsAncestor(owner, repo, c.SHA, base) {
			return c.SHA
		}
	}
	return ""
}

// ListPullRequests handles GET /repos/{owner}/{repo}/pulls
func (h *Handler) ListPullRequests(w http.ResponseWriter, r *http.Request) {
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
	var prs []store.PullRequest
	for _, p := range h.store.ListRepoPRs(owner, repo, state) {
		if hd := q.Get("head"); hd != "" && hd != p.Head.Label && hd != p.Head.Ref {
			continue
		}
		if b := q.Get("base"); b != "" && b != p.Base.Ref {
			continue
		}
		prs = append(prs, p)
	}
	asc := q.Get("direction") == "asc"
	sortKey := func(p store.PullRequest) string { return p.CreatedAt }
	if q.Get("sort") == "updated" {
		sortKey = func(p store.PullRequest) string { return p.UpdatedAt }
	}
	sort.SliceStable(prs, func(i, j int) bool {
		a, b := sortKey(prs[i]), sortKey(prs[j])
		if a == b {
			return (prs[i].Number < prs[j].Number) == asc
		}
		return (a < b) == asc
	})
	x := h.rd(r)
	out := []map[string]any{}
	for _, p := range paginate(w, r, prs) {
		out = append(out, x.pullSimple(p))
	}
	ghJSON(w, 200, out)
}

// CreatePullRequest handles POST /repos/{owner}/{repo}/pulls. Head and base
// must be branches of the repository, head must have commits base lacks,
// and only one open pull request may exist per head and base.
func (h *Handler) CreatePullRequest(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	if _, ok := h.store.GetRepo(owner, repo); !ok {
		ghError(w, 404, "Not Found")
		return
	}

	var req struct {
		Title               string `json:"title"`
		Body                string `json:"body"`
		Head                string `json:"head"`
		Base                string `json:"base"`
		Draft               bool   `json:"draft"`
		MaintainerCanModify *bool  `json:"maintainer_can_modify"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if req.Title == "" {
		ghValidationError(w, "PullRequest", "title", "missing_field")
		return
	}
	headRef := req.Head
	if i := strings.IndexByte(headRef, ':'); i >= 0 {
		headRef = headRef[i+1:]
	}
	head, ok := h.store.GetBranch(owner, repo, headRef)
	if !ok || req.Head == "" {
		ghValidationError(w, "PullRequest", "head", "invalid")
		return
	}
	base, ok := h.store.GetBranch(owner, repo, req.Base)
	if !ok {
		ghValidationError(w, "PullRequest", "base", "invalid")
		return
	}
	if h.store.IsAncestor(owner, repo, head.Commit.SHA, base.Commit.SHA) {
		ghValidationErrors(w, "Validation Failed", map[string]any{
			"resource": "PullRequest", "code": "custom", "message": fmt.Sprintf("No commits between %s and %s", req.Base, headRef),
		})
		return
	}
	for _, p := range h.store.ListRepoPRs(owner, repo, "open") {
		if p.Head.Ref == headRef && p.Base.Ref == req.Base {
			ghValidationErrors(w, "Validation Failed", map[string]any{
				"resource": "PullRequest", "code": "custom", "message": fmt.Sprintf("A pull request already exists for %s:%s.", owner, headRef),
			})
			return
		}
	}

	now := h.store.Now()
	num := h.store.NextIssueNumber(owner, repo)
	pr := store.PullRequest{
		ID:                  h.store.NewID(store.KindPull),
		Number:              num,
		Title:               req.Title,
		Body:                req.Body,
		State:               "open",
		Draft:               req.Draft,
		User:                h.userRef(actor(r)),
		Head:                store.PRRef{Label: owner + ":" + headRef, Ref: headRef, SHA: head.Commit.SHA},
		Base:                store.PRRef{Label: owner + ":" + req.Base, Ref: req.Base, SHA: base.Commit.SHA},
		MergeBase:           h.mergeBase(owner, repo, base.Commit.SHA, head.Commit.SHA),
		MaintainerCanModify: boolOr(req.MaintainerCanModify, true),
		CreatedAt:           now,
		UpdatedAt:           now,
		RepoOwner:           owner,
		RepoName:            repo,
	}
	h.store.PullRequests.Set(h.store.PullRequests.NextID(), pr)
	h.onPull(r, "opened", pr)
	ghJSON(w, 201, h.rd(r).pull(pr))
}

// GetPullRequest handles GET /repos/{owner}/{repo}/pulls/{pull_number}
func (h *Handler) GetPullRequest(w http.ResponseWriter, r *http.Request) {
	pr, _, ok := h.pullFromPath(w, r)
	if !ok {
		return
	}
	ghJSON(w, 200, h.rd(r).pull(*pr))
}

// UpdatePullRequest handles PATCH /repos/{owner}/{repo}/pulls/{pull_number}
func (h *Handler) UpdatePullRequest(w http.ResponseWriter, r *http.Request) {
	pr, id, ok := h.pullFromPath(w, r)
	if !ok {
		return
	}
	var req struct {
		Title               *string `json:"title"`
		Body                *string `json:"body"`
		State               *string `json:"state"`
		Base                *string `json:"base"`
		MaintainerCanModify *bool   `json:"maintainer_can_modify"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	action := "edited"
	if req.Title != nil {
		pr.Title = *req.Title
	}
	if req.Body != nil {
		pr.Body = *req.Body
	}
	if req.MaintainerCanModify != nil {
		pr.MaintainerCanModify = *req.MaintainerCanModify
	}
	if req.Base != nil {
		b, found := h.store.GetBranch(pr.RepoOwner, pr.RepoName, *req.Base)
		if !found {
			ghValidationError(w, "PullRequest", "base", "invalid")
			return
		}
		pr.Base = store.PRRef{Label: pr.RepoOwner + ":" + *req.Base, Ref: *req.Base, SHA: b.Commit.SHA}
		pr.MergeBase = h.mergeBase(pr.RepoOwner, pr.RepoName, b.Commit.SHA, h.prState(*pr).headSHA)
	}
	if req.State != nil && *req.State != pr.State && !pr.Merged {
		switch *req.State {
		case "closed":
			st := h.prState(*pr)
			pr.Head.SHA, pr.Base.SHA = st.headSHA, st.baseSHA
			pr.State, pr.ClosedAt, action = "closed", h.store.Now(), "closed"
		case "open":
			pr.State, pr.ClosedAt, action = "open", "", "reopened"
		default:
			ghValidationError(w, "PullRequest", "state", "invalid")
			return
		}
	}
	pr.UpdatedAt = h.store.Now()
	h.store.PullRequests.Set(id, *pr)
	h.onPull(r, action, *pr)
	ghJSON(w, 200, h.rd(r).pull(*pr))
}

// applyChanges overlays the changes from `from` to `to` onto files.
func applyChanges(files, from, to map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range files {
		out[k] = v
	}
	for p, sha := range to {
		if from[p] != sha {
			out[p] = sha
		}
	}
	for p := range from {
		if _, kept := to[p]; !kept {
			delete(out, p)
		}
	}
	return out
}

// MergePullRequest handles PUT /repos/{owner}/{repo}/pulls/{pull_number}/merge.
// It writes the merge, squash or rebase commit(s) onto the base branch.
func (h *Handler) MergePullRequest(w http.ResponseWriter, r *http.Request) {
	pr, id, ok := h.pullFromPath(w, r)
	if !ok {
		return
	}
	owner, repo := pr.RepoOwner, pr.RepoName
	var req struct {
		CommitTitle   string `json:"commit_title"`
		CommitMessage string `json:"commit_message"`
		SHA           string `json:"sha"`
		MergeMethod   string `json:"merge_method"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.MergeMethod == "" {
		req.MergeMethod = "merge"
	}

	rp, _ := h.store.GetRepo(owner, repo)
	allowed := map[string]bool{
		"merge":  boolOr(rp.AllowMergeCommit, true),
		"squash": boolOr(rp.AllowSquashMerge, true),
		"rebase": boolOr(rp.AllowRebaseMerge, true),
	}
	if a, known := allowed[req.MergeMethod]; !known {
		ghValidationError(w, "PullRequest", "merge_method", "invalid")
		return
	} else if !a {
		ghError(w, 405, map[string]string{
			"merge": "Merge commits are not allowed on this repository.", "squash": "Squash merges are not allowed on this repository.",
			"rebase": "Rebase merges are not allowed on this repository.",
		}[req.MergeMethod])
		return
	}
	st := h.prState(*pr)
	if pr.State != "open" || pr.Draft {
		ghError(w, 405, "Pull Request is not mergeable")
		return
	}
	if req.SHA != "" && req.SHA != st.headSHA {
		ghError(w, 409, "Head branch was modified. Review and try the merge again.")
		return
	}
	if st.conflicts {
		ghError(w, 405, "Merge conflict")
		return
	}

	base, _ := h.store.GetCommit(owner, repo, st.baseSHA)
	head, _ := h.store.GetCommit(owner, repo, st.headSHA)
	fork, _ := h.store.GetCommit(owner, repo, pr.MergeBase)
	sig := h.signature(r, contentsRequest{})
	commit := func(msg string, parents []string, files map[string]string, author store.Signature) store.Commit {
		return h.store.PutCommit(store.Commit{
			Message: msg, Parents: parents, Files: files,
			AuthorName: author.Name, AuthorEmail: author.Email, AuthorDate: author.Date, Login: author.Login,
			CommitterDate: h.store.Now(), RepoOwner: owner, RepoName: repo,
		})
	}

	var result store.Commit
	switch req.MergeMethod {
	case "merge":
		title := req.CommitTitle
		if title == "" {
			title = fmt.Sprintf("Merge pull request #%d from %s/%s", pr.Number, owner, pr.Head.Ref)
		}
		msg := title + "\n\n" + pr.Title
		if req.CommitMessage != "" {
			msg = title + "\n\n" + req.CommitMessage
		}
		result = commit(msg, []string{base.SHA, head.SHA}, applyChanges(base.Files, fork.Files, head.Files), sig)
	case "squash":
		title := req.CommitTitle
		if title == "" {
			title = fmt.Sprintf("%s (#%d)", pr.Title, pr.Number)
		}
		msg := req.CommitMessage
		if msg == "" {
			var lines []string
			for i := len(st.commits) - 1; i >= 0; i-- {
				lines = append(lines, "* "+st.commits[i].Message)
			}
			msg = strings.Join(lines, "\n\n")
		}
		result = commit(title+"\n\n"+msg, []string{base.SHA}, applyChanges(base.Files, fork.Files, head.Files), sig)
	case "rebase":
		tip := base
		for i := len(st.commits) - 1; i >= 0; i-- {
			c := st.commits[i]
			parent, _ := h.store.GetCommit(owner, repo, firstParent(c))
			author := store.Signature{Name: c.AuthorName, Email: c.AuthorEmail, Date: c.AuthorDate, Login: c.Login}
			tip = commit(c.Message, []string{tip.SHA}, applyChanges(tip.Files, parent.Files, c.Files), author)
		}
		result = tip
	}

	h.store.SetBranch(owner, repo, pr.Base.Ref, result.SHA)
	h.touchRepo(owner, repo)
	now := h.store.Now()
	pr.Merged, pr.State = true, "closed"
	pr.MergeCommitSHA, pr.MergedAt, pr.ClosedAt, pr.UpdatedAt = result.SHA, now, now, now
	pr.MergedBy = actor(r)
	pr.Head.SHA, pr.Base.SHA = st.headSHA, st.baseSHA
	h.store.PullRequests.Set(id, *pr)
	if rp.DeleteBranchOnMerge {
		h.store.DeleteBranch(owner, repo, pr.Head.Ref)
	}
	h.onRefUpdated(r, owner, repo, "refs/heads/"+pr.Base.Ref, st.baseSHA, result.SHA, false)
	h.onPull(r, "closed", *pr)

	ghJSON(w, 200, map[string]any{"sha": result.SHA, "merged": true, "message": "Pull Request successfully merged"})
}

func firstParent(c store.Commit) string {
	if len(c.Parents) == 0 {
		return ""
	}
	return c.Parents[0]
}

// CheckPRMerged handles GET /repos/{owner}/{repo}/pulls/{pull_number}/merge
func (h *Handler) CheckPRMerged(w http.ResponseWriter, r *http.Request) {
	pr, _, ok := h.pullFromPath(w, r)
	if !ok {
		return
	}
	if !pr.Merged {
		ghError(w, 404, "Not Found")
		return
	}
	w.WriteHeader(204)
}

// ListPRCommits handles GET /repos/{owner}/{repo}/pulls/{pull_number}/commits,
// oldest first.
func (h *Handler) ListPRCommits(w http.ResponseWriter, r *http.Request) {
	pr, _, ok := h.pullFromPath(w, r)
	if !ok {
		return
	}
	commits := h.prState(*pr).commits
	for i, j := 0, len(commits)-1; i < j; i, j = i+1, j-1 {
		commits[i], commits[j] = commits[j], commits[i]
	}
	x := h.rd(r)
	out := []map[string]any{}
	for _, c := range paginate(w, r, commits) {
		out = append(out, x.commit(c))
	}
	ghJSON(w, 200, out)
}

// ListPRFiles handles GET /repos/{owner}/{repo}/pulls/{pull_number}/files
func (h *Handler) ListPRFiles(w http.ResponseWriter, r *http.Request) {
	pr, _, ok := h.pullFromPath(w, r)
	if !ok {
		return
	}
	owner, repo := pr.RepoOwner, pr.RepoName
	st := h.prState(*pr)
	x := h.rd(r)
	out := []map[string]any{}
	for _, c := range paginate(w, r, st.changes) {
		sha := c.NewSHA
		if sha == "" {
			sha = c.OldSHA
		}
		out = append(out, map[string]any{
			"sha": sha, "filename": c.Path, "status": c.Status,
			"additions": c.Additions, "deletions": c.Deletions, "changes": c.Additions + c.Deletions,
			"blob_url":     x.web("/%s/%s/blob/%s/%s", owner, repo, st.headSHA, c.Path),
			"raw_url":      x.web("/%s/%s/raw/%s/%s", owner, repo, st.headSHA, c.Path),
			"contents_url": x.api("/repos/%s/%s/contents/%s?ref=%s", owner, repo, c.Path, st.headSHA),
		})
	}
	ghJSON(w, 200, out)
}

// ListRequestedReviewers handles GET /repos/{owner}/{repo}/pulls/{pull_number}/requested_reviewers
func (h *Handler) ListRequestedReviewers(w http.ResponseWriter, r *http.Request) {
	pr, _, ok := h.pullFromPath(w, r)
	if !ok {
		return
	}
	x := h.rd(r)
	users := []map[string]any{}
	for _, l := range pr.RequestedReviewers {
		users = append(users, x.user(l))
	}
	ghJSON(w, 200, map[string]any{"users": users, "teams": []any{}})
}

type reviewersRequest struct {
	Reviewers []string `json:"reviewers"`
}

// RequestReviewers handles POST /repos/{owner}/{repo}/pulls/{pull_number}/requested_reviewers
func (h *Handler) RequestReviewers(w http.ResponseWriter, r *http.Request) {
	pr, id, ok := h.pullFromPath(w, r)
	if !ok {
		return
	}
	var req reviewersRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	for _, l := range req.Reviewers {
		if l == pr.User.Login {
			ghValidationErrors(w, "Review cannot be requested from pull request author.")
			return
		}
	}
	pr.RequestedReviewers = dedupe(append(pr.RequestedReviewers, req.Reviewers...))
	h.store.PullRequests.Set(id, *pr)
	ghJSON(w, 201, h.rd(r).pullSimple(*pr))
}

// RemoveRequestedReviewers handles DELETE /repos/{owner}/{repo}/pulls/{pull_number}/requested_reviewers
func (h *Handler) RemoveRequestedReviewers(w http.ResponseWriter, r *http.Request) {
	pr, id, ok := h.pullFromPath(w, r)
	if !ok {
		return
	}
	var req reviewersRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	drop := map[string]bool{}
	for _, l := range req.Reviewers {
		drop[l] = true
	}
	var kept []string
	for _, l := range pr.RequestedReviewers {
		if !drop[l] {
			kept = append(kept, l)
		}
	}
	pr.RequestedReviewers = kept
	h.store.PullRequests.Set(id, *pr)
	ghJSON(w, 200, h.rd(r).pullSimple(*pr))
}

// UpdatePRBranch handles PUT /repos/{owner}/{repo}/pulls/{pull_number}/update-branch
// by merging the base branch into the head branch.
func (h *Handler) UpdatePRBranch(w http.ResponseWriter, r *http.Request) {
	pr, id, ok := h.pullFromPath(w, r)
	if !ok {
		return
	}
	owner, repo := pr.RepoOwner, pr.RepoName
	st := h.prState(*pr)
	if h.store.IsAncestor(owner, repo, st.baseSHA, st.headSHA) {
		ghValidationErrors(w, "There are no new commits on the base branch.")
		return
	}
	if st.conflicts {
		ghValidationErrors(w, "merge conflict between base and head")
		return
	}
	base, _ := h.store.GetCommit(owner, repo, st.baseSHA)
	head, _ := h.store.GetCommit(owner, repo, st.headSHA)
	fork, _ := h.store.GetCommit(owner, repo, pr.MergeBase)
	sig := h.signature(r, contentsRequest{})
	c := h.store.PutCommit(store.Commit{
		Message:    fmt.Sprintf("Merge branch '%s' into %s", pr.Base.Ref, pr.Head.Ref),
		Parents:    []string{head.SHA, base.SHA},
		Files:      applyChanges(head.Files, fork.Files, base.Files),
		AuthorName: sig.Name, AuthorEmail: sig.Email, AuthorDate: sig.Date, Login: sig.Login,
		RepoOwner: owner, RepoName: repo,
	})
	h.store.SetBranch(owner, repo, pr.Head.Ref, c.SHA)
	pr.MergeBase = base.SHA
	pr.UpdatedAt = h.store.Now()
	h.store.PullRequests.Set(id, *pr)
	h.onRefUpdated(r, owner, repo, "refs/heads/"+pr.Head.Ref, head.SHA, c.SHA, false)
	ghJSON(w, 202, map[string]any{"message": "Updating pull request branch.", "url": h.rd(r).web("/%s/%s/pull/%d", owner, repo, pr.Number)})
}

// --- Reviews ---

// ListPRReviews handles GET /repos/{owner}/{repo}/pulls/{pull_number}/reviews
func (h *Handler) ListPRReviews(w http.ResponseWriter, r *http.Request) {
	pr, _, ok := h.pullFromPath(w, r)
	if !ok {
		return
	}
	x := h.rd(r)
	out := []map[string]any{}
	for _, rv := range paginate(w, r, h.store.ListPRReviews(pr.RepoOwner, pr.RepoName, pr.Number)) {
		out = append(out, x.review(rv))
	}
	ghJSON(w, 200, out)
}

func reviewState(event string) (string, bool) {
	switch event {
	case "APPROVE":
		return "APPROVED", true
	case "REQUEST_CHANGES":
		return "CHANGES_REQUESTED", true
	case "COMMENT":
		return "COMMENTED", true
	case "":
		return "PENDING", true
	}
	return "", false
}

// ownReviewRefusal is GitHub's refusal when the author reviews their own
// pull request with an approval or a change request.
func ownReviewRefusal(state string) string {
	switch state {
	case "APPROVED":
		return "Can not approve your own pull request"
	case "CHANGES_REQUESTED":
		return "Can not request changes on your own pull request"
	}
	return ""
}

// CreatePRReview handles POST /repos/{owner}/{repo}/pulls/{pull_number}/reviews
func (h *Handler) CreatePRReview(w http.ResponseWriter, r *http.Request) {
	pr, _, ok := h.pullFromPath(w, r)
	if !ok {
		return
	}
	var req struct {
		Body     string `json:"body"`
		Event    string `json:"event"`
		CommitID string `json:"commit_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	state, valid := reviewState(req.Event)
	if !valid {
		ghValidationError(w, "PullRequestReview", "event", "invalid")
		return
	}
	who := actor(r)
	if msg := ownReviewRefusal(state); msg != "" && who == pr.User.Login {
		ghErrorList(w, 422, "Unprocessable Entity", msg)
		return
	}
	commitID := req.CommitID
	if commitID == "" {
		commitID = h.prState(*pr).headSHA
	}
	review := store.PRReview{
		ID:        h.store.NewID(store.KindReview),
		User:      h.userRef(who),
		Body:      req.Body,
		State:     state,
		CommitID:  commitID,
		RepoOwner: pr.RepoOwner,
		RepoName:  pr.RepoName,
		PRNumber:  pr.Number,
	}
	if state != "PENDING" {
		review.SubmittedAt = h.store.Now()
	}
	h.store.PRReviews.Set(h.store.PRReviews.NextID(), review)
	ghJSON(w, 200, h.rd(r).review(review))
}

func (h *Handler) reviewFromPath(w http.ResponseWriter, r *http.Request) (store.PRReview, string, bool) {
	reviewID, _ := strconv.ParseInt(chi.URLParam(r, "review_id"), 10, 64)
	num, _ := strconv.Atoi(chi.URLParam(r, "pull_number"))
	ids, reviews := h.store.PRReviews.FilterWithIDs(func(_ string, rv store.PRReview) bool {
		return rv.ID == reviewID && rv.PRNumber == num && rv.RepoOwner == chi.URLParam(r, "owner") && rv.RepoName == chi.URLParam(r, "repo")
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return store.PRReview{}, "", false
	}
	return reviews[0], ids[0], true
}

// GetPRReview handles GET /repos/{owner}/{repo}/pulls/{pull_number}/reviews/{review_id}
func (h *Handler) GetPRReview(w http.ResponseWriter, r *http.Request) {
	rv, _, ok := h.reviewFromPath(w, r)
	if !ok {
		return
	}
	ghJSON(w, 200, h.rd(r).review(rv))
}

// UpdatePRReview handles PUT /repos/{owner}/{repo}/pulls/{pull_number}/reviews/{review_id},
// which changes a review's body.
func (h *Handler) UpdatePRReview(w http.ResponseWriter, r *http.Request) {
	rv, id, ok := h.reviewFromPath(w, r)
	if !ok {
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Body == "" {
		ghValidationError(w, "PullRequestReview", "body", "missing_field")
		return
	}
	rv.Body = req.Body
	h.store.PRReviews.Set(id, rv)
	ghJSON(w, 200, h.rd(r).review(rv))
}

// SubmitPRReview handles POST /repos/{owner}/{repo}/pulls/{pull_number}/reviews/{review_id}/events
func (h *Handler) SubmitPRReview(w http.ResponseWriter, r *http.Request) {
	rv, id, ok := h.reviewFromPath(w, r)
	if !ok {
		return
	}
	var req struct {
		Body  string `json:"body"`
		Event string `json:"event"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	state, valid := reviewState(req.Event)
	if !valid || state == "PENDING" {
		ghValidationError(w, "PullRequestReview", "event", "invalid")
		return
	}
	if rv.State != "PENDING" {
		ghErrorList(w, 422, "Unprocessable Entity", "Can not submit a review that is not pending")
		return
	}
	if pr, _, found := h.store.GetPR(rv.RepoOwner, rv.RepoName, rv.PRNumber); found {
		if msg := ownReviewRefusal(state); msg != "" && rv.User.Login == pr.User.Login {
			ghErrorList(w, 422, "Unprocessable Entity", msg)
			return
		}
	}
	rv.State = state
	if req.Body != "" {
		rv.Body = req.Body
	}
	rv.SubmittedAt = h.store.Now()
	h.store.PRReviews.Set(id, rv)
	ghJSON(w, 200, h.rd(r).review(rv))
}

// DismissPRReview handles PUT /repos/{owner}/{repo}/pulls/{pull_number}/reviews/{review_id}/dismissals
func (h *Handler) DismissPRReview(w http.ResponseWriter, r *http.Request) {
	rv, id, ok := h.reviewFromPath(w, r)
	if !ok {
		return
	}
	if rv.State != "APPROVED" && rv.State != "CHANGES_REQUESTED" {
		ghErrorList(w, 422, "Unprocessable Entity", "Can not dismiss a "+strings.ToLower(rv.State)+" pull request review")
		return
	}
	rv.State = "DISMISSED"
	h.store.PRReviews.Set(id, rv)
	ghJSON(w, 200, h.rd(r).review(rv))
}

// onPull is where webhook delivery attaches to pull request changes.
func (h *Handler) onPull(_ *http.Request, _ string, _ store.PullRequest) {}
