package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// ListPRReviews handles GET /repos/{owner}/{repo}/pulls/{pull_number}/reviews
func (h *Handler) ListPRReviews(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "pull_number"))

	reviews := h.store.ListPRReviews(owner, repo, num)
	ghJSON(w, 200, reviews)
}

// CreatePRReview handles POST /repos/{owner}/{repo}/pulls/{pull_number}/reviews
func (h *Handler) CreatePRReview(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "pull_number"))

	pr, _, ok := h.store.GetPR(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	var req struct {
		Body  string `json:"body"`
		Event string `json:"event"` // "APPROVE", "REQUEST_CHANGES", "COMMENT"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}

	state := "COMMENTED"
	if req.Event != "" {
		switch req.Event {
		case "APPROVE":
			state = "APPROVED"
		case "REQUEST_CHANGES":
			state = "CHANGES_REQUESTED"
		default:
			state = "COMMENTED"
		}
	}

	review := store.PRReview{
		ID:          h.store.NextID(),
		User:        store.User{ID: 1, Login: "twin-bot", Type: "User"},
		Body:        req.Body,
		State:       state,
		CommitID:    pr.Head.SHA,
		HTMLURL:     fmt.Sprintf("%s/%s/%s/pull/%d#pullrequestreview-%d", h.store.BaseURL(), owner, repo, num, h.store.NextID()),
		SubmittedAt: h.store.Now(),
		RepoOwner:   owner,
		RepoName:    repo,
		PRNumber:    num,
	}

	id := h.store.PRReviews.NextID()
	h.store.PRReviews.Set(id, review)
	ghJSON(w, 200, review)
}

// GetPRReview handles GET /repos/{owner}/{repo}/pulls/{pull_number}/reviews/{review_id}
func (h *Handler) GetPRReview(w http.ResponseWriter, r *http.Request) {
	reviewID, _ := strconv.ParseInt(chi.URLParam(r, "review_id"), 10, 64)

	_, reviews := h.store.PRReviews.FilterWithIDs(func(_ string, rv store.PRReview) bool {
		return rv.ID == reviewID
	})
	if len(reviews) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, reviews[0])
}

// DismissPRReview handles PUT /repos/{owner}/{repo}/pulls/{pull_number}/reviews/{review_id}/dismissals
func (h *Handler) DismissPRReview(w http.ResponseWriter, r *http.Request) {
	reviewID, _ := strconv.ParseInt(chi.URLParam(r, "review_id"), 10, 64)

	ids, reviews := h.store.PRReviews.FilterWithIDs(func(_ string, rv store.PRReview) bool {
		return rv.ID == reviewID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}

	rv := reviews[0]
	rv.State = "DISMISSED"
	h.store.PRReviews.Set(ids[0], rv)
	ghJSON(w, 200, rv)
}

// ListPRReviewComments handles GET /repos/{owner}/{repo}/pulls/{pull_number}/comments
func (h *Handler) ListPRReviewComments(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "pull_number"))

	comments := h.store.PRReviewComments.Filter(func(_ string, c store.PRReviewComment) bool {
		return c.RepoOwner == owner && c.RepoName == repo && c.PRNumber == num
	})
	ghJSON(w, 200, comments)
}

// CreatePRReviewComment handles POST /repos/{owner}/{repo}/pulls/{pull_number}/comments
func (h *Handler) CreatePRReviewComment(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "pull_number"))

	pr, _, ok := h.store.GetPR(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	var req struct {
		Body     string `json:"body"`
		Path     string `json:"path"`
		Position *int   `json:"position,omitempty"`
		Line     *int   `json:"line,omitempty"`
		Side     string `json:"side,omitempty"`
		CommitID string `json:"commit_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}

	commitID := req.CommitID
	if commitID == "" {
		commitID = pr.Head.SHA
	}

	now := h.store.Now()
	rc := store.PRReviewComment{
		ID:               h.store.NextID(),
		Body:             req.Body,
		Path:             req.Path,
		Position:         req.Position,
		Line:             req.Line,
		Side:             req.Side,
		CommitID:         commitID,
		OriginalCommitID: commitID,
		DiffHunk:         "@@ -0,0 +1 @@",
		User:             store.User{ID: 1, Login: "twin-bot", Type: "User"},
		HTMLURL:          fmt.Sprintf("%s/%s/%s/pull/%d#discussion_r%d", h.store.BaseURL(), owner, repo, num, h.store.NextID()),
		CreatedAt:        now,
		UpdatedAt:        now,
		RepoOwner:        owner,
		RepoName:         repo,
		PRNumber:         num,
	}

	id := h.store.PRReviewComments.NextID()
	h.store.PRReviewComments.Set(id, rc)
	ghJSON(w, 201, rc)
}

// GetPRReviewComment handles GET /repos/{owner}/{repo}/pulls/comments/{comment_id}
func (h *Handler) GetPRReviewComment(w http.ResponseWriter, r *http.Request) {
	commentID, _ := strconv.ParseInt(chi.URLParam(r, "comment_id"), 10, 64)

	_, comments := h.store.PRReviewComments.FilterWithIDs(func(_ string, c store.PRReviewComment) bool {
		return c.ID == commentID
	})
	if len(comments) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, comments[0])
}

// UpdatePRReviewComment handles PATCH /repos/{owner}/{repo}/pulls/comments/{comment_id}
func (h *Handler) UpdatePRReviewComment(w http.ResponseWriter, r *http.Request) {
	commentID, _ := strconv.ParseInt(chi.URLParam(r, "comment_id"), 10, 64)

	ids, comments := h.store.PRReviewComments.FilterWithIDs(func(_ string, c store.PRReviewComment) bool {
		return c.ID == commentID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}

	var req struct {
		Body string `json:"body"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	c := comments[0]
	c.Body = req.Body
	c.UpdatedAt = h.store.Now()
	h.store.PRReviewComments.Set(ids[0], c)
	ghJSON(w, 200, c)
}

// DeletePRReviewComment handles DELETE /repos/{owner}/{repo}/pulls/comments/{comment_id}
func (h *Handler) DeletePRReviewComment(w http.ResponseWriter, r *http.Request) {
	commentID, _ := strconv.ParseInt(chi.URLParam(r, "comment_id"), 10, 64)

	ids, _ := h.store.PRReviewComments.FilterWithIDs(func(_ string, c store.PRReviewComment) bool {
		return c.ID == commentID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.PRReviewComments.Delete(ids[0])
	w.WriteHeader(204)
}
