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
		"channels":          membershipViews(r, channels),
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

	// The caller's read cursor, and for a direct or multi-person message
	// whether it is open for them.
	view := membershipView(r, ch)
	rs, _ := h.store.ReadStates.Get(store.ReadStateKey(ch.ID, callerUserID(r)))
	view.LastRead = rs.LastRead
	if ch.IsIM || ch.IsMPIM {
		open := !rs.Closed
		view.IsOpen = &open
	}
	slackOK(w, map[string]any{"channel": view})
}

// ConversationsHistory handles POST /api/conversations.history
func (h *Handler) ConversationsHistory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel            string `json:"channel"`
		Cursor             string `json:"cursor"`
		Limit              int    `json:"limit"`
		IncludeAllMetadata bool   `json:"include_all_metadata"`
		Oldest             string `json:"oldest"`
		Latest             string `json:"latest"`
		Inclusive          bool   `json:"inclusive"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if _, ok := h.store.Channels.Get(req.Channel); !ok {
		slackError(w, "channel_not_found")
		return
	}
	window, code := newTimeWindow(req.Oldest, req.Latest, req.Inclusive)
	if code != "" {
		slackError(w, code)
		return
	}

	messages, next, err := pageOf(window.filter(h.store.GetChannelMessages(req.Channel, 0)), func(m store.Message) string { return m.TS },
		pageConversationsHistory, req.Cursor, req.Limit)
	if err != nil {
		slackError(w, "invalid_cursor")
		return
	}
	out := map[string]any{
		"messages":          withMetadata(messages, req.IncludeAllMetadata),
		"has_more":          next != "",
		"response_metadata": map[string]any{"next_cursor": next},
	}
	// The docs' example echoes latest when the call gives it.
	if req.Latest != "" {
		out["latest"] = req.Latest
	}
	slackOK(w, out)
}

// ConversationsReplies handles POST /api/conversations.replies
func (h *Handler) ConversationsReplies(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel            string `json:"channel"`
		TS                 string `json:"ts"`
		Cursor             string `json:"cursor"`
		Limit              int    `json:"limit"`
		IncludeAllMetadata bool   `json:"include_all_metadata"`
		Oldest             string `json:"oldest"`
		Latest             string `json:"latest"`
		Inclusive          bool   `json:"inclusive"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	if _, ok := h.store.Channels.Get(req.Channel); !ok {
		slackError(w, "channel_not_found")
		return
	}
	// The docs answer thread_not_found for a ts that is "missing or invalid":
	// no message in the channel has it.
	if _, _, ok := h.store.GetMessageByTS(req.Channel, req.TS); !ok {
		slackError(w, "thread_not_found")
		return
	}

	window, code := newTimeWindow(req.Oldest, req.Latest, req.Inclusive)
	if code != "" {
		slackError(w, code)
		return
	}

	replies, next, err := pageOf(window.filter(h.store.GetThreadReplies(req.Channel, req.TS, 0)), func(m store.Message) string { return m.TS },
		pageConversationsReplies, req.Cursor, req.Limit)
	if err != nil {
		slackError(w, "invalid_cursor")
		return
	}
	slackOK(w, map[string]any{
		"messages":          withMetadata(replies, req.IncludeAllMetadata),
		"has_more":          next != "",
		"response_metadata": map[string]any{"next_cursor": next},
	})
}

// withMetadata returns messages with their metadata only when the caller asked
// for it with include_all_metadata.
func withMetadata(messages []store.Message, include bool) []store.Message {
	if include {
		return messages
	}
	out := make([]store.Message, len(messages))
	for i, m := range messages {
		m.Metadata = nil
		out[i] = m
	}
	return out
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
		Creator:    callerUserID(r),
		Created:    h.store.Clock.Now().Unix(),
		Members:    []string{callerUserID(r)},
		NumMembers: 1,
	}
	h.store.Channels.Set(id, ch)

	slackOK(w, map[string]any{"channel": membershipView(r, ch)})
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
	if req.Name == "" {
		slackError(w, "invalid_name_required")
		return
	}

	if _, _, taken := h.store.GetChannelByName(req.Name); taken {
		slackError(w, "name_taken")
		return
	}

	ch.Name = req.Name
	h.store.Channels.Set(req.Channel, ch)
	slackOK(w, map[string]any{"channel": membershipView(r, ch)})
}

