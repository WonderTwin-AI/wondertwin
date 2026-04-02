package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// ListMilestones handles GET /repos/{owner}/{repo}/milestones
func (h *Handler) ListMilestones(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	milestones := h.store.ListRepoMilestones(owner, repo)
	ghJSON(w, 200, milestones)
}

// CreateMilestone handles POST /repos/{owner}/{repo}/milestones
func (h *Handler) CreateMilestone(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if req.Title == "" {
		ghValidationError(w, "Milestone", "title", "missing_field")
		return
	}
	if req.State == "" {
		req.State = "open"
	}

	now := h.store.Now()
	num := int(h.store.NextID())
	ms := store.Milestone{
		ID:          h.store.NextID(),
		Number:      num,
		Title:       req.Title,
		Description: req.Description,
		State:       req.State,
		HTMLURL:     fmt.Sprintf("%s/%s/%s/milestone/%d", h.store.BaseURL(), owner, repo, num),
		CreatedAt:   now,
		UpdatedAt:   now,
		RepoOwner:   owner,
		RepoName:    repo,
	}

	id := h.store.Milestones.NextID()
	h.store.Milestones.Set(id, ms)
	ghJSON(w, 201, ms)
}

// GetMilestone handles GET /repos/{owner}/{repo}/milestones/{milestone_number}
func (h *Handler) GetMilestone(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "milestone_number"))

	for _, ms := range h.store.ListRepoMilestones(owner, repo) {
		if ms.Number == num {
			ghJSON(w, 200, ms)
			return
		}
	}
	ghError(w, 404, "Not Found")
}

// UpdateMilestone handles PATCH /repos/{owner}/{repo}/milestones/{milestone_number}
func (h *Handler) UpdateMilestone(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "milestone_number"))

	ids, milestones := h.store.Milestones.FilterWithIDs(func(_ string, ms store.Milestone) bool {
		return ms.RepoOwner == owner && ms.RepoName == repo && ms.Number == num
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}

	var req map[string]any
	json.NewDecoder(r.Body).Decode(&req)

	ms := milestones[0]
	if title, ok := req["title"].(string); ok {
		ms.Title = title
	}
	if desc, ok := req["description"].(string); ok {
		ms.Description = desc
	}
	if state, ok := req["state"].(string); ok {
		ms.State = state
	}
	ms.UpdatedAt = h.store.Now()

	h.store.Milestones.Set(ids[0], ms)
	ghJSON(w, 200, ms)
}

// DeleteMilestone handles DELETE /repos/{owner}/{repo}/milestones/{milestone_number}
func (h *Handler) DeleteMilestone(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "milestone_number"))

	ids, _ := h.store.Milestones.FilterWithIDs(func(_ string, ms store.Milestone) bool {
		return ms.RepoOwner == owner && ms.RepoName == repo && ms.Number == num
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.Milestones.Delete(ids[0])
	w.WriteHeader(204)
}
