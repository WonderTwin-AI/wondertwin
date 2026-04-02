package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// ChatPostMessage handles POST /api/chat.postMessage
func (h *Handler) ChatPostMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel  string `json:"channel"`
		Text     string `json:"text"`
		ThreadTS string `json:"thread_ts,omitempty"`
		Blocks   any    `json:"blocks,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}
	if req.Channel == "" {
		slackError(w, "channel_not_found")
		return
	}
	if req.Text == "" && req.Blocks == nil {
		slackError(w, "no_text")
		return
	}

	// Verify channel exists
	if _, ok := h.store.Channels.Get(req.Channel); !ok {
		slackError(w, "channel_not_found")
		return
	}

	ts := h.store.NextTS()
	msg := store.Message{
		Type:     "message",
		Channel:  req.Channel,
		User:     "U_BOT",
		Text:     req.Text,
		TS:       ts,
		ThreadTS: req.ThreadTS,
		Team:     h.store.Team.ID,
		Blocks:   req.Blocks,
	}

	id := h.store.Messages.NextID()
	h.store.Messages.Set(id, msg)

	slackOK(w, map[string]any{
		"channel": req.Channel,
		"ts":      ts,
		"message": msg,
	})
}

// ChatPostEphemeral handles POST /api/chat.postEphemeral
func (h *Handler) ChatPostEphemeral(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		User    string `json:"user"`
		Text    string `json:"text"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}
	if req.Channel == "" {
		slackError(w, "channel_not_found")
		return
	}
	if req.User == "" {
		slackError(w, "user_not_found")
		return
	}

	ts := h.store.NextTS()
	slackOK(w, map[string]any{
		"message_ts": ts,
	})
}

// ChatUpdate handles POST /api/chat.update
func (h *Handler) ChatUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
		Text    string `json:"text"`
		Blocks  any    `json:"blocks,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}

	msg, id, ok := h.store.GetMessageByTS(req.Channel, req.TS)
	if !ok {
		slackError(w, "message_not_found")
		return
	}

	msg.Text = req.Text
	if req.Blocks != nil {
		msg.Blocks = req.Blocks
	}
	editTS := h.store.NextTS()
	msg.Edited = &store.MessageEdit{User: "U_BOT", TS: editTS}
	h.store.Messages.Set(id, *msg)

	slackOK(w, map[string]any{
		"channel": req.Channel,
		"ts":      req.TS,
		"text":    msg.Text,
		"message": msg,
	})
}

// ChatDelete handles POST /api/chat.delete
func (h *Handler) ChatDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}

	msg, id, ok := h.store.GetMessageByTS(req.Channel, req.TS)
	if !ok {
		slackError(w, "message_not_found")
		return
	}

	msg.IsDeleted = true
	h.store.Messages.Set(id, *msg)

	slackOK(w, map[string]any{
		"channel": req.Channel,
		"ts":      req.TS,
	})
}

// ChatGetPermalink handles POST /api/chat.getPermalink
func (h *Handler) ChatGetPermalink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel   string `json:"channel"`
		MessageTS string `json:"message_ts"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}

	ch, ok := h.store.Channels.Get(req.Channel)
	if !ok {
		slackError(w, "channel_not_found")
		return
	}

	permalink := "https://" + h.store.Team.Domain + ".slack.com/archives/" + ch.ID + "/p" + req.MessageTS
	slackOK(w, map[string]any{
		"channel":   req.Channel,
		"permalink": permalink,
	})
}

// ChatScheduleMessage handles POST /api/chat.scheduleMessage
func (h *Handler) ChatScheduleMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Text    string `json:"text"`
		PostAt  int64  `json:"post_at"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}
	if req.Channel == "" {
		slackError(w, "channel_not_found")
		return
	}

	id := h.store.ScheduledMessages.NextID()
	sm := store.ScheduledMessage{
		ID:          id,
		Channel:     req.Channel,
		Text:        req.Text,
		PostAt:      req.PostAt,
		DateCreated: h.store.Clock.Now().Unix(),
	}
	h.store.ScheduledMessages.Set(id, sm)

	slackOK(w, map[string]any{
		"channel":              req.Channel,
		"scheduled_message_id": id,
		"post_at":              req.PostAt,
	})
}

// ChatDeleteScheduledMessage handles POST /api/chat.deleteScheduledMessage
func (h *Handler) ChatDeleteScheduledMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel            string `json:"channel"`
		ScheduledMessageID string `json:"scheduled_message_id"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}

	if _, ok := h.store.ScheduledMessages.Get(req.ScheduledMessageID); !ok {
		slackError(w, "invalid_scheduled_message_id")
		return
	}
	h.store.ScheduledMessages.Delete(req.ScheduledMessageID)
	slackOK(w, nil)
}

// ChatScheduledMessagesList handles POST /api/chat.scheduledMessages.list
func (h *Handler) ChatScheduledMessagesList(w http.ResponseWriter, r *http.Request) {
	msgs := h.store.ScheduledMessages.List()
	slackOK(w, map[string]any{
		"scheduled_messages": msgs,
	})
}

// ChatMeMessage handles POST /api/chat.meMessage
func (h *Handler) ChatMeMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Text    string `json:"text"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}

	ts := h.store.NextTS()
	msg := store.Message{
		Type:    "message",
		Subtype: "me_message",
		Channel: req.Channel,
		User:    "U_BOT",
		Text:    req.Text,
		TS:      ts,
		Team:    h.store.Team.ID,
	}
	id := h.store.Messages.NextID()
	h.store.Messages.Set(id, msg)

	slackOK(w, map[string]any{
		"channel": req.Channel,
		"ts":      ts,
	})
}
