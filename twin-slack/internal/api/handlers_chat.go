package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// ChatPostMessage handles POST /api/chat.postMessage
func (h *Handler) ChatPostMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel  string `json:"channel"`
		Text     string `json:"text"`
		ThreadTS string `json:"thread_ts,omitempty"`
		Blocks   any    `json:"blocks,omitempty"`
		// Attachments and Metadata arrive as JSON, or as JSON text in a form.
		Attachments    any    `json:"attachments,omitempty"`
		Metadata       any    `json:"metadata,omitempty"`
		Username       string `json:"username,omitempty"`
		IconEmoji      string `json:"icon_emoji,omitempty"`
		ReplyBroadcast bool   `json:"reply_broadcast,omitempty"`
		MarkdownText   string `json:"markdown_text,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if req.Channel == "" {
		slackError(w, "channel_not_found")
		return
	}
	text, code := messageText(req.Text, req.Blocks, req.Attachments, req.MarkdownText)
	if code != "" {
		slackError(w, code)
		return
	}
	attachments, ok := messageAttachments(req.Attachments)
	if !ok {
		slackError(w, "invalid_arguments")
		return
	}
	if len(attachments) > maxAttachments {
		slackError(w, "too_many_attachments")
		return
	}
	metadata, ok := messageMetadata(req.Metadata)
	if !ok {
		slackError(w, "invalid_metadata_format")
		return
	}

	ch, ok := h.store.Channels.Get(req.Channel)
	if !ok {
		slackError(w, "channel_not_found")
		return
	}
	if ch.IsArchived {
		slackError(w, "is_archived")
		return
	}

	ts := h.store.NextTS()
	msg := h.authored(r, store.Message{
		Type:     "message",
		Channel:  req.Channel,
		Text:     text,
		TS:       ts,
		ThreadTS: req.ThreadTS,
		Team:     h.store.Team.ID,
		Blocks:   req.Blocks,

		Username:    req.Username,
		Attachments: attachments,
		Metadata:    metadata,
	})
	if req.IconEmoji != "" {
		msg.Icons = map[string]string{"emoji": req.IconEmoji}
	}
	// A broadcast reply is also shown in the channel.
	if req.ReplyBroadcast && req.ThreadTS != "" {
		msg.Subtype = "thread_broadcast"
	}

	id := h.store.Messages.NextID()
	h.store.Messages.Set(id, msg)
	h.emitMessage(msg)

	slackOK(w, map[string]any{
		"channel": req.Channel,
		"ts":      ts,
		"message": msg,
	})
}

// ChatPostEphemeral handles POST /api/chat.postEphemeral
func (h *Handler) ChatPostEphemeral(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel      string `json:"channel"`
		User         string `json:"user"`
		Text         string `json:"text"`
		Blocks       any    `json:"blocks,omitempty"`
		Attachments  any    `json:"attachments,omitempty"`
		MarkdownText string `json:"markdown_text,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if req.Channel == "" {
		slackError(w, "channel_not_found")
		return
	}
	if _, code := messageText(req.Text, req.Blocks, req.Attachments, req.MarkdownText); code != "" {
		slackError(w, code)
		return
	}
	if req.User == "" {
		slackError(w, "user_not_found")
		return
	}
	if _, ok := h.store.Channels.Get(req.Channel); !ok {
		slackError(w, "channel_not_found")
		return
	}

	ts := h.store.NextTS()
	slackOK(w, map[string]any{
		"message_ts": ts,
	})
}

