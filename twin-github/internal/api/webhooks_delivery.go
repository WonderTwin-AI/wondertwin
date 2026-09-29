package api

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // G505: GitHub's legacy X-Hub-Signature header is HMAC-SHA1 by definition
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
)

// GitHub's delivery behaviour, which the dispatcher is configured to match:
// one attempt with no automatic retry, and a delivery that has not finished
// in 10 seconds counts as failed.
const (
	deliveryTimeout = 10 * time.Second
	hookshotAgent   = "GitHub-Hookshot/emulator"
)

// githubSigner produces GitHub's signature headers: X-Hub-Signature-256
// (HMAC-SHA256) and the legacy X-Hub-Signature (HMAC-SHA1), both hex digests
// of the exact body, keyed by the hook's secret.
type githubSigner struct{}

func (githubSigner) Sign(payload []byte, secret string) map[string]string {
	m256 := hmac.New(sha256.New, []byte(secret))
	m256.Write(payload)
	m1 := hmac.New(sha1.New, []byte(secret))
	m1.Write(payload)
	return map[string]string{
		"X-Hub-Signature-256": "sha256=" + hex.EncodeToString(m256.Sum(nil)),
		"X-Hub-Signature":     "sha1=" + hex.EncodeToString(m1.Sum(nil)),
	}
}

type queuedDelivery struct {
	event, action, guid string
	payload             map[string]any
	installationID      int64
	repoID              int64
}

// hookWorker delivers one hook's events in order through a twinkit
// dispatcher.
type hookWorker struct {
	hookID int64
	d      *webhook.Dispatcher
	ch     chan queuedDelivery
	mu     sync.Mutex
	meta   map[string]queuedDelivery // dispatcher event ID -> delivery
	url    string
	secret string
	form   bool
}

// hookBus fans repository events out to the hooks that subscribe to them.
type hookBus struct {
	h       *Handler
	mu      sync.Mutex
	workers map[int64]*hookWorker
	pending sync.WaitGroup
}

func newHookBus(h *Handler) *hookBus { return &hookBus{h: h, workers: map[int64]*hookWorker{}} }

func deliveryGUID(id int64) string {
	b := store.MakeSHA("delivery:" + strconv.FormatInt(id, 10))
	return b[0:8] + "-" + b[8:12] + "-" + b[12:16] + "-" + b[16:20] + "-" + b[20:32]
}

func (b *hookBus) worker(hook store.Webhook) *hookWorker {
	b.mu.Lock()
	defer b.mu.Unlock()
	wk, ok := b.workers[hook.ID]
	if !ok {
		wk = &hookWorker{hookID: hook.ID, ch: make(chan queuedDelivery, 256), meta: map[string]queuedDelivery{}}
		wk.d = webhook.NewDispatcher(webhook.Config{
			Signer:      githubSigner{},
			MaxRetries:  1,
			Timeout:     deliveryTimeout,
			EventPrefix: fmt.Sprintf("hook%d", hook.ID),
			Encode:      wk.encode,
			Headers:     wk.headers,
		})
		b.workers[hook.ID] = wk
		go b.run(wk)
	}
	wk.mu.Lock()
	wk.url, wk.secret, wk.form = hook.Config.URL, hook.Config.Secret, hook.Config.ContentType != "json"
	wk.mu.Unlock()
	wk.d.SetURL(hook.Config.URL)
	wk.d.SetSecret(hook.Config.Secret)
	return wk
}

// encode sends the payload itself, as GitHub does: raw JSON for content
// type json, or payload=<json> for form.
func (wk *hookWorker) encode(evt webhook.Event) ([]byte, error) {
	body, err := json.Marshal(evt.Payload)
	if err != nil {
		return nil, err
	}
	wk.mu.Lock()
	form := wk.form
	wk.mu.Unlock()
	if form {
		return []byte("payload=" + url.QueryEscape(string(body))), nil
	}
	return body, nil
}

func (wk *hookWorker) headers(evt webhook.Event) map[string]string {
	wk.mu.Lock()
	q := wk.meta[evt.ID]
	form := wk.form
	wk.mu.Unlock()
	ctype := "application/json"
	if form {
		ctype = "application/x-www-form-urlencoded"
	}
	return map[string]string{
		"Content-Type":                           ctype,
		"User-Agent":                             hookshotAgent,
		"X-GitHub-Event":                         q.event,
		"X-GitHub-Delivery":                      q.guid,
		"X-GitHub-Hook-ID":                       strconv.FormatInt(wk.hookID, 10),
		"X-GitHub-Hook-Installation-Target-ID":   strconv.FormatInt(q.repoID, 10),
		"X-GitHub-Hook-Installation-Target-Type": "repository",
	}
}

