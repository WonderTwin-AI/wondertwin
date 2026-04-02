package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// ListPullRequests handles GET /repos/{owner}/{repo}/pulls
func (h *Handler) ListPullRequests(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}

	prs := h.store.ListRepoPRs(owner, repo, state)
	ghJSON(w, 200, prs)
}

// CreatePullRequest handles POST /repos/{owner}/{repo}/pulls
func (h *Handler) CreatePullRequest(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	if _, ok := h.store.GetRepo(owner, repo); !ok {
		ghError(w, 404, "Not Found")
		return
	}

	var req struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		Head  string `json:"head"`
		Base  string `json:"base"`
		Draft bool   `json:"draft"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if req.Title == "" {
		ghValidationError(w, "PullRequest", "title", "missing_field")
		return
	}
	if req.Head == "" || req.Base == "" {
		ghValidationError(w, "PullRequest", "head", "missing_field")
		return
	}

	now := h.store.Now()
	num := h.store.NextIssueNumber()
	mergeable := true
	pr := store.PullRequest{
		ID:     h.store.NextID(),
		Number: num,
		Title:  req.Title,
		Body:   req.Body,
		State:  "open",
		Draft:  req.Draft,
		User:   store.User{ID: 1, Login: "twin-bot", Type: "User"},
		Head: store.PRRef{
			Label: owner + ":" + req.Head,
			Ref:   req.Head,
			SHA:   store.MakeSHA(req.Head),
		},
		Base: store.PRRef{
			Label: owner + ":" + req.Base,
			Ref:   req.Base,
			SHA:   store.MakeSHA(req.Base),
		},
		Mergeable: &mergeable,
		HTMLURL:   fmt.Sprintf("%s/%s/%s/pull/%d", h.store.BaseURL(), owner, repo, num),
		DiffURL:   fmt.Sprintf("%s/%s/%s/pull/%d.diff", h.store.BaseURL(), owner, repo, num),
		CreatedAt: now,
		UpdatedAt: now,
		RepoOwner: owner,
		RepoName:  repo,
	}

	id := h.store.PullRequests.NextID()
	h.store.PullRequests.Set(id, pr)
	ghJSON(w, 201, pr)
}

// GetPullRequest handles GET /repos/{owner}/{repo}/pulls/{pull_number}
func (h *Handler) GetPullRequest(w http.ResponseWriter, r *http.Request) {
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

// UpdatePullRequest handles PATCH /repos/{owner}/{repo}/pulls/{pull_number}
func (h *Handler) UpdatePullRequest(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "pull_number"))

	pr, id, ok := h.store.GetPR(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	var req map[string]any
	json.NewDecoder(r.Body).Decode(&req)

	if title, ok := req["title"].(string); ok {
		pr.Title = title
	}
	if body, ok := req["body"].(string); ok {
		pr.Body = body
	}
	if state, ok := req["state"].(string); ok {
		pr.State = state
		if state == "closed" {
			pr.ClosedAt = h.store.Now()
		}
	}
	pr.UpdatedAt = h.store.Now()

	h.store.PullRequests.Set(id, *pr)
	ghJSON(w, 200, pr)
}

// MergePullRequest handles PUT /repos/{owner}/{repo}/pulls/{pull_number}/merge
func (h *Handler) MergePullRequest(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "pull_number"))

	pr, id, ok := h.store.GetPR(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	if pr.State == "closed" {
		ghError(w, 405, "Pull Request is not mergeable")
		return
	}

	now := h.store.Now()
	sha := store.MakeSHA(fmt.Sprintf("merge-%d-%s", num, now))
	pr.Merged = true
	pr.State = "closed"
	pr.MergeCommitSHA = sha
	pr.MergedAt = now
	pr.ClosedAt = now
	pr.UpdatedAt = now

	h.store.PullRequests.Set(id, *pr)

	ghJSON(w, 200, map[string]any{
		"sha":     sha,
		"merged":  true,
		"message": "Pull Request successfully merged",
	})
}

// CheckPRMerged handles GET /repos/{owner}/{repo}/pulls/{pull_number}/merge
func (h *Handler) CheckPRMerged(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "pull_number"))

	pr, _, ok := h.store.GetPR(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	if pr.Merged {
		w.WriteHeader(204)
	} else {
		ghError(w, 404, "Not Found")
	}
}
