package api

import (
	"net/http"
	"slices"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// UsersList handles POST /api/users.list
func (h *Handler) UsersList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cursor string `json:"cursor"`
		Limit  int    `json:"limit"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	users, next, err := pageOf(h.store.Users.List(), func(u store.User) string { return u.ID },
		pageUsersList, req.Cursor, req.Limit)
	if err != nil {
		slackError(w, "invalid_cursor")
		return
	}
	slackOK(w, map[string]any{
		"members":           users,
		"response_metadata": map[string]any{"next_cursor": next},
	})
}

// UsersInfo handles POST /api/users.info
func (h *Handler) UsersInfo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User string `json:"user"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	u, ok := h.store.Users.Get(req.User)
	if !ok {
		slackError(w, "user_not_found")
		return
	}
	slackOK(w, map[string]any{"user": u})
}

// UsersLookupByEmail handles POST /api/users.lookupByEmail
func (h *Handler) UsersLookupByEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	u, ok := h.store.GetUserByEmail(req.Email)
	if !ok {
		slackError(w, "users_not_found")
		return
	}
	slackOK(w, map[string]any{"user": u})
}

// UsersConversations handles POST /api/users.conversations
func (h *Handler) UsersConversations(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User   string `json:"user"`
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

	userID := req.User
	if userID == "" {
		userID = callerUserID(r)
	}

	// Another user's non-public conversations are listed only where the
	// caller is a member too.
	var member []store.Channel
	for _, ch := range h.store.Channels.List() {
		if types[conversationType(ch)] && slices.Contains(ch.Members, userID) && visibleTo(ch, caller) {
			member = append(member, ch)
		}
	}

	channels, next, err := pageOf(member, func(c store.Channel) string { return c.ID },
		pageUsersConversations, req.Cursor, req.Limit)
	if err != nil {
		slackError(w, "invalid_cursor")
		return
	}
	slackOK(w, map[string]any{
		"channels":          membershipViews(r, channels),
		"response_metadata": map[string]any{"next_cursor": next},
	})
}

// UsersProfileGet handles POST /api/users.profile.get
func (h *Handler) UsersProfileGet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User string `json:"user"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	userID := req.User
	if userID == "" {
		userID = callerUserID(r)
	}

	u, ok := h.store.Users.Get(userID)
	if !ok {
		slackError(w, "user_not_found")
		return
	}
	slackOK(w, map[string]any{"profile": u.Profile})
}

// UsersProfileSet handles POST /api/users.profile.set. It takes a user token,
// and sets either the fields of profile, or the one field name names to value.
// Another user's profile, and any email, may be set only by an admin or owner,
// and an admin's only by an owner.
func (h *Handler) UsersProfileSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile any    `json:"profile"`
		Name    string `json:"name"`
		Value   string `json:"value"`
		User    string `json:"user"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if principal(r).Type != store.TokenUser {
		slackError(w, "not_allowed_token_type")
		return
	}

	fields := map[string]any{}
	switch p := req.Profile.(type) {
	case nil:
		// A custom field is named by its ID, which starts with Xf.
		if strings.HasPrefix(req.Name, "Xf") {
			fields["fields"] = map[string]any{req.Name: map[string]any{"value": req.Value}}
		} else if req.Name != "" {
			fields[req.Name] = req.Value
		}
	case map[string]any:
		fields = p
	case string:
		if p != "" {
			slackError(w, "invalid_profile")
			return
		}
	default:
		slackError(w, "invalid_profile")
		return
	}

	caller := callerUserID(r)
	target := req.User
	if target == "" {
		target = caller
	}
	u, ok := h.store.Users.Get(target)
	if !ok {
		slackError(w, "user_not_found")
		return
	}
	me, _ := h.store.Users.Get(caller)
	if target != caller && !me.IsAdmin && !me.IsOwner {
		slackError(w, "not_admin")
		return
	}
	if target != caller && (u.IsAdmin || u.IsOwner) && !me.IsOwner {
		slackError(w, "cannot_update_admin_user")
		return
	}

	// Only an admin changes an email, and never their own: "You cannot update
	// your own email using this method" (the users.profile.set docs).
	if code := applyProfile(&u.Profile, fields, target != caller && (me.IsAdmin || me.IsOwner)); code != "" {
		slackError(w, code)
		return
	}
	if email, set := fields["email"].(string); set {
		if other, taken := h.store.GetUserByEmail(email); taken && other.ID != target {
			slackError(w, "email_taken")
			return
		}
	}
	u.RealName = u.Profile.RealName
	u.Updated = h.store.Clock.Now().Unix()
	h.store.Users.Set(target, u)
	slackOK(w, map[string]any{"profile": u.Profile})
}

