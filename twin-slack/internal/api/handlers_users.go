package api

import (
	"net/http"
	"strings"
)

// UsersList handles POST /api/users.list
func (h *Handler) UsersList(w http.ResponseWriter, r *http.Request) {
	users := h.store.Users.List()
	slackOK(w, map[string]any{
		"members": users,
		"response_metadata": map[string]any{
			"next_cursor": "",
		},
	})
}

// UsersInfo handles POST /api/users.info
func (h *Handler) UsersInfo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User string `json:"user"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
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
		slackError(w, "invalid_json")
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
		User string `json:"user"`
	}
	parseJSON(r, &req)

	userID := req.User
	if userID == "" {
		userID = "U_BOT"
	}

	channels := h.store.Channels.List()
	var result []any
	for _, ch := range channels {
		for _, m := range ch.Members {
			if m == userID {
				result = append(result, ch)
				break
			}
		}
	}

	slackOK(w, map[string]any{
		"channels": result,
		"response_metadata": map[string]any{
			"next_cursor": "",
		},
	})
}

// UsersProfileGet handles POST /api/users.profile.get
func (h *Handler) UsersProfileGet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User string `json:"user"`
	}
	parseJSON(r, &req)

	userID := req.User
	if userID == "" {
		userID = "U_BOT"
	}

	u, ok := h.store.Users.Get(userID)
	if !ok {
		slackError(w, "user_not_found")
		return
	}
	slackOK(w, map[string]any{"profile": u.Profile})
}

// UsersProfileSet handles POST /api/users.profile.set
func (h *Handler) UsersProfileSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile struct {
			StatusText  *string `json:"status_text,omitempty"`
			StatusEmoji *string `json:"status_emoji,omitempty"`
			DisplayName *string `json:"display_name,omitempty"`
			RealName    *string `json:"real_name,omitempty"`
		} `json:"profile"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}

	// Apply to bot user
	u, ok := h.store.Users.Get("U_BOT")
	if !ok {
		slackError(w, "user_not_found")
		return
	}

	if req.Profile.StatusText != nil {
		u.Profile.StatusText = *req.Profile.StatusText
	}
	if req.Profile.StatusEmoji != nil {
		u.Profile.StatusEmoji = *req.Profile.StatusEmoji
	}
	if req.Profile.DisplayName != nil {
		u.Profile.DisplayName = *req.Profile.DisplayName
		u.Profile.DisplayNameNorm = strings.ToLower(*req.Profile.DisplayName)
	}
	if req.Profile.RealName != nil {
		u.Profile.RealName = *req.Profile.RealName
		u.Profile.RealNameNorm = strings.ToLower(*req.Profile.RealName)
	}

	h.store.Users.Set("U_BOT", u)
	slackOK(w, map[string]any{"profile": u.Profile})
}

// UsersGetPresence handles POST /api/users.getPresence
func (h *Handler) UsersGetPresence(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User string `json:"user"`
	}
	parseJSON(r, &req)

	u, ok := h.store.Users.Get(req.User)
	if !ok {
		slackOK(w, map[string]any{"presence": "active"})
		return
	}

	presence := u.Presence
	if presence == "" {
		presence = "active"
	}
	slackOK(w, map[string]any{"presence": presence})
}

// UsersSetPresence handles POST /api/users.setPresence
func (h *Handler) UsersSetPresence(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Presence string `json:"presence"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}

	u, ok := h.store.Users.Get("U_BOT")
	if ok {
		u.Presence = req.Presence
		h.store.Users.Set("U_BOT", u)
	}
	slackOK(w, nil)
}
