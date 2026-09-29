package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

func (x renderer) hook(hk store.Webhook) map[string]any {
	u := x.api("/repos/%s/%s/hooks/%d", hk.RepoOwner, hk.RepoName, hk.ID)
	config := map[string]any{"url": hk.Config.URL, "content_type": hk.Config.ContentType, "insecure_ssl": hk.Config.InsecureSSL}
	if hk.Config.Secret != "" {
		config["secret"] = "********"
	}
	last := map[string]any{"code": nil, "status": "unused", "message": nil}
	if hk.LastResponse != nil {
		last = map[string]any{"code": hk.LastResponse.Code, "status": hk.LastResponse.Status, "message": hk.LastResponse.Message}
	}
	return map[string]any{
		"type": "Repository", "id": hk.ID, "name": "web", "active": hk.Active, "events": nonNil(hk.Events),
		"config": config, "updated_at": hk.UpdatedAt, "created_at": hk.CreatedAt,
		"url": u, "test_url": u + "/test", "ping_url": u + "/pings", "deliveries_url": u + "/deliveries",
		"last_response": last,
	}
}

// ListWebhooks handles GET /repos/{owner}/{repo}/hooks
func (h *Handler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	owner, repo := param(r, "owner"), param(r, "repo")
	if _, ok := h.store.GetRepo(owner, repo); !ok {
		ghError(w, 404, "Not Found")
		return
	}
	x := h.rd(r)
	out := []map[string]any{}
	for _, hk := range paginate(w, r, h.store.ListRepoWebhooks(owner, repo)) {
		out = append(out, x.hook(hk))
	}
	ghJSON(w, 200, out)
}

type hookRequest struct {
	Active       *bool     `json:"active"`
	Events       *[]string `json:"events"`
	AddEvents    []string  `json:"add_events"`
	RemoveEvents []string  `json:"remove_events"`
	Config       *struct {
		URL         *string         `json:"url"`
		ContentType *string         `json:"content_type"`
		Secret      *string         `json:"secret"`
		InsecureSSL json.RawMessage `json:"insecure_ssl"`
	} `json:"config"`
}

func (req hookRequest) apply(hk *store.Webhook) {
	if req.Active != nil {
		hk.Active = *req.Active
	}
	if req.Events != nil {
		hk.Events = *req.Events
	}
	hk.Events = dedupe(append(hk.Events, req.AddEvents...))
	if len(req.RemoveEvents) > 0 {
		var kept []string
		for _, e := range hk.Events {
			if !contains(req.RemoveEvents, e) {
				kept = append(kept, e)
			}
		}
		hk.Events = kept
	}
	if c := req.Config; c != nil {
		if c.URL != nil {
			hk.Config.URL = *c.URL
		}
		if c.ContentType != nil {
			hk.Config.ContentType = *c.ContentType
		}
		if c.Secret != nil {
			hk.Config.Secret = *c.Secret
		}
		if len(c.InsecureSSL) > 0 {
			hk.Config.InsecureSSL = strings.Trim(string(c.InsecureSSL), `"`)
		}
	}
}

// CreateWebhook handles POST /repos/{owner}/{repo}/hooks. GitHub defaults
// events to push and content_type to form, and sends a ping on creation.
func (h *Handler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	owner, repo := param(r, "owner"), param(r, "repo")
	if _, ok := h.store.GetRepo(owner, repo); !ok {
		ghError(w, 404, "Not Found")
		return
	}
	var req hookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	now := h.store.Now()
	hk := store.Webhook{ID: h.store.NewID(store.KindHook), Name: "web", Active: true, Events: []string{"push"},
		Config: store.WebhookConfig{ContentType: "form", InsecureSSL: "0"}, CreatedAt: now, UpdatedAt: now,
		RepoOwner: owner, RepoName: repo}
	req.apply(&hk)
	if hk.Config.URL == "" {
		ghValidationErrors(w, "Validation Failed", map[string]any{"resource": "Hook", "code": "custom", "message": "Config must contain URL for webhooks"})
		return
	}
	if hk.Config.ContentType != "json" && hk.Config.ContentType != "form" {
		ghValidationErrors(w, "Validation Failed", map[string]any{"resource": "Hook", "code": "custom", "message": "Config content_type must be json or form"})
		return
	}
	for _, other := range h.store.ListRepoWebhooks(owner, repo) {
		if other.Config.URL == hk.Config.URL {
			ghValidationErrors(w, "Validation Failed", map[string]any{"resource": "Hook", "code": "custom", "message": "Hook already exists on this repository"})
			return
		}
	}
	h.store.Webhooks.Set(strconv.FormatInt(hk.ID, 10), hk)
	h.ping(r, hk)
	ghJSON(w, 201, h.rd(r).hook(hk))
}