// maxStatusText is the longest custom status the docs allow (too_long).
const maxStatusText = 100

// applyProfile sets the documented profile fields from a profile object. It
// returns the Slack error for a field that cannot be set, or "". Image fields
// are not settable here, and skype always stays empty.
func applyProfile(p *store.UserProfile, fields map[string]any, admin bool) string {
	str := func(k string) (string, bool) {
		v, ok := fields[k]
		if !ok {
			return "", false
		}
		s, _ := v.(string)
		return s, true
	}
	if v, ok := str("status_text"); ok {
		if len([]rune(v)) > maxStatusText {
			return "too_long"
		}
		p.StatusText = v
	}
	if v, ok := str("status_emoji"); ok {
		p.StatusEmoji = v
	}
	if v, ok := fields["status_expiration"].(float64); ok {
		p.StatusExpiration = int64(v)
	}
	if v, ok := str("email"); ok {
		if !admin {
			return "not_admin"
		}
		p.Email = v
	}
	if v, ok := str("display_name"); ok {
		p.DisplayName, p.DisplayNameNorm = v, strings.ToLower(v)
	}
	for k, dst := range map[string]*string{"title": &p.Title, "phone": &p.Phone, "pronouns": &p.Pronouns, "start_date": &p.StartDate} {
		if v, ok := str(k); ok {
			*dst = v
		}
	}
	if v, ok := str("real_name"); ok {
		// Setting real_name sets first_name and last_name; a single name
		// clears last_name.
		first, last, _ := strings.Cut(strings.TrimSpace(v), " ")
		p.FirstName, p.LastName = first, strings.TrimSpace(last)
	}
	if v, ok := str("first_name"); ok {
		p.FirstName = v
	}
	if v, ok := str("last_name"); ok {
		p.LastName = v
	}
	_, rn := fields["real_name"]
	_, fn := fields["first_name"]
	_, ln := fields["last_name"]
	if rn || fn || ln {
		p.RealName = strings.TrimSpace(p.FirstName + " " + p.LastName)
		p.RealNameNorm = strings.ToLower(p.RealName)
	}
	if custom, ok := fields["fields"].(map[string]any); ok {
		if p.Fields == nil {
			p.Fields = map[string]store.ProfileField{}
		}
		for id, raw := range custom {
			f, _ := raw.(map[string]any)
			value, _ := f["value"].(string)
			alt, _ := f["alt"].(string)
			p.Fields[id] = store.ProfileField{Value: value, Alt: alt}
		}
	}
	return ""
}

// UsersGetPresence handles POST /api/users.getPresence
func (h *Handler) UsersGetPresence(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User string `json:"user"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	// The user defaults to the caller.
	user := req.User
	if user == "" {
		user = callerUserID(r)
	}
	slackOK(w, map[string]any{"presence": h.presence(user)})
}

// presence is a user's presence: away when they set it manually, otherwise
// their seeded presence, active by default.
func (h *Handler) presence(user string) string {
	if p, ok := h.store.Presences.Get(user); ok && p.Presence == "away" {
		return "away"
	}
	if u, ok := h.store.Users.Get(user); ok && u.Presence != "" {
		return u.Presence
	}
	return "active"
}

// UsersSetPresence handles POST /api/users.setPresence
func (h *Handler) UsersSetPresence(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Presence string `json:"presence"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	// The docs accept auto or away.
	if req.Presence != "auto" && req.Presence != "away" {
		slackError(w, "invalid_presence")
		return
	}

	caller := callerUserID(r)
	h.store.Presences.Set(caller, store.Presence{User: caller, Presence: req.Presence})
	if u, ok := h.store.Users.Get(caller); ok {
		u.Presence = map[string]string{"auto": "active", "away": "away"}[req.Presence]
		h.store.Users.Set(caller, u)
	}
	slackOK(w, nil)
}
