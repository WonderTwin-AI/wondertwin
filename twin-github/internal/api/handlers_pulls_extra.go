package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

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
		ID:          h.store.NewID(store.KindReviewCmt),
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
	ghJSON(w, 200, paginate(w, r, comments))
}
