package api

import (
	"crypto/subtle"
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
	wh, code := h.findWebhook(chi.URLParam(r, "team"), chi.URLParam(r, "service"), chi.URLParam(r, "secret"))
	if code != "" {
		webhookError(w, code)
		return
	}
	var req struct {
		Text        string `json:"text"`
		ThreadTS    string `json:"thread_ts"`
		Blocks      any    `json:"blocks"`
		Attachments any    `json:"attachments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webhookError(w, "invalid_payload")
		return
	}
	attachments, ok := messageAttachments(req.Attachments)
	if !ok {
		webhookError(w, "invalid_payload")
		return
	}
	if len(attachments) > maxAttachments {
		webhookError(w, "too_many_attachments")
		return
	}
	if _, code := messageText(req.Text, req.Blocks, req.Attachments, ""); code != "" {
		webhookError(w, code)
		return
	}
	ch, ok := h.store.Channels.Get(wh.Channel)
	if !ok {
		webhookError(w, "channel_not_found")
		return
	}
	if ch.IsArchived {
		webhookError(w, "channel_is_archived")
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

// findWebhook finds the webhook a URL names, or the error the URL earns: a
// team that is not this workspace is no_team, a service that names no webhook
// is no_service, and a secret that does not match is invalid_token. The order
// of these checks, and which string Slack answers for each, is unverified
// (events-028, events-030).
func (h *Handler) findWebhook(team, service, secret string) (store.IncomingWebhook, string) {
	if team != h.store.Team.ID {
		return store.IncomingWebhook{}, "no_team"
	}
	for _, wh := range h.store.IncomingWebhooks.List() {
		if wh.Team != team || wh.Service != service {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(wh.Secret), []byte(secret)) != 1 {
			return store.IncomingWebhook{}, "invalid_token"
		}
		return wh, ""
	}
	return store.IncomingWebhook{}, "no_service"
}

// webhookStatus is the HTTP status each error string is answered with. Slack's
// 2016 changelog gives 400 invalid_payload, 403 action_prohibited, 404
// channel_not_found, 410 channel_is_archived and 500 rollup_error. A made-up
// team drew 404 no_team from Slack in a probe. The rest are not documented and
// are chosen here (events-028).
var webhookStatus = map[string]int{
	"no_team":              http.StatusNotFound,
	"no_service":           http.StatusNotFound,
	"invalid_token":        http.StatusForbidden,
	"invalid_payload":      http.StatusBadRequest,
	"no_text":              http.StatusBadRequest,
	"too_many_attachments": http.StatusBadRequest,
	"channel_not_found":    http.StatusNotFound,
	"channel_is_archived":  http.StatusGone,
}

// webhookError answers with the error string as the body. Slack's one observed
// error answer, a 404 no_team, was text/html and 7 bytes, so errors carry
// text/html and no newline; the success answer's content type was not captured
// (events-029).
func webhookError(w http.ResponseWriter, code string) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(webhookStatus[code])
	w.Write([]byte(code))
}