// maxPurpose is the longest description conversations.setPurpose takes
// (too_long).
const maxPurpose = 250

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

	if ch.IsArchived {
		slackError(w, "is_archived")
		return
	}
	if len([]rune(req.Purpose)) > maxPurpose {
		slackError(w, "too_long")
		return
	}

	ch.Purpose = store.Topic{Value: req.Purpose, Creator: callerUserID(r), LastSet: h.store.Clock.Now().Unix()}
	h.store.Channels.Set(req.Channel, ch)
	// The method docs show the purpose string, and the SDK types the
	// conversation; Slack's answer is given as both.
	slackOK(w, map[string]any{"channel": membershipView(r, ch), "purpose": req.Purpose})
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

	if ch.IsArchived {
		slackError(w, "is_archived")
		return
	}

	ch.Topic = store.Topic{Value: req.Topic, Creator: callerUserID(r), LastSet: h.store.Clock.Now().Unix()}
	h.store.Channels.Set(req.Channel, ch)
	slackOK(w, map[string]any{"channel": membershipView(r, ch)})
}

// ConversationsInvite handles POST /api/conversations.invite
func (h *Handler) ConversationsInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Users   string `json:"users"` // comma-separated
		Force   bool   `json:"force"`
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

	users := splitCSV(req.Users)
	if len(users) == 0 {
		slackError(w, "no_user")
		return
	}
	// Slack checks every user named and, unless force is set, invites nobody
	// when any of them fails, answering with each failure in errors.
	caller := callerUserID(r)
	var valid []string
	var failures []map[string]any
	for _, u := range users {
		code := ""
		switch {
		case u == caller:
			code = "cant_invite_self"
		case !h.store.KnownUser(u):
			code = "user_not_found"
		}
		if code != "" {
			failures = append(failures, map[string]any{"user": u, "ok": false, "error": code})
			continue
		}
		valid = append(valid, u)
	}
	if len(failures) > 0 && !req.Force {
		slackErrorWith(w, failures[0]["error"].(string), map[string]any{"errors": failures})
		return
	}

	var added []string
	for _, u := range valid {
		if slices.Contains(ch.Members, u) {
			continue
		}
		ch.Members = append(ch.Members, u)
		ch.NumMembers++
		added = append(added, u)
	}
	if len(added) == 0 {
		if len(valid) == 0 {
			// force set, and every user named failed.
			slackErrorWith(w, failures[0]["error"].(string), map[string]any{"errors": failures})
			return
		}
		// Slack refuses an invite that adds nobody: every user named is already in.
		slackError(w, "already_in_channel")
		return
	}
	h.store.Channels.Set(req.Channel, ch)
	for _, u := range added {
		h.emitMemberJoined(ch, u, caller)
	}
	slackOK(w, map[string]any{"channel": membershipView(r, ch)})
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
	h.emitMemberLeft(ch, req.User)
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
	if ch.IsArchived {
		slackError(w, "is_archived")
		return
	}

	caller := callerUserID(r)
	if slices.Contains(ch.Members, caller) {
		// Slack still answers ok, with the already_in_channel warning.
		slackOK(w, withWarnings(map[string]any{"channel": membershipView(r, ch)}, "already_in_channel"))
		return
	}
	ch.Members = append(ch.Members, caller)
	ch.NumMembers++
	h.store.Channels.Set(req.Channel, ch)
	h.emitMemberJoined(ch, caller, "")

	slackOK(w, map[string]any{"channel": membershipView(r, ch)})
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
	if ch.IsArchived {
		slackError(w, "is_archived")
		return
	}

	caller := callerUserID(r)
	members := make([]string, 0, len(ch.Members))
	for _, m := range ch.Members {
		if m != caller {
			members = append(members, m)
		}
	}
	left := len(members) < len(ch.Members)
	ch.Members = members
	ch.NumMembers = len(members)
	h.store.Channels.Set(req.Channel, ch)
	if left {
		h.emitMemberLeft(ch, caller)
	}
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
		h.openAnswer(w, r, ch, true, req.ReturnIM)
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
		h.openAnswer(w, r, ch, true, req.ReturnIM)
		return
	}
	if req.PreventCreation {
		slackOK(w, map[string]any{"no_op": true, "already_open": false})
		return
	}

	id := h.store.Channels.NextID()
	ch := store.Channel{
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
	h.openAnswer(w, r, ch, false, req.ReturnIM)
}

