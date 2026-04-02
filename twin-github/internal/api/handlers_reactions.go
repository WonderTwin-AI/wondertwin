package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// ListIssueReactions handles GET /repos/{owner}/{repo}/issues/{issue_number}/reactions
func (h *Handler) ListIssueReactions(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num := chi.URLParam(r, "issue_number")

	reactions := h.store.ListSubjectReactions(owner, repo, "issue:"+num)
	ghJSON(w, 200, reactions)
}

// CreateIssueReaction handles POST /repos/{owner}/{repo}/issues/{issue_number}/reactions
func (h *Handler) CreateIssueReaction(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num := chi.URLParam(r, "issue_number")

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}

	rx := store.Reaction{
		ID:        h.store.NextID(),
		User:      store.User{ID: 1, Login: "twin-bot", Type: "User"},
		Content:   req.Content,
		CreatedAt: h.store.Now(),
		RepoOwner: owner,
		RepoName:  repo,
		Subject:   "issue:" + num,
	}
	id := h.store.Reactions.NextID()
	h.store.Reactions.Set(id, rx)
	ghJSON(w, 201, rx)
}

// DeleteIssueReaction handles DELETE /repos/{owner}/{repo}/issues/{issue_number}/reactions/{reaction_id}
func (h *Handler) DeleteIssueReaction(w http.ResponseWriter, r *http.Request) {
	reactionID, _ := strconv.ParseInt(chi.URLParam(r, "reaction_id"), 10, 64)

	ids, _ := h.store.Reactions.FilterWithIDs(func(_ string, rx store.Reaction) bool {
		return rx.ID == reactionID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.Reactions.Delete(ids[0])
	w.WriteHeader(204)
}

// ListCommentReactions handles GET /repos/{owner}/{repo}/issues/comments/{comment_id}/reactions
func (h *Handler) ListCommentReactions(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	commentID := chi.URLParam(r, "comment_id")

	reactions := h.store.ListSubjectReactions(owner, repo, "comment:"+commentID)
	ghJSON(w, 200, reactions)
}

// CreateCommentReaction handles POST /repos/{owner}/{repo}/issues/comments/{comment_id}/reactions
func (h *Handler) CreateCommentReaction(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	commentID := chi.URLParam(r, "comment_id")

	var req struct {
		Content string `json:"content"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	rx := store.Reaction{
		ID:        h.store.NextID(),
		User:      store.User{ID: 1, Login: "twin-bot", Type: "User"},
		Content:   req.Content,
		CreatedAt: h.store.Now(),
		RepoOwner: owner,
		RepoName:  repo,
		Subject:   "comment:" + commentID,
	}
	id := h.store.Reactions.NextID()
	h.store.Reactions.Set(id, rx)
	ghJSON(w, 201, rx)
}

// DeleteCommentReaction handles DELETE /repos/{owner}/{repo}/issues/comments/{comment_id}/reactions/{reaction_id}
func (h *Handler) DeleteCommentReaction(w http.ResponseWriter, r *http.Request) {
	reactionID, _ := strconv.ParseInt(chi.URLParam(r, "reaction_id"), 10, 64)

	ids, _ := h.store.Reactions.FilterWithIDs(func(_ string, rx store.Reaction) bool {
		return rx.ID == reactionID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.Reactions.Delete(ids[0])
	w.WriteHeader(204)
}

// SearchIssues handles GET /search/issues
func (h *Handler) SearchIssues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	_ = q // Search is simplified — return all issues

	issues := h.store.Issues.List()
	ghJSON(w, 200, map[string]any{
		"total_count":        len(issues),
		"incomplete_results": false,
		"items":              issues,
	})
}

// SearchRepos handles GET /search/repositories
func (h *Handler) SearchRepos(w http.ResponseWriter, r *http.Request) {
	repos := h.store.Repos.List()
	ghJSON(w, 200, map[string]any{
		"total_count":        len(repos),
		"incomplete_results": false,
		"items":              repos,
	})
}

// SearchUsers handles GET /search/users
func (h *Handler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	users := h.store.Users.List()
	ghJSON(w, 200, map[string]any{
		"total_count":        len(users),
		"incomplete_results": false,
		"items":              users,
	})
}

// ListCollaborators handles GET /repos/{owner}/{repo}/collaborators
func (h *Handler) ListCollaborators(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, []any{})
}

// AddCollaborator handles PUT /repos/{owner}/{repo}/collaborators/{username}
func (h *Handler) AddCollaborator(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 201, map[string]any{
		"id":         h.store.NextID(),
		"repository": chi.URLParam(r, "repo"),
		"invitee":    store.User{Login: chi.URLParam(r, "username")},
	})
}

// RemoveCollaborator handles DELETE /repos/{owner}/{repo}/collaborators/{username}
func (h *Handler) RemoveCollaborator(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(204)
}

// GetCollaboratorPermission handles GET /repos/{owner}/{repo}/collaborators/{username}/permission
func (h *Handler) GetCollaboratorPermission(w http.ResponseWriter, r *http.Request) {
	ghJSON(w, 200, map[string]any{
		"permission": "write",
		"user":       store.User{Login: chi.URLParam(r, "username")},
	})
}

// ListRepoTopics handles GET /repos/{owner}/{repo}/topics
func (h *Handler) ListRepoTopics(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	rp, ok := h.store.GetRepo(owner, repo)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, map[string]any{"names": rp.Topics})
}

// ReplaceRepoTopics handles PUT /repos/{owner}/{repo}/topics
func (h *Handler) ReplaceRepoTopics(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	rp, ok := h.store.GetRepo(owner, repo)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	var req struct {
		Names []string `json:"names"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	rp.Topics = req.Names
	h.store.Repos.Set(store.RepoKey(owner, repo), *rp)
	ghJSON(w, 200, map[string]any{"names": rp.Topics})
}

// CreateFork handles POST /repos/{owner}/{repo}/forks
func (h *Handler) CreateFork(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	rp, ok := h.store.GetRepo(owner, repo)
	if !ok {
		ghError(w, 404, "Not Found")
		return
	}

	forkOwner := "twin-bot"
	now := h.store.Now()
	fork := *rp
	fork.ID = h.store.NextID()
	fork.FullName = forkOwner + "/" + repo
	fork.Fork = true
	fork.Owner = store.User{ID: 1, Login: forkOwner, Type: "User"}
	fork.HTMLURL = fmt.Sprintf("%s/%s/%s", h.store.BaseURL(), forkOwner, repo)
	fork.CreatedAt = now
	fork.UpdatedAt = now

	h.store.Repos.Set(store.RepoKey(forkOwner, repo), fork)
	ghJSON(w, 202, fork)
}

// ListReleaseAssets handles GET /repos/{owner}/{repo}/releases/{release_id}/assets
func (h *Handler) ListReleaseAssets(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	releaseID, _ := strconv.ParseInt(chi.URLParam(r, "release_id"), 10, 64)

	assets := h.store.ListReleaseAssets(owner, repo, releaseID)
	ghJSON(w, 200, assets)
}

// UploadReleaseAsset handles POST /repos/{owner}/{repo}/releases/{release_id}/assets
func (h *Handler) UploadReleaseAsset(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	releaseID, _ := strconv.ParseInt(chi.URLParam(r, "release_id"), 10, 64)
	name := r.URL.Query().Get("name")

	now := h.store.Now()
	asset := store.ReleaseAsset{
		ID:                 h.store.NextID(),
		Name:               name,
		ContentType:        r.Header.Get("Content-Type"),
		Size:               int(r.ContentLength),
		State:              "uploaded",
		BrowserDownloadURL: fmt.Sprintf("%s/%s/%s/releases/download/v1/%s", h.store.BaseURL(), owner, repo, name),
		CreatedAt:          now,
		UpdatedAt:          now,
		Uploader:           store.User{ID: 1, Login: "twin-bot", Type: "User"},
		RepoOwner:          owner,
		RepoName:           repo,
		ReleaseID:          releaseID,
	}
	id := h.store.ReleaseAssets.NextID()
	h.store.ReleaseAssets.Set(id, asset)
	ghJSON(w, 201, asset)
}
