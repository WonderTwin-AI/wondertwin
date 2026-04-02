package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// ListPRCommits handles GET /repos/{owner}/{repo}/pulls/{pull_number}/commits
func (h *Handler) ListPRCommits(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "pull_number"))

	pr, _, ok := h.store.GetPR(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	ghJSON(w, 200, []map[string]any{
		{
			"sha": pr.Head.SHA,
			"commit": map[string]any{
				"message": "PR commit",
				"author":  store.GitSignature{Name: "twin-bot", Email: "bot@wondertwin.dev", Date: h.store.Now()},
			},
		},
	})
}

// ListPRFiles handles GET /repos/{owner}/{repo}/pulls/{pull_number}/files
func (h *Handler) ListPRFiles(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []map[string]any{
		{
			"sha":       store.DefaultSHA(),
			"filename":  "example.go",
			"status":    "modified",
			"additions": 10,
			"deletions": 2,
			"changes":   12,
			"patch":     "@@ -1,5 +1,13 @@",
		},
	})
}

// ListRequestedReviewers handles GET /repos/{owner}/{repo}/pulls/{pull_number}/requested_reviewers
func (h *Handler) ListRequestedReviewers(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{
		"users": []any{},
		"teams": []any{},
	})
}

// RequestReviewers handles POST /repos/{owner}/{repo}/pulls/{pull_number}/requested_reviewers
func (h *Handler) RequestReviewers(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "pull_number"))

	pr, _, ok := h.store.GetPR(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 201, pr)
}

// RemoveRequestedReviewers handles DELETE /repos/{owner}/{repo}/pulls/{pull_number}/requested_reviewers
func (h *Handler) RemoveRequestedReviewers(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "pull_number"))

	pr, _, ok := h.store.GetPR(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, pr)
}

// UpdatePRBranch handles PUT /repos/{owner}/{repo}/pulls/{pull_number}/update-branch
func (h *Handler) UpdatePRBranch(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "pull_number"))

	pr, id, ok := h.store.GetPR(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	pr.Head.SHA = store.MakeSHA(pr.Head.SHA + h.store.Now())
	pr.UpdatedAt = h.store.Now()
	h.store.PullRequests.Set(id, *pr)

	ghJSON(w, 202, map[string]any{
		"message": "Updating pull request branch.",
		"url":     pr.HTMLURL,
	})
}

// UpdatePRReview handles PATCH /repos/{owner}/{repo}/pulls/{pull_number}/reviews/{review_id}
func (h *Handler) UpdatePRReview(w http.ResponseWriter, r *http.Request) {
	reviewID, _ := strconv.ParseInt(chi.URLParam(r, "review_id"), 10, 64)

	ids, reviews := h.store.PRReviews.FilterWithIDs(func(_ string, rv store.PRReview) bool {
		return rv.ID == reviewID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}

	var req struct {
		Body string `json:"body"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	rv := reviews[0]
	rv.Body = req.Body
	h.store.PRReviews.Set(ids[0], rv)
	ghJSON(w, 200, rv)
}

// SubmitPRReview handles POST /repos/{owner}/{repo}/pulls/{pull_number}/reviews/{review_id}/events
func (h *Handler) SubmitPRReview(w http.ResponseWriter, r *http.Request) {
	reviewID, _ := strconv.ParseInt(chi.URLParam(r, "review_id"), 10, 64)

	ids, reviews := h.store.PRReviews.FilterWithIDs(func(_ string, rv store.PRReview) bool {
		return rv.ID == reviewID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}

	var req struct {
		Body  string `json:"body"`
		Event string `json:"event"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	rv := reviews[0]
	switch req.Event {
	case "APPROVE":
		rv.State = "APPROVED"
	case "REQUEST_CHANGES":
		rv.State = "CHANGES_REQUESTED"
	default:
		rv.State = "COMMENTED"
	}
	if req.Body != "" {
		rv.Body = req.Body
	}
	rv.SubmittedAt = h.store.Now()
	h.store.PRReviews.Set(ids[0], rv)
	ghJSON(w, 200, rv)
}

// ReplyToPRReviewComment handles POST /repos/{owner}/{repo}/pulls/{pull_number}/comments/{comment_id}/replies
func (h *Handler) ReplyToPRReviewComment(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "pull_number"))
	parentID, _ := strconv.ParseInt(chi.URLParam(r, "comment_id"), 10, 64)

	var req struct {
		Body string `json:"body"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	now := h.store.Now()
	rc := store.PRReviewComment{
		ID:          h.store.NextID(),
		Body:        req.Body,
		User:        store.User{ID: 1, Login: "twin-bot", Type: "User"},
		CreatedAt:   now,
		UpdatedAt:   now,
		InReplyToID: &parentID,
		RepoOwner:   owner,
		RepoName:    repo,
		PRNumber:    num,
	}
	id := h.store.PRReviewComments.NextID()
	h.store.PRReviewComments.Set(id, rc)
	ghJSON(w, 201, rc)
}

// ListAllPRReviewComments handles GET /repos/{owner}/{repo}/pulls/comments (all in repo)
func (h *Handler) ListAllPRReviewComments(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	comments := h.store.PRReviewComments.Filter(func(_ string, c store.PRReviewComment) bool {
		return c.RepoOwner == owner && c.RepoName == repo
	})
	ghJSON(w, 200, comments)
}

// SetIssueLabels handles PUT /repos/{owner}/{repo}/issues/{issue_number}/labels
func (h *Handler) SetIssueLabels(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))

	issue, id, ok := h.store.GetIssue(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	var req struct {
		Labels []string `json:"labels"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	issue.Labels = nil
	for _, name := range req.Labels {
		issue.Labels = append(issue.Labels, store.Label{Name: name, Color: "ededed"})
	}
	h.store.Issues.Set(id, *issue)
	ghJSON(w, 200, issue.Labels)
}

// RemoveAllIssueLabels handles DELETE /repos/{owner}/{repo}/issues/{issue_number}/labels
func (h *Handler) RemoveAllIssueLabels(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))

	issue, id, ok := h.store.GetIssue(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	issue.Labels = nil
	h.store.Issues.Set(id, *issue)
	w.WriteHeader(204)
}