// ChatUpdate handles POST /api/chat.update. Only the author of a message may
// update it, a bot its own posts included. Given text and no blocks, the
// message's blocks are dropped; attachments and metadata are kept unless the
// call replaces them, and an empty array or object removes them.
func (h *Handler) ChatUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel      string `json:"channel"`
		TS           string `json:"ts"`
		Text         string `json:"text"`
		Blocks       any    `json:"blocks,omitempty"`
		Attachments  any    `json:"attachments,omitempty"`
		Metadata     any    `json:"metadata,omitempty"`
		MarkdownText string `json:"markdown_text,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	ch, ok := h.store.Channels.Get(req.Channel)
	if !ok {
		slackError(w, "channel_not_found")
		return
	}
	msg, id, ok := h.store.GetMessageByTS(req.Channel, req.TS)
	if !ok {
		slackError(w, "message_not_found")
		return
	}
	if msg.User != callerUserID(r) {
		slackError(w, "cant_update_message")
		return
	}
	if ch.IsArchived {
		slackError(w, "is_inactive")
		return
	}
	text, code := messageText(req.Text, req.Blocks, req.Attachments, req.MarkdownText)
	if code != "" {
		slackError(w, code)
		return
	}
	attachments, ok := messageAttachments(req.Attachments)
	if !ok {
		slackError(w, "invalid_attachments")
		return
	}
	if len(attachments) > maxAttachments {
		slackError(w, "too_many_attachments")
		return
	}

	msg.Text = text
	switch {
	case req.Blocks != nil:
		msg.Blocks = req.Blocks
		if b, isList := req.Blocks.([]any); isList && len(b) == 0 {
			msg.Blocks = nil
		}
	case text != "":
		msg.Blocks = nil
	}
	if req.Attachments != nil {
		msg.Attachments = attachments
		if len(attachments) == 0 {
			msg.Attachments = nil
		}
	}
	if req.Metadata != nil {
		if obj, isObj := req.Metadata.(map[string]any); isObj && len(obj) == 0 {
			msg.Metadata = nil
		} else if metadata, ok := messageMetadata(req.Metadata); ok {
			msg.Metadata = metadata
		} else {
			slackError(w, "invalid_metadata_format")
			return
		}
	}
	editTS := h.store.NextTS()
	msg.Edited = &store.MessageEdit{User: callerUserID(r), TS: editTS}
	h.store.Messages.Set(id, *msg)

	slackOK(w, map[string]any{
		"channel": req.Channel,
		"ts":      req.TS,
		"text":    msg.Text,
		"message": msg,
	})
}

// ChatDelete handles POST /api/chat.delete. A bot token deletes only the
// bot's own messages; a user token deletes the user's own, and an admin or
// owner deletes anyone's.
func (h *Handler) ChatDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	if _, ok := h.store.Channels.Get(req.Channel); !ok {
		slackError(w, "channel_not_found")
		return
	}
	msg, id, ok := h.store.GetMessageByTS(req.Channel, req.TS)
	if !ok {
		slackError(w, "message_not_found")
		return
	}
	if !h.mayDelete(principal(r), *msg) {
		slackError(w, "cant_delete_message")
		return
	}

	msg.IsDeleted = true
	h.store.Messages.Set(id, *msg)
	h.emitMessageDeleted(req.Channel, req.TS)

	slackOK(w, map[string]any{
		"channel": req.Channel,
		"ts":      req.TS,
	})
}

func (h *Handler) mayDelete(t store.Token, msg store.Message) bool {
	if msg.User == t.UserID {
		return true
	}
	if t.Type != store.TokenUser {
		return false
	}
	u, _ := h.store.Users.Get(t.UserID)
	return u.IsAdmin || u.IsOwner
}

// ChatGetPermalink handles POST /api/chat.getPermalink
func (h *Handler) ChatGetPermalink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel   string `json:"channel"`
		MessageTS string `json:"message_ts"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	ch, ok := h.store.Channels.Get(req.Channel)
	if !ok {
		slackError(w, "channel_not_found")
		return
	}

	msg, _, ok := h.store.GetMessageByTS(req.Channel, req.MessageTS)
	if !ok {
		slackError(w, "message_not_found")
		return
	}

	// Slack's permalink is p followed by the ts without its dot; a reply adds
	// its thread and channel.
	permalink := "https://" + h.store.Team.Domain + ".slack.com/archives/" + ch.ID + "/p" + strings.ReplaceAll(req.MessageTS, ".", "")
	if msg.ThreadTS != "" && msg.ThreadTS != msg.TS {
		permalink += "?thread_ts=" + msg.ThreadTS + "&cid=" + ch.ID
	}
	slackOK(w, map[string]any{
		"channel":   req.Channel,
		"permalink": permalink,
	})
}

// maxScheduleAhead is how far ahead a message may be scheduled: 120 days
// (time_too_far).
const maxScheduleAhead = 120 * 24 * 60 * 60