func (b *hookBus) run(wk *hookWorker) {
	for q := range wk.ch {
		b.deliver(wk, q)
		b.pending.Done()
	}
}

func (b *hookBus) deliver(wk *hookWorker, q queuedDelivery) {
	evt := wk.d.Enqueue(q.event, q.payload)
	wk.mu.Lock()
	wk.meta[evt.ID] = q
	wk.mu.Unlock()

	start := time.Now()
	err := wk.d.Flush()
	elapsed := time.Since(start).Seconds()

	code, status := 0, "OK"
	if ds := wk.d.Deliveries(); len(ds) > 0 {
		code = ds[len(ds)-1].StatusCode
	}
	switch {
	case err != nil && code == 0:
		status = "failed to connect to host"
	case err != nil:
		status = fmt.Sprintf("Invalid HTTP Response: %d", code)
	}
	body, _ := wk.encode(evt)
	headers := wk.headers(evt)
	wk.mu.Lock()
	delete(wk.meta, evt.ID)
	wk.mu.Unlock()

	s := b.h.store
	id := s.NewID(store.KindDelivery)
	s.HookDeliveries.Set(strconv.FormatInt(id, 10), store.HookDelivery{
		ID: id, GUID: q.guid, HookID: wk.hookID, DeliveredAt: s.Now(), Duration: elapsed,
		Status: status, StatusCode: code, Event: q.event, Action: q.action,
		InstallationID: q.installationID, RepositoryID: q.repoID,
		RequestHeaders: headers, RequestBody: string(body),
	})
	for _, key := range s.Webhooks.ListIDs() {
		if hk, ok := s.Webhooks.Get(key); ok && hk.ID == wk.hookID {
			msg := status
			if err == nil {
				msg = "OK"
			}
			hk.LastResponse = &store.HookResponse{Code: code, Status: map[bool]string{true: "active", false: "failed"}[err == nil], Message: msg}
			s.Webhooks.Set(key, hk)
		}
	}
}

// subscribed reports whether a hook takes an event.
func subscribed(hook store.Webhook, event string) bool {
	if event == "ping" {
		return true
	}
	for _, e := range hook.Events {
		if e == event || e == "*" {
			return true
		}
	}
	return false
}

// emit queues event for every active hook on the repository that subscribes
// to it. only, when non-zero, limits delivery to one hook (pings and tests).
func (h *Handler) emit(r *http.Request, owner, repo, event, action string, payload map[string]any, only int64) {
	rp, ok := h.store.GetRepo(owner, repo)
	if !ok {
		return
	}
	x := h.rd(r)
	payload["repository"] = x.repo(*rp, "")
	payload["sender"] = x.user(actor(r))
	var instID int64
	if p := principalFrom(r); p.Kind == principalInstallation {
		instID = p.InstallationID
		payload["installation"] = map[string]any{"id": instID,
			"node_id": base64.StdEncoding.EncodeToString([]byte("023:IntegrationInstallation" + strconv.FormatInt(instID, 10)))}
	}
	if action != "" {
		payload["action"] = action
	}
	for _, hook := range h.store.ListRepoWebhooks(owner, repo) {
		if !hook.Active || !subscribed(hook, event) || (only != 0 && hook.ID != only) {
			continue
		}
		p := payload
		if event == "ping" {
			p = map[string]any{}
			for k, v := range payload {
				p[k] = v
			}
			p["hook_id"] = hook.ID
			p["hook"] = x.hook(hook)
		}
		q := queuedDelivery{event: event, action: action, guid: deliveryGUID(h.store.NextID()),
			payload: p, installationID: instID, repoID: rp.ID}
		h.hooks.pending.Add(1)
		h.hooks.worker(hook).ch <- q
	}
}

// FlushWebhooks waits until every queued delivery has been attempted. It
// implements the admin flush, so a test can wait for deliveries to land.
func (h *Handler) FlushWebhooks() error {
	h.hooks.pending.Wait()
	return nil
}

