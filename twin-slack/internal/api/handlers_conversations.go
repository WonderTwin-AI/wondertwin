package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// ConversationsList handles POST /api/conversations.list
func (h *Handler) ConversationsList(w http.ResponseWriter, r *http.Request) {
	channels := h.store.Channels.List()
	slackOK(w, map[string]any{
		"channels": channels,
		"response_metadata": map[string]any{
			"next_cursor": "",
		},
	})
}

// ConversationsInfo handles POST /api/conversations.info
func (h *Handler) ConversationsInfo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
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

	slackOK(w, map[string]any{"channel": ch})
}

// ConversationsHistory handles POST /api/conversations.history
func (h *Handler) ConversationsHistory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Limit   int    `json:"limit,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}
	if req.Channel == "" {
		slackError(w, "channel_not_found")
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	messages := h.store.GetChannelMessages(req.Channel, limit)
	slackOK(w, map[string]any{
		"messages": messages,
		"has_more": false,
		"response_metadata": map[string]any{
			"next_cursor": "",
		},
	})
}

// ConversationsReplies handles POST /api/conversations.replies
func (h *Handler) ConversationsReplies(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
		Limit   int    `json:"limit,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	replies := h.store.GetThreadReplies(req.Channel, req.TS, limit)
	slackOK(w, map[string]any{
		"messages": replies,
		"has_more": false,
	})
}

// ConversationsMembers handles POST /api/conversations.members
func (h *Handler) ConversationsMembers(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
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

	slackOK(w, map[string]any{
		"members": ch.Members,
		"response_metadata": map[string]any{
			"next_cursor": "",
		},
	})
}

// ConversationsCreate handles POST /api/conversations.create
func (h *Handler) ConversationsCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		IsPrivate bool   `json:"is_private,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}
	if req.Name == "" {
		slackError(w, "invalid_name_required")
		return
	}

	// Check for duplicate
	if _, _, ok := h.store.GetChannelByName(req.Name); ok {
		slackError(w, "name_taken")
		return
	}

	id := h.store.Channels.NextID()
	ch := store.Channel{
		ID:         id,
		Name:       req.Name,
		IsChannel:  !req.IsPrivate,
		IsGroup:    req.IsPrivate,
		IsPrivate:  req.IsPrivate,
		IsMember:   true,
		Creator:    "U_BOT",
		Created:    h.store.Clock.Now().Unix(),
		Members:    []string{"U_BOT"},
		NumMembers: 1,
	}
	h.store.Channels.Set(id, ch)

	slackOK(w, map[string]any{"channel": ch})
}

// ConversationsArchive handles POST /api/conversations.archive
func (h *Handler) ConversationsArchive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
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
	if ch.IsArchived {
		slackError(w, "already_archived")
		return
	}

	ch.IsArchived = true
	h.store.Channels.Set(req.Channel, ch)
	slackOK(w, nil)
}

// ConversationsUnarchive handles POST /api/conversations.unarchive
func (h *Handler) ConversationsUnarchive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
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
	if !ch.IsArchived {
		slackError(w, "not_archived")
		return
	}

	ch.IsArchived = false
	h.store.Channels.Set(req.Channel, ch)
	slackOK(w, nil)
}

// ConversationsRename handles POST /api/conversations.rename
func (h *Handler) ConversationsRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Name    string `json:"name"`
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

	if _, _, taken := h.store.GetChannelByName(req.Name); taken {
		slackError(w, "name_taken")
		return
	}

	ch.Name = req.Name
	h.store.Channels.Set(req.Channel, ch)
	slackOK(w, map[string]any{"channel": ch})
}

// ConversationsSetPurpose handles POST /api/conversations.setPurpose
func (h *Handler) ConversationsSetPurpose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Purpose string `json:"purpose"`
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

	ch.Purpose = store.Topic{Value: req.Purpose, Creator: "U_BOT", LastSet: h.store.Clock.Now().Unix()}
	h.store.Channels.Set(req.Channel, ch)
	slackOK(w, map[string]any{"purpose": req.Purpose})
}

// ConversationsSetTopic handles POST /api/conversations.setTopic
func (h *Handler) ConversationsSetTopic(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Topic   string `json:"topic"`
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

	ch.Topic = store.Topic{Value: req.Topic, Creator: "U_BOT", LastSet: h.store.Clock.Now().Unix()}
	h.store.Channels.Set(req.Channel, ch)
	slackOK(w, map[string]any{"topic": req.Topic})
}

// ConversationsInvite handles POST /api/conversations.invite
func (h *Handler) ConversationsInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Users   string `json:"users"` // comma-separated
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

	// Add users (simplified — no dedup check)
	for _, u := range splitCSV(req.Users) {
		ch.Members = append(ch.Members, u)
		ch.NumMembers++
	}
	h.store.Channels.Set(req.Channel, ch)
	slackOK(w, map[string]any{"channel": ch})
}

// ConversationsKick handles POST /api/conversations.kick
func (h *Handler) ConversationsKick(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		User    string `json:"user"`
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

	members := make([]string, 0, len(ch.Members))
	found := false
	for _, m := range ch.Members {
		if m == req.User {
			found = true
			continue
		}
		members = append(members, m)
	}
	if !found {
		slackError(w, "not_in_channel")
		return
	}

	ch.Members = members
	ch.NumMembers = len(members)
	h.store.Channels.Set(req.Channel, ch)
	slackOK(w, nil)
}

// ConversationsJoin handles POST /api/conversations.join
func (h *Handler) ConversationsJoin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
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

	if !ch.IsMember {
		ch.Members = append(ch.Members, "U_BOT")
		ch.NumMembers++
		ch.IsMember = true
		h.store.Channels.Set(req.Channel, ch)
	}

	slackOK(w, map[string]any{"channel": ch})
}

// ConversationsLeave handles POST /api/conversations.leave
func (h *Handler) ConversationsLeave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
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

	members := make([]string, 0, len(ch.Members))
	for _, m := range ch.Members {
		if m != "U_BOT" {
			members = append(members, m)
		}
	}
	ch.Members = members
	ch.NumMembers = len(members)
	ch.IsMember = false
	h.store.Channels.Set(req.Channel, ch)
	slackOK(w, nil)
}

// ConversationsOpen handles POST /api/conversations.open
func (h *Handler) ConversationsOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel,omitempty"`
		Users   string `json:"users,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}

	// If channel provided, return it
	if req.Channel != "" {
		ch, ok := h.store.Channels.Get(req.Channel)
		if !ok {
			slackError(w, "channel_not_found")
			return
		}
		slackOK(w, map[string]any{"channel": ch})
		return
	}

	// Create a DM channel for the users
	id := h.store.Channels.NextID()
	ch := store.Channel{
		ID:         id,
		Name:       "dm-" + id,
		IsIM:       true,
		IsMember:   true,
		Creator:    "U_BOT",
		Created:    h.store.Clock.Now().Unix(),
		Members:    append(splitCSV(req.Users), "U_BOT"),
		NumMembers: len(splitCSV(req.Users)) + 1,
	}
	h.store.Channels.Set(id, ch)
	slackOK(w, map[string]any{
		"channel":      ch,
		"no_op":        false,
		"already_open": false,
	})
}

// ConversationsClose handles POST /api/conversations.close
func (h *Handler) ConversationsClose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}
	slackOK(w, nil)
}

// ConversationsMark handles POST /api/conversations.mark
func (h *Handler) ConversationsMark(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}
	slackOK(w, nil)
}

// splitCSV splits a comma-separated string into non-empty trimmed parts.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, p := range splitParts(s) {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func splitParts(s string) []string {
	parts := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