// ChatScheduleMessage handles POST /api/chat.scheduleMessage. The message is
// posted when post_at passes on the app emulator's clock, by the next call
// that reaches it. A message that carries metadata is accepted but never
// posted, as the docs' bug alert says of Slack.
func (h *Handler) ChatScheduleMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel        string `json:"channel"`
		Text           string `json:"text"`
		PostAt         int64  `json:"post_at"`
		Blocks         any    `json:"blocks,omitempty"`
		Attachments    any    `json:"attachments,omitempty"`
		Metadata       any    `json:"metadata,omitempty"`
		ThreadTS       string `json:"thread_ts,omitempty"`
		ReplyBroadcast bool   `json:"reply_broadcast,omitempty"`
		MarkdownText   string `json:"markdown_text,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	ch, ok := h.store.Channels.Get(req.Channel)
	if !ok {
		slackError(w, "channel_not_found")
		return
	}
	if ch.IsArchived {
		slackError(w, "is_archived")
		return
	}
	text, code := messageText(req.Text, req.Blocks, req.Attachments, req.MarkdownText)
	if code != "" {
		slackError(w, code)
		return
	}
	now := h.store.Clock.Now().Unix()
	switch {
	case req.PostAt <= 0:
		slackError(w, "invalid_time")
		return
	case req.PostAt <= now:
		slackError(w, "time_in_past")
		return
	case req.PostAt > now+maxScheduleAhead:
		slackError(w, "time_too_far")
		return
	}
	attachments, ok := messageAttachments(req.Attachments)
	if !ok {
		slackError(w, "invalid_arguments")
		return
	}
	if len(attachments) > maxAttachments {
		slackError(w, "too_many_attachments")
		return
	}
	metadata, ok := messageMetadata(req.Metadata)
	if !ok {
		slackError(w, "invalid_metadata_format")
		return
	}

	msg := h.authored(r, store.Message{
		Type:        "message",
		Channel:     req.Channel,
		Text:        text,
		ThreadTS:    req.ThreadTS,
		Team:        h.store.Team.ID,
		Blocks:      req.Blocks,
		Attachments: attachments,
		Metadata:    metadata,
	})
	if req.ReplyBroadcast && req.ThreadTS != "" {
		msg.Subtype = "thread_broadcast"
	}
	id := h.store.ScheduledMessages.NextID()
	h.store.ScheduledMessages.Set(id, store.ScheduledMessage{
		ID:          id,
		Channel:     req.Channel,
		Text:        text,
		PostAt:      req.PostAt,
		DateCreated: now,
		Token:       principal(r).Token,
		Message:     msg,
		Hold:        metadata != nil,
	})

	// The answer shows the message as it waits, a delayed_message.
	pending := msg
	pending.Type = "delayed_message"
	pending.Channel = ""
	slackOK(w, map[string]any{
		"channel":              req.Channel,
		"scheduled_message_id": id,
		"post_at":              req.PostAt,
		"message":              pending,
	})
}

// deliverScheduled posts every scheduled message whose post_at has passed on
// the app emulator's clock, oldest first, and emits it as a message event.
func (h *Handler) deliverScheduled() {
	h.deliverMu.Lock()
	defer h.deliverMu.Unlock()
	now := h.store.Clock.Now().Unix()
	due := h.store.ScheduledMessages.Filter(func(_ string, sm store.ScheduledMessage) bool {
		return sm.PostAt <= now && !sm.Hold
	})
	sort.SliceStable(due, func(i, j int) bool { return due[i].PostAt < due[j].PostAt })
	for _, sm := range due {
		h.store.ScheduledMessages.Delete(sm.ID)
		if _, ok := h.store.Channels.Get(sm.Channel); !ok {
			continue
		}
		msg := sm.Message
		msg.Channel = sm.Channel
		msg.TS = h.store.NextTS()
		h.store.Messages.Set(h.store.Messages.NextID(), msg)
		h.emitMessage(msg)
	}
}

// deliverDue is middleware that posts due scheduled messages before a call
// is handled, so the call sees them.
func (h *Handler) deliverDue(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.deliverScheduled()
		next.ServeHTTP(w, r)
	})
}

// ChatDeleteScheduledMessage handles POST /api/chat.deleteScheduledMessage
func (h *Handler) ChatDeleteScheduledMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel            string `json:"channel"`
		ScheduledMessageID string `json:"scheduled_message_id"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if _, ok := h.store.Channels.Get(req.Channel); !ok {
		slackError(w, "channel_not_found")
		return
	}
	sm, ok := h.store.ScheduledMessages.Get(req.ScheduledMessageID)
	if !ok || sm.Channel != req.Channel {
		slackError(w, "invalid_scheduled_message_id")
		return
	}
	h.store.ScheduledMessages.Delete(req.ScheduledMessageID)
	slackOK(w, nil)
}

// pageScheduledMessages pages the scheduled list by message ID.
var pageScheduledMessages = pageSpec{kind: "scheduled", def: 100, max: 1000}