// --- Event payloads ---

func (h *Handler) onIssue(r *http.Request, action string, issue store.Issue) {
	payload := map[string]any{"issue": h.rd(r).issue(issue)}
	if action == "labeled" && len(issue.Labels) > 0 {
		payload["label"] = h.rd(r).label(issue.RepoOwner, issue.RepoName, issue.Labels[len(issue.Labels)-1])
	}
	h.emit(r, issue.RepoOwner, issue.RepoName, "issues", action, payload, 0)
}

func (h *Handler) onPull(r *http.Request, action string, pr store.PullRequest) {
	h.emit(r, pr.RepoOwner, pr.RepoName, "pull_request", action, map[string]any{
		"number": pr.Number, "pull_request": h.rd(r).pull(pr),
	}, 0)
}

func (h *Handler) onCheckRun(r *http.Request, action string, cr store.CheckRun) {
	h.emit(r, cr.RepoOwner, cr.RepoName, "check_run", action, map[string]any{"check_run": h.rd(r).checkRun(cr)}, 0)
}

const zeroSHA = "0000000000000000000000000000000000000000"

func (h *Handler) onRefCreated(r *http.Request, owner, repo, ref, sha string) {
	h.onRefUpdated(r, owner, repo, ref, zeroSHA, sha, false)
}

// onRefUpdated delivers a push for a ref that moved, and synchronize for
// the open pull requests whose head branch it is.
func (h *Handler) onRefUpdated(r *http.Request, owner, repo, ref, before, after string, forced bool) {
	h.emit(r, owner, repo, "push", "", h.pushPayload(r, owner, repo, ref, before, after, forced), 0)
	if before == zeroSHA {
		return
	}
	for _, pr := range h.store.ListRepoPRs(owner, repo, "open") {
		if "refs/heads/"+pr.Head.Ref == ref {
			payload := map[string]any{"number": pr.Number, "pull_request": h.rd(r).pull(pr), "before": before, "after": after}
			h.emit(r, owner, repo, "pull_request", "synchronize", payload, 0)
		}
	}
}

func (h *Handler) pushPayload(r *http.Request, owner, repo, ref, before, after string, forced bool) map[string]any {
	x := h.rd(r)
	stop := before
	if before == zeroSHA {
		stop = ""
	}
	history := h.store.History(owner, repo, after, stop)
	if before == zeroSHA && len(history) > 1 {
		history = history[:1] // a new ref reports only its head commit
	}
	commits := []map[string]any{}
	for i := len(history) - 1; i >= 0; i-- {
		commits = append(commits, h.pushCommit(r, history[i]))
	}
	var head any
	if len(history) > 0 {
		head = h.pushCommit(r, history[0])
	}
	u := h.userRef(actor(r))
	return map[string]any{
		"ref": ref, "before": before, "after": after,
		"created": before == zeroSHA, "deleted": after == zeroSHA, "forced": forced, "base_ref": nil,
		"compare":     x.web("/%s/%s/compare/%s...%s", owner, repo, before[:12], after[:12]),
		"commits":     commits,
		"head_commit": head,
		"pusher":      map[string]any{"name": u.Login, "email": u.Login + "@users.noreply.github.com"},
	}
}

func (h *Handler) pushCommit(r *http.Request, c store.Commit) map[string]any {
	parent, _ := h.store.GetCommit(c.RepoOwner, c.RepoName, firstParent(c))
	added, removed, modified := []string{}, []string{}, []string{}
	for _, fc := range h.diffTrees(c.RepoOwner, c.RepoName, parent.Files, c.Files) {
		switch fc.Status {
		case "added":
			added = append(added, fc.Path)
		case "removed":
			removed = append(removed, fc.Path)
		default:
			modified = append(modified, fc.Path)
		}
	}
	return map[string]any{
		"id": c.SHA, "tree_id": c.TreeSHA, "distinct": true, "message": c.Message, "timestamp": c.AuthorDate,
		"url":       h.rd(r).web("/%s/%s/commit/%s", c.RepoOwner, c.RepoName, c.SHA),
		"author":    map[string]any{"name": c.AuthorName, "email": c.AuthorEmail, "username": c.Login},
		"committer": map[string]any{"name": c.CommitterName, "email": c.CommitterEmail},
		"added":     added, "removed": removed, "modified": modified,
	}
}