func (h *Handler) ping(r *http.Request, hk store.Webhook) {
	h.emit(r, hk.RepoOwner, hk.RepoName, "ping", "", map[string]any{"zen": zen[int(hk.ID)%len(zen)]}, hk.ID)
}

func (h *Handler) hookFromPath(w http.ResponseWriter, r *http.Request) (store.Webhook, bool) {
	hk, ok := h.store.Webhooks.Get(param(r, "hook_id"))
	if !ok || hk.RepoOwner != param(r, "owner") || hk.RepoName != param(r, "repo") {
		ghError(w, 404, "Not Found")
		return hk, false
	}
	return hk, true
}

// GetWebhook handles GET /repos/{owner}/{repo}/hooks/{hook_id}
func (h *Handler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	if hk, ok := h.hookFromPath(w, r); ok {
		ghJSON(w, 200, h.rd(r).hook(hk))
	}
}

// UpdateWebhook handles PATCH /repos/{owner}/{repo}/hooks/{hook_id}
func (h *Handler) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	hk, ok := h.hookFromPath(w, r)
	if !ok {
		return
	}
	var req hookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	req.apply(&hk)
	hk.UpdatedAt = h.store.Now()
	h.store.Webhooks.Set(strconv.FormatInt(hk.ID, 10), hk)
	ghJSON(w, 200, h.rd(r).hook(hk))
}

// DeleteWebhook handles DELETE /repos/{owner}/{repo}/hooks/{hook_id}
func (h *Handler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	hk, ok := h.hookFromPath(w, r)
	if !ok {
		return
	}
	h.store.Webhooks.Delete(strconv.FormatInt(hk.ID, 10))
	w.WriteHeader(204)
}

// PingWebhook handles POST /repos/{owner}/{repo}/hooks/{hook_id}/pings
func (h *Handler) PingWebhook(w http.ResponseWriter, r *http.Request) {
	if hk, ok := h.hookFromPath(w, r); ok {
		h.ping(r, hk)
		w.WriteHeader(204)
	}
}

// TestWebhook handles POST /repos/{owner}/{repo}/hooks/{hook_id}/tests,
// which redelivers a push for the default branch to a hook subscribed to
// push.
func (h *Handler) TestWebhook(w http.ResponseWriter, r *http.Request) {
	hk, ok := h.hookFromPath(w, r)
	if !ok {
		return
	}
	rp, _ := h.store.GetRepo(hk.RepoOwner, hk.RepoName)
	if b, found := h.store.GetBranch(hk.RepoOwner, hk.RepoName, rp.DefaultBranch); found && subscribed(hk, "push") {
		c, _ := h.store.GetCommit(hk.RepoOwner, hk.RepoName, b.Commit.SHA)
		before := firstParent(c)
		if before == "" {
			before = zeroSHA
		}
		payload := h.pushPayload(r, hk.RepoOwner, hk.RepoName, "refs/heads/"+b.Name, before, b.Commit.SHA, false)
		h.emit(r, hk.RepoOwner, hk.RepoName, "push", "", payload, hk.ID)
	}
	w.WriteHeader(204)
}

// ListWebhookDeliveries handles GET /repos/{owner}/{repo}/hooks/{hook_id}/deliveries,
// newest first, paged by cursor as GitHub pages it.
func (h *Handler) ListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	hk, ok := h.hookFromPath(w, r)
	if !ok {
		return
	}
	ds := h.store.HookDeliveries.Filter(func(_ string, d store.HookDelivery) bool { return d.HookID == hk.ID })
	sort.Slice(ds, func(i, j int) bool { return ds[i].ID > ds[j].ID })
	if cursor, err := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64); err == nil {
		var after []store.HookDelivery
		for _, d := range ds {
			if d.ID < cursor {
				after = append(after, d)
			}
		}
		ds = after
	}
	_, perPage := pageParams(r)
	if len(ds) > perPage {
		next := fmt.Sprintf("%s%s?per_page=%d&cursor=%d", origin(r), r.URL.Path, perPage, ds[perPage-1].ID)
		w.Header().Set("Link", fmt.Sprintf("<%s>; rel=%q", next, "next"))
		ds = ds[:perPage]
	}
	out := []map[string]any{}
	for _, d := range ds {
		out = append(out, map[string]any{
			"id": d.ID, "guid": d.GUID, "delivered_at": d.DeliveredAt, "redelivery": false, "duration": d.Duration,
			"status": d.Status, "status_code": d.StatusCode, "event": d.Event, "action": nullable(d.Action),
			"installation_id": nullableID(d.InstallationID), "repository_id": nullableID(d.RepositoryID), "throttled_at": nil,
		})
	}
	ghJSON(w, 200, out)
}

func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