// ChatScheduledMessagesList handles POST /api/chat.scheduledMessages.list. It
// lists the pending messages the calling token scheduled, in channel when
// given, with post_at between oldest and latest.
func (h *Handler) ChatScheduledMessagesList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Cursor  string `json:"cursor"`
		Limit   int    `json:"limit"`
		Oldest  string `json:"oldest"`
		Latest  string `json:"latest"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if req.Channel != "" {
		if _, ok := h.store.Channels.Get(req.Channel); !ok {
			slackError(w, "invalid_channel")
			return
		}
	}
	oldest, okOldest := unixBound(req.Oldest)
	latest, okLatest := unixBound(req.Latest)
	if !okOldest || !okLatest {
		slackError(w, "invalid_arguments")
		return
	}
	token := principal(r).Token
	pending := h.store.ScheduledMessages.Filter(func(_ string, sm store.ScheduledMessage) bool {
		return sm.Token == token &&
			(req.Channel == "" || sm.Channel == req.Channel) &&
			(oldest == 0 || sm.PostAt >= oldest) &&
			(latest == 0 || sm.PostAt <= latest)
	})
	page, next, err := pageOf(pending, func(sm store.ScheduledMessage) string { return sm.ID },
		pageScheduledMessages, req.Cursor, req.Limit)
	if err != nil {
		slackError(w, "invalid_cursor")
		return
	}
	items := make([]map[string]any, len(page))
	for i, sm := range page {
		items[i] = map[string]any{
			"id": sm.ID, "channel_id": sm.Channel, "post_at": sm.PostAt,
			"date_created": sm.DateCreated, "text": sm.Text,
		}
	}
	slackOK(w, map[string]any{
		"scheduled_messages": items,
		"response_metadata":  map[string]any{"next_cursor": next},
	})
}

// unixBound reads an optional Unix timestamp bound. It reports false for one
// that is not a timestamp.
func unixBound(s string) (int64, bool) {
	if s == "" {
		return 0, true
	}
	if !isTimestamp(s) {
		return 0, false
	}
	sec, _, _ := strings.Cut(s, ".")
	n, err := strconv.ParseInt(sec, 10, 64)
	return n, err == nil
}

// ChatMeMessage handles POST /api/chat.meMessage
func (h *Handler) ChatMeMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Text    string `json:"text"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if _, ok := h.store.Channels.Get(req.Channel); !ok {
		slackError(w, "channel_not_found")
		return
	}
	if req.Text == "" {
		slackError(w, "no_text")
		return
	}

	ts := h.store.NextTS()
	msg := store.Message{
		Type:    "message",
		Subtype: "me_message",
		Channel: req.Channel,
		User:    callerUserID(r),
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

// maxAttachments is the most attachments a message may carry, per the
// chat.postMessage docs for too_many_attachments.
const maxAttachments = 100

// messageAttachments reads the attachments argument: a JSON array of objects.
// Each attachment gets the 1-based id Slack's response example shows.
func messageAttachments(v any) ([]map[string]any, bool) {
	if v == nil {
		return nil, true
	}
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]map[string]any, 0, len(items))
	for i, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		if _, set := obj["id"]; !set {
			obj["id"] = i + 1
		}
		out = append(out, obj)
	}
	return out, true
}

// messageMetadata reads the metadata argument: an object with a string
// event_type and an object event_payload.
func messageMetadata(v any) (map[string]any, bool) {
	if v == nil {
		return nil, true
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	if t, ok := obj["event_type"].(string); !ok || t == "" {
		return nil, false
	}
	if _, ok := obj["event_payload"].(map[string]any); !ok {
		return nil, false
	}
	return obj, true
}

// messageText checks a message's content the way chat.postMessage and its
// siblings do. markdown_text stands alone: with text or blocks it is
// markdown_text_conflict. A message needs text, blocks, attachments or
// markdown_text, or it is no_text. The text kept is the text, or the
// markdown as given: Slack's rendering of markdown into blocks is not
// reproduced.
func messageText(text string, blocks, attachments any, markdown string) (string, string) {
	if markdown != "" {
		if text != "" || blocks != nil {
			return "", "markdown_text_conflict"
		}
		return markdown, ""
	}
	if text == "" && blocks == nil && attachments == nil {
		return "", "no_text"
	}
	return text, ""
}

// authored sets who a message is from: the token's user, and for a bot token
// the bot, its app and its bot_profile, as Slack shows a bot's post.
func (h *Handler) authored(r *http.Request, msg store.Message) store.Message {
	t := principal(r)
	msg.User = callerUserID(r)
	if t.Type == store.TokenBot && t.BotID != "" {
		msg.BotID = t.BotID
		if p := h.store.BotProfileFor(t.BotID); p != nil {
			msg.AppID = p.AppID
			msg.BotProfile = p
		}
	}
	return msg
}
