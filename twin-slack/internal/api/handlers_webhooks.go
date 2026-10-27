package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// webhooksPath is where the emulator serves incoming webhook URLs, which on
// Slack are under hooks.slack.com/services/.
const webhooksPath = "/services/"

// webhookURL is the URL an app posts to for wh, under base, the scheme and
// host the client called (see origin). Slack's host is hooks.slack.com, so the
// host here is a declared difference. The path is Slack's documented form
// /services/{team}/{service}/{secret}.
func webhookURL(base string, wh store.IncomingWebhook) string {
	return base + webhooksPath + wh.Team + "/" + wh.Service + "/" + wh.Secret
}

// IncomingWebhookPost handles POST /services/{team}/{service}/{secret}: an
// app posting a message to the one channel the webhook is bound to. Slack
// answers in plain text, HTTP 200 and ok on success, not in the Web API's
// JSON. The post is the webhook's app's, since a webhook carries no token.
//
// What is settled is text, blocks and thread_ts, with attachments implied by
// the too_many_attachments error. Everything else a payload may carry, the
// channel, username and icon a legacy webhook could override, and the fields
// chat.postMessage takes, is ignored, because Slack's pages say only that an
// app's webhook cannot override the channel, username or icon (events-019,
// events-020). The stored message's shape is unverified (events-026).
func (h *Handler) IncomingWebhookPost(w http.ResponseWriter, r *http.Request) {
	wh, ok := h.findWebhook(chi.URLParam(r, "team"), chi.URLParam(r, "service"), chi.URLParam(r, "secret"))
	if !ok {
		webhookError(w, http.StatusNotFound, "no_service")
		return
	}
	var req struct {
		Text        string `json:"text"`
		ThreadTS    string `json:"thread_ts"`
		Blocks      any    `json:"blocks"`
		Attachments any    `json:"attachments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webhookError(w, http.StatusBadRequest, "invalid_payload")
		return
	}
	attachments, ok := messageAttachments(req.Attachments)
	if !ok {
		webhookError(w, http.StatusBadRequest, "invalid_payload")
		return
	}
	if _, code := messageText(req.Text, req.Blocks, req.Attachments, ""); code != "" {
		webhookError(w, http.StatusBadRequest, code)
		return
	}
	ch, ok := h.store.Channels.Get(wh.Channel)
	if !ok {
		webhookError(w, http.StatusNotFound, "channel_not_found")
		return
	}
	if ch.IsArchived {
		webhookError(w, http.StatusGone, "channel_is_archived")
		return
	}

	msg := store.Message{
		Type:        "message",
		Subtype:     "bot_message",
		Channel:     wh.Channel,
		Text:        req.Text,
		TS:          h.store.NextTS(),
		ThreadTS:    req.ThreadTS,
		Team:        wh.Team,
		Blocks:      req.Blocks,
		Attachments: attachments,
		BotID:       wh.BotID,
		AppID:       wh.AppID,
	}
	if p := h.store.BotProfileFor(wh.BotID); p != nil {
		msg.BotProfile = p
		msg.Username = p.Name
		msg.User = p.UserID
	}
	h.store.Messages.Set(h.store.Messages.NextID(), msg)
	h.emitMessage(msg)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("ok"))
}

// findWebhook finds the webhook a URL names. The secret is compared whole.
func (h *Handler) findWebhook(team, service, secret string) (store.IncomingWebhook, bool) {
	for _, wh := range h.store.IncomingWebhooks.List() {
		if wh.Team == team && wh.Service == service && wh.Secret == secret {
			return wh, true
		}
	}
	return store.IncomingWebhook{}, false
}

// webhookError answers in plain text with the error string as the body, as
// Slack's webhook docs describe. Slack's changelog gives 400 for invalid_payload
// and 404 for channel_not_found, and 410 for channel_is_archived. The statuses
// for no_service and no_text are not documented; 404 is chosen for the first
// because Slack answered a made-up team with 404, and 400 for the second as
// the status of the other payload errors (events-022, events-028).
func webhookError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(code))
}
