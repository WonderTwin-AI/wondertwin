package api

import (
	"cmp"
	"encoding/base64"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// webhookEventTypes are the event types POST /webhooks accepts, in the order
// Resend lists them (probe o07 of 2026-11-08).
var webhookEventTypes = []string{
	"email.bounced", "email.clicked", "email.complained", "email.delivered",
	"email.delivery_delayed", "email.failed", "email.opened", "email.scheduled",
	"email.sent", "email.suppressed", "email.received",
	"contact.created", "contact.updated", "contact.deleted", "contact.topics.updated",
	"domain.created", "domain.updated", "domain.deleted",
	"suppression.added", "suppression.removed",
	"topic.created", "topic.updated", "topic.deleted",
	"inbox.created", "inbox.updated", "inbox.deleted", "inbox.thread.created",
	"inbox.email.received", "inbox.email.sent", "inbox.thread.folder.updated",
	"inbox.thread.assigned", "inbox.thread.unassigned", "inbox.thread.labels.updated",
	"inbox.draft.created", "inbox.draft.updated", "inbox.draft.sent", "inbox.draft.deleted",
}

// webhookRequest is the body of POST and PATCH /webhooks.
type webhookRequest struct {
	Endpoint *string  `json:"endpoint"`
	Events   []string `json:"events"`
	Status   *string  `json:"status"`
}

// validateWebhookEndpoint returns Resend's message for an endpoint it
// refuses, or "". Resend refuses http (probe o08 of 2026-11-08); the
// emulator accepts it so receivers on a local network need no TLS, and
// refuses any other scheme with Resend's message (divergences.json).
func validateWebhookEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "Endpoint URL scheme must be https"
	}
	return ""
}

// validateWebhookEvents returns Resend's message for the first event type it
// does not know, or "".
func validateWebhookEvents(events []string) string {
	for _, e := range events {
		if !slices.Contains(webhookEventTypes, e) {
			return "`" + e + "` is not a webhook event type. Use one of " + strings.Join(webhookEventTypes, ", ") + "."
		}
	}
	return ""
}

// CreateWebhook handles POST /webhooks. It answers 201 {object, id,
// signing_secret} (probes o01, o07 to o09 of 2026-11-08).
func (h *Handler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	var req webhookRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Endpoint == nil || *req.Endpoint == "" {
		missingRequiredField(w, "endpoint")
		return
	}
	if len(req.Events) == 0 {
		missingRequiredField(w, "events")
		return
	}
	if msg := cmp.Or(validateWebhookEndpoint(*req.Endpoint), validateWebhookEvents(req.Events)); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", msg)
		return
	}

	hook := store.Webhook{
		ID:            h.resendID(),
		CreatedAt:     h.store.Clock.Now().UTC().Format(pgTimestamp),
		Status:        store.WebhookEnabled,
		Endpoint:      *req.Endpoint,
		Events:        slices.Clone(req.Events),
		SigningSecret: h.signingSecret(),
	}
	h.store.Webhooks.Set(hook.ID, hook)
	twincore.JSON(w, http.StatusCreated, map[string]any{
		"object": "webhook", "id": hook.ID, "signing_secret": hook.SigningSecret,
	})
}

// GetWebhook handles GET /webhooks/{id}, which includes the signing secret
// (probe o03 of 2026-11-08, quirk delivery-004).
func (h *Handler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	hook, ok := h.store.Webhooks.Get(chi.URLParam(r, "id"))
	if !ok {
		webhookNotFound(w)
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "webhook", "id": hook.ID, "created_at": hook.CreatedAt, "status": hook.Status,
		"endpoint": hook.Endpoint, "events": hook.Events, "signing_secret": hook.SigningSecret,
	})
}

// listedWebhook is a webhook as GET /webhooks lists it: no object and no
// signing secret (probe o04 of 2026-11-08).
type listedWebhook struct {
	ID        string   `json:"id"`
	CreatedAt string   `json:"created_at"`
	Status    string   `json:"status"`
	Endpoint  string   `json:"endpoint"`
	Events    []string `json:"events"`
}

// ListWebhooks handles GET /webhooks, newest first.
func (h *Handler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	pq, ok := readPageQuery(w, r)
	if !ok {
		return
	}
	hooks := h.store.Webhooks.List()
	items := make([]listedWebhook, len(hooks))
	for i, hook := range hooks {
		items[i] = listedWebhook{ID: hook.ID, CreatedAt: hook.CreatedAt, Status: hook.Status, Endpoint: hook.Endpoint, Events: hook.Events}
	}
	twincore.JSON(w, http.StatusOK, paginate(items, pq, 0, func(l listedWebhook) string { return l.ID }))
}

func webhookNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "not_found", "Webhook endpoint not found")
}

// signingSecret returns a Svix-style secret, whsec_ and the base64 of 24
// bytes from the store seed, so a seeded run repeats it.
func (h *Handler) signingSecret() string {
	key := make([]byte, 24)
	h.store.RandBytes(key)
	return "whsec_" + base64.StdEncoding.EncodeToString(key)
}
