package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// ListIssues handles GET /repos/{owner}/{repo}/issues
func (h *Handler) ListIssues(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}

	issues := h.store.ListRepoIssues(owner, repo, state)
	ghJSON(w, 200, issues)
}

// CreateIssue handles POST /repos/{owner}/{repo}/issues
func (h *Handler) CreateIssue(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	if _, ok := h.store.GetRepo(owner, repo); !ok {
		ghError(w, 404, "Not Found")
		return
	}

	var req struct {
		Title     string   `json:"title"`
		Body      string   `json:"body"`
		Labels    []string `json:"labels,omitempty"`
		Assignees []string `json:"assignees,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if req.Title == "" {
		ghValidationError(w, "Issue", "title", "missing_field")
		return
	}

	now := h.store.Now()
	num := h.store.NextIssueNumber()
	issue := store.Issue{
		ID:        h.store.NextID(),
		Number:    num,
		Title:     req.Title,
		Body:      req.Body,
		State:     "open",
		User:      store.User{ID: 1, Login: "twin-bot", Type: "User"},
		HTMLURL:   fmt.Sprintf("%s/%s/%s/issues/%d", h.store.BaseURL(), owner, repo, num),
		CreatedAt: now,
		UpdatedAt: now,
		RepoOwner: owner,
		RepoName:  repo,
	}

	// Attach labels
	for _, name := range req.Labels {
		issue.Labels = append(issue.Labels, store.Label{Name: name, Color: "ededed"})
	}

	id := h.store.Issues.NextID()
	h.store.Issues.Set(id, issue)
	ghJSON(w, 201, issue)
}

// GetIssue handles GET /repos/{owner}/{repo}/issues/{issue_number}
func (h *Handler) GetIssue(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))

	issue, _, ok := h.store.GetIssue(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, issue)
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

	var req map[string]any
	json.NewDecoder(r.Body).Decode(&req)

	if title, ok := req["title"].(string); ok {
		issue.Title = title
	}
	if body, ok := req["body"].(string); ok {
		issue.Body = body
	}
	if state, ok := req["state"].(string); ok {
		issue.State = state
		if state == "closed" {
			issue.ClosedAt = h.store.Now()
		} else {
			issue.ClosedAt = ""
		}
	}
	issue.UpdatedAt = h.store.Now()

	h.store.Issues.Set(id, *issue)
	ghJSON(w, 200, issue)
}

// ListIssueComments handles GET /repos/{owner}/{repo}/issues/{issue_number}/comments
func (h *Handler) ListIssueComments(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))

	comments := h.store.ListIssueComments(owner, repo, num)
	ghJSON(w, 200, comments)
}

// CreateIssueComment handles POST /repos/{owner}/{repo}/issues/{issue_number}/comments
func (h *Handler) CreateIssueComment(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))

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
	cid := h.store.NextID()
	comment := store.Comment{
		ID:          cid,
		Body:        req.Body,
		User:        store.User{ID: 1, Login: "twin-bot", Type: "User"},
		HTMLURL:     fmt.Sprintf("%s/%s/%s/issues/%d#issuecomment-%d", h.store.BaseURL(), owner, repo, num, cid),
		CreatedAt:   now,
		UpdatedAt:   now,
		RepoOwner:   owner,
		RepoName:    repo,
		IssueNumber: num,
	}

	id := h.store.Comments.NextID()
	h.store.Comments.Set(id, comment)

	// Increment comment count
	if iss, issID, ok := h.store.GetIssue(owner, repo, num); ok {
		iss.Comments++
		h.store.Issues.Set(issID, *iss)
	}

	ghJSON(w, 201, comment)
}

// GetIssueComment handles GET /repos/{owner}/{repo}/issues/comments/{comment_id}
func (h *Handler) GetIssueComment(w http.ResponseWriter, r *http.Request) {
	commentID, _ := strconv.ParseInt(chi.URLParam(r, "comment_id"), 10, 64)

	ids, comments := h.store.Comments.FilterWithIDs(func(_ string, c store.Comment) bool {
		return c.ID == commentID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, comments[0])
}

// UpdateIssueComment handles PATCH /repos/{owner}/{repo}/issues/comments/{comment_id}
func (h *Handler) UpdateIssueComment(w http.ResponseWriter, r *http.Request) {
	commentID, _ := strconv.ParseInt(chi.URLParam(r, "comment_id"), 10, 64)

	ids, comments := h.store.Comments.FilterWithIDs(func(_ string, c store.Comment) bool {
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
	h.store.Comments.Set(ids[0], c)
	ghJSON(w, 200, c)
}

// DeleteIssueComment handles DELETE /repos/{owner}/{repo}/issues/comments/{comment_id}
func (h *Handler) DeleteIssueComment(w http.ResponseWriter, r *http.Request) {
	commentID, _ := strconv.ParseInt(chi.URLParam(r, "comment_id"), 10, 64)

	ids, _ := h.store.Comments.FilterWithIDs(func(_ string, c store.Comment) bool {
		return c.ID == commentID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.Comments.Delete(ids[0])
	w.WriteHeader(204)
}

// ListLabels handles GET /repos/{owner}/{repo}/labels
func (h *Handler) ListLabels(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	labels := h.store.ListRepoLabels(owner, repo)
	ghJSON(w, 200, labels)
}

// CreateLabel handles POST /repos/{owner}/{repo}/labels
func (h *Handler) CreateLabel(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

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
		ghError(w, 422, "Validation Failed")
		return
	}

	label := store.Label{
		ID:          h.store.NextID(),
		Name:        req.Name,
		Color:       req.Color,
		Description: req.Description,
		RepoOwner:   owner,
		RepoName:    repo,
	}
	id := h.store.Labels.NextID()
	h.store.Labels.Set(id, label)
	ghJSON(w, 201, label)
}

// GetLabel handles GET /repos/{owner}/{repo}/labels/{name}
func (h *Handler) GetLabel(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	name := chi.URLParam(r, "name")

	label, _, ok := h.store.GetLabel(owner, repo, name)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, label)
}

// UpdateLabel handles PATCH /repos/{owner}/{repo}/labels/{name}
func (h *Handler) UpdateLabel(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	name := chi.URLParam(r, "name")

	label, id, ok := h.store.GetLabel(owner, repo, name)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	var req map[string]any
	json.NewDecoder(r.Body).Decode(&req)

	if newName, ok := req["new_name"].(string); ok {
		label.Name = newName
	}
	if color, ok := req["color"].(string); ok {
		label.Color = color
	}
	if desc, ok := req["description"].(string); ok {
		label.Description = desc
	}

	h.store.Labels.Set(id, *label)
	ghJSON(w, 200, label)
}

// DeleteLabel handles DELETE /repos/{owner}/{repo}/labels/{name}
func (h *Handler) DeleteLabel(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	name := chi.URLParam(r, "name")

	_, id, ok := h.store.GetLabel(owner, repo, name)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.Labels.Delete(id)
	w.WriteHeader(204)
}

// ListIssueLabels handles GET /repos/{owner}/{repo}/issues/{issue_number}/labels
func (h *Handler) ListIssueLabels(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))

	issue, _, ok := h.store.GetIssue(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, issue.Labels)
}

// AddIssueLabels handles POST /repos/{owner}/{repo}/issues/{issue_number}/labels
func (h *Handler) AddIssueLabels(w http.ResponseWriter, r *http.Request) {
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

	for _, name := range req.Labels {
		issue.Labels = append(issue.Labels, store.Label{Name: name, Color: "ededed"})
	}
	h.store.Issues.Set(id, *issue)
	ghJSON(w, 200, issue.Labels)
}

// RemoveIssueLabel handles DELETE /repos/{owner}/{repo}/issues/{issue_number}/labels/{name}
func (h *Handler) RemoveIssueLabel(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num, _ := strconv.Atoi(chi.URLParam(r, "issue_number"))
	name := chi.URLParam(r, "name")

	issue, id, ok := h.store.GetIssue(owner, repo, num)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	labels := make([]store.Label, 0, len(issue.Labels))
	found := false
	for _, l := range issue.Labels {
		if l.Name == name {
			found = true
			continue
		}
		labels = append(labels, l)
	}
	if !found {
		ghError(w, 404, "Not Found")
		return
	}

	issue.Labels = labels
	h.store.Issues.Set(id, *issue)
	ghJSON(w, 200, labels)
}
