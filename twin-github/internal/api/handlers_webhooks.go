package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// ListWebhooks handles GET /repos/{owner}/{repo}/hooks
func (h *Handler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	hooks := h.store.ListRepoWebhooks(owner, repo)
	ghJSON(w, 200, hooks)
}

// CreateWebhook handles POST /repos/{owner}/{repo}/hooks
func (h *Handler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	var req struct {
		Name   string            `json:"name"`
		Active *bool             `json:"active,omitempty"`
		Events []string          `json:"events"`
		Config store.WebhookConfig `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}
	if req.Name == "" {
		req.Name = "web"
	}

	now := h.store.Now()
	hook := store.Webhook{
		ID:        h.store.NextID(),
		Name:      req.Name,
		Active:    active,
		Events:    req.Events,
		Config:    req.Config,
		CreatedAt: now,
		UpdatedAt: now,
		RepoOwner: owner,
		RepoName:  repo,
	}

	id := h.store.Webhooks.NextID()
	h.store.Webhooks.Set(id, hook)
	ghJSON(w, 201, hook)
}

// GetWebhook handles GET /repos/{owner}/{repo}/hooks/{hook_id}
func (h *Handler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	hookID, _ := strconv.ParseInt(chi.URLParam(r, "hook_id"), 10, 64)

	_, hooks := h.store.Webhooks.FilterWithIDs(func(_ string, wh store.Webhook) bool {
		return wh.ID == hookID
	})
	if len(hooks) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, hooks[0])
}

// UpdateWebhook handles PATCH /repos/{owner}/{repo}/hooks/{hook_id}
func (h *Handler) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	hookID, _ := strconv.ParseInt(chi.URLParam(r, "hook_id"), 10, 64)

	ids, hooks := h.store.Webhooks.FilterWithIDs(func(_ string, wh store.Webhook) bool {
		return wh.ID == hookID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}

	var req map[string]any
	json.NewDecoder(r.Body).Decode(&req)

	hook := hooks[0]
	if active, ok := req["active"].(bool); ok {
		hook.Active = active
	}
	if events, ok := req["events"].([]any); ok {
		hook.Events = make([]string, len(events))
		for i, e := range events {
			hook.Events[i], _ = e.(string)
		}
	}
	hook.UpdatedAt = h.store.Now()

	h.store.Webhooks.Set(ids[0], hook)
	ghJSON(w, 200, hook)
}

// DeleteWebhook handles DELETE /repos/{owner}/{repo}/hooks/{hook_id}
func (h *Handler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	hookID, _ := strconv.ParseInt(chi.URLParam(r, "hook_id"), 10, 64)

	ids, _ := h.store.Webhooks.FilterWithIDs(func(_ string, wh store.Webhook) bool {
		return wh.ID == hookID
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.Webhooks.Delete(ids[0])
	w.WriteHeader(204)
}