// openAnswer writes the answer to conversations.open. Without return_im, the
// channel is answered with its id only, as the docs describe.
func (h *Handler) openAnswer(w http.ResponseWriter, r *http.Request, ch store.Channel, existing, returnIM bool) {
	// Opening reopens a conversation the caller closed.
	key := store.ReadStateKey(ch.ID, callerUserID(r))
	if rs, ok := h.store.ReadStates.Get(key); ok && rs.Closed {
		rs.Closed = false
		h.store.ReadStates.Set(key, rs)
	}
	fields := map[string]any{"channel": map[string]any{"id": ch.ID}}
	if returnIM {
		fields["channel"] = membershipView(r, ch)
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

// ConversationsClose handles POST /api/conversations.close. It closes a direct
// or multi-person message for the caller; closing one already closed answers
// ok with no_op and already_closed (the conversations.close docs).
func (h *Handler) ConversationsClose(w http.ResponseWriter, r *http.Request) {
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
	if !ch.IsIM && !ch.IsMPIM {
		slackError(w, "method_not_supported_for_channel_type")
		return
	}
	caller := callerUserID(r)
	if !slices.Contains(ch.Members, caller) {
		slackError(w, "user_does_not_own_channel")
		return
	}
	key := store.ReadStateKey(ch.ID, caller)
	rs, _ := h.store.ReadStates.Get(key)
	if rs.Closed {
		slackOK(w, map[string]any{"no_op": true, "already_closed": true})
		return
	}
	rs.Channel, rs.User, rs.Closed = ch.ID, caller, true
	h.store.ReadStates.Set(key, rs)
	slackOK(w, nil)
}

// ConversationsMark handles POST /api/conversations.mark. It moves the
// caller's read cursor, which conversations.info reports as last_read.
func (h *Handler) ConversationsMark(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
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
	if !isTimestamp(req.TS) {
		slackError(w, "invalid_timestamp")
		return
	}
	caller := callerUserID(r)
	if !slices.Contains(ch.Members, caller) {
		slackError(w, "not_in_channel")
		return
	}
	key := store.ReadStateKey(ch.ID, caller)
	rs, _ := h.store.ReadStates.Get(key)
	rs.Channel, rs.User, rs.LastRead = ch.ID, caller, req.TS
	h.store.ReadStates.Set(key, rs)
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
// channel, and other conversations only when the caller is a member. The
// users.conversations docs state this rule; applying it to conversations.list
// is an unverified guess to confirm at the next sandbox refresh.
func visibleTo(ch store.Channel, user string) bool {
	return conversationType(ch) == "public_channel" || slices.Contains(ch.Members, user)
}

// membershipView is the channel as the caller sees it: is_member is whether
// the token's user is in Members, as Slack computes it per caller.
func membershipView(r *http.Request, ch store.Channel) store.Channel {
	ch.IsMember = slices.Contains(ch.Members, callerUserID(r))
	return ch
}

func membershipViews(r *http.Request, chs []store.Channel) []store.Channel {
	out := make([]store.Channel, len(chs))
	for i, ch := range chs {
		out[i] = membershipView(r, ch)
	}
	return out
}
