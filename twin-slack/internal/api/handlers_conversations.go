package api

import (
	"net/http"
	"slices"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// ConversationsList handles POST /api/conversations.list
func (h *Handler) ConversationsList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cursor string `json:"cursor"`
		Limit  int    `json:"limit"`
		Types  string `json:"types"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	types, ok := conversationTypes(req.Types)
	if !ok {
		slackError(w, "invalid_types")
		return
	}
	caller := callerUserID(r)
	listed := h.store.Channels.Filter(func(_ string, ch store.Channel) bool {
		return types[conversationType(ch)] && visibleTo(ch, caller)
	})
	channels, next, err := pageOf(listed, func(c store.Channel) string { return c.ID },
		pageConversationsList, req.Cursor, req.Limit)
	if err != nil {
		slackError(w, "invalid_cursor")
		return
	}
	slackOK(w, map[string]any{
		"channels":          channels,
		"response_metadata": map[string]any{"next_cursor": next},
	})
}

// ConversationsInfo handles POST /api/conversations.info
func (h *Handler) ConversationsInfo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
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

	slackOK(w, map[string]any{"channel": ch})
}

// ConversationsHistory handles POST /api/conversations.history
func (h *Handler) ConversationsHistory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Cursor  string `json:"cursor"`
		Limit   int    `json:"limit"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if req.Channel == "" {
		slackError(w, "channel_not_found")
		return
	}

	messages, next, err := pageOf(h.store.GetChannelMessages(req.Channel, 0), func(m store.Message) string { return m.TS },
		pageConversationsHistory, req.Cursor, req.Limit)
	if err != nil {
		slackError(w, "invalid_cursor")
		return
	}
	slackOK(w, map[string]any{
		"messages":          messages,
		"has_more":          next != "",
		"response_metadata": map[string]any{"next_cursor": next},
	})
}

// ConversationsReplies handles POST /api/conversations.replies
func (h *Handler) ConversationsReplies(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
		Cursor  string `json:"cursor"`
		Limit   int    `json:"limit"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	replies, next, err := pageOf(h.store.GetThreadReplies(req.Channel, req.TS, 0), func(m store.Message) string { return m.TS },
		pageConversationsReplies, req.Cursor, req.Limit)
	if err != nil {
		slackError(w, "invalid_cursor")
		return
	}
	slackOK(w, map[string]any{
		"messages":          replies,
		"has_more":          next != "",
		"response_metadata": map[string]any{"next_cursor": next},
	})
}

// ConversationsMembers handles POST /api/conversations.members
func (h *Handler) ConversationsMembers(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Cursor  string `json:"cursor"`
		Limit   int    `json:"limit"`
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

	members, next, err := pageOf(ch.Members, func(id string) string { return id },
		pageConversationsMembers, req.Cursor, req.Limit)
	if err != nil {
		slackError(w, "invalid_cursor")
		return
	}
	slackOK(w, map[string]any{
		"members":           members,
		"response_metadata": map[string]any{"next_cursor": next},
	})
}

// ConversationsCreate handles POST /api/conversations.create
func (h *Handler) ConversationsCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		IsPrivate bool   `json:"is_private,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
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
		slackArgsError(w, err)
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
		slackArgsError(w, err)
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
		slackArgsError(w, err)
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
		slackArgsError(w, err)
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
		slackArgsError(w, err)
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
		slackArgsError(w, err)
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
		slackArgsError(w, err)
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
		slackArgsError(w, err)
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
		slackArgsError(w, err)
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
		Channel         string `json:"channel,omitempty"`
		Users           string `json:"users,omitempty"`
		ReturnIM        bool   `json:"return_im,omitempty"`
		PreventCreation bool   `json:"prevent_creation,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	// Resuming by channel works only for a direct or multi-person message.
	if req.Channel != "" {
		ch, ok := h.store.Channels.Get(req.Channel)
		if !ok {
			slackError(w, "channel_not_found")
			return
		}
		if !ch.IsIM && !ch.IsMPIM {
			slackError(w, "method_not_supported_for_channel_type")
			return
		}
		h.openAnswer(w, ch, true, req.ReturnIM)
		return
	}

	users := splitCSV(req.Users)
	if len(users) == 0 {
		slackError(w, "users_list_not_supplied")
		return
	}
	if len(users) > 8 {
		slackError(w, "too_many_users")
		return
	}
	for _, u := range users {
		if !h.store.KnownUser(u) {
			slackError(w, "user_not_found")
			return
		}
		if user, ok := h.store.Users.Get(u); ok && user.Deleted {
			slackError(w, "user_disabled")
			return
		}
	}

	// The caller is a member without being named, so the same set of users
	// always resumes the same conversation.
	caller := principal(r).UserID
	if caller == "" {
		caller = store.DefaultBotUserID
	}
	members := memberSet(append(users, caller))
	if ch, ok := h.conversationWith(members); ok {
		h.openAnswer(w, ch, true, req.ReturnIM)
		return
	}
	if req.PreventCreation {
		slackOK(w, map[string]any{"no_op": true, "already_open": false})
		return
	}

	id := h.store.Channels.NextID()
	ch := store.Channel{
		IsMember:   true,
		Creator:    caller,
		Created:    h.store.Clock.Now().Unix(),
		Members:    members,
		NumMembers: len(members),
	}
	if len(members) <= 2 {
		// A direct message has a D id and names the other user, as Slack's
		// IM objects do; a message to oneself names the caller.
		ch.ID = "D" + strings.TrimPrefix(id, "C")
		ch.IsIM = true
		ch.User = caller
		for _, m := range members {
			if m != caller {
				ch.User = m
			}
		}
	} else {
		ch.ID = id
		ch.IsMPIM = true
		ch.IsPrivate = true
		ch.Name = h.mpimName(members)
	}
	h.store.Channels.Set(ch.ID, ch)
	h.openAnswer(w, ch, false, req.ReturnIM)
}

// openAnswer writes the answer to conversations.open. Without return_im, the
// channel is answered with its id only, as the docs describe.
func (h *Handler) openAnswer(w http.ResponseWriter, ch store.Channel, existing, returnIM bool) {
	fields := map[string]any{"channel": map[string]any{"id": ch.ID}}
	if returnIM {
		fields["channel"] = ch
	}
	if existing {
		fields["no_op"] = true
		fields["already_open"] = true
	}
	slackOK(w, fields)
}

// conversationWith finds the direct or multi-person message whose members are
// exactly members, which memberSet has sorted.
func (h *Handler) conversationWith(members []string) (store.Channel, bool) {
	found := h.store.Channels.Filter(func(_ string, ch store.Channel) bool {
		return (ch.IsIM || ch.IsMPIM) && slices.Equal(memberSet(ch.Members), members)
	})
	if len(found) == 0 {
		return store.Channel{}, false
	}
	return found[0], true
}

// memberSet sorts and de-duplicates user ids.
func memberSet(ids []string) []string {
	out := slices.Clone(ids)
	slices.Sort(out)
	return slices.Compact(out)
}

// mpimName follows Slack's naming for a multi-person message:
// mpdm-{name}--{name}...-1.
func (h *Handler) mpimName(members []string) string {
	names := make([]string, len(members))
	for i, id := range members {
		names[i] = id
		if u, ok := h.store.Users.Get(id); ok && u.Name != "" {
			names[i] = u.Name
		}
	}
	return "mpdm-" + strings.Join(names, "--") + "-1"
}

// ConversationsClose handles POST /api/conversations.close
func (h *Handler) ConversationsClose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
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
		slackArgsError(w, err)
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

// conversationType names a conversation the way the types argument does.
func conversationType(ch store.Channel) string {
	switch {
	case ch.IsIM:
		return "im"
	case ch.IsMPIM:
		return "mpim"
	case ch.IsPrivate:
		return "private_channel"
	}
	return "public_channel"
}

// conversationTypes reads a types argument. Slack's default is public
// channels only; a name that is not a conversation type is invalid_types.
func conversationTypes(arg string) (map[string]bool, bool) {
	if strings.TrimSpace(arg) == "" {
		return map[string]bool{"public_channel": true}, true
	}
	out := map[string]bool{}
	for _, t := range strings.Split(arg, ",") {
		t = strings.TrimSpace(t)
		switch t {
		case "public_channel", "private_channel", "mpim", "im":
			out[t] = true
		default:
			return nil, false
		}
	}
	return out, true
}

// visibleTo reports whether a caller can list a conversation: any public
// channel, and other conversations only when the caller is a member.
func visibleTo(ch store.Channel, user string) bool {
	return conversationType(ch) == "public_channel" || slices.Contains(ch.Members, user)
}
