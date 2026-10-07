package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// --- bookmarks.* (stateful) ---

func (h *Handler) BookmarksAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChannelID string `json:"channel_id"`
		Title     string `json:"title"`
		Link      string `json:"link"`
		Emoji     string `json:"emoji"`
		Type      string `json:"type"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	switch {
	case req.Title == "":
		invalidArgument(w, "title")
		return
	case req.Type == "":
		invalidArgument(w, "type")
		return
	case req.Type != "link":
		// link is the only type the docs say bookmarks.add accepts.
		slackError(w, "invalid_bookmark_type")
		return
	case !strings.HasPrefix(req.Link, "http://") && !strings.HasPrefix(req.Link, "https://"):
		slackError(w, "invalid_link")
		return
	}
	if _, ok := h.store.Channels.Get(req.ChannelID); !ok {
		slackError(w, "channel_not_found")
		return
	}

	now := h.store.Clock.Now().Unix()
	id := h.store.Bookmarks.NextID()
	bm := store.Bookmark{
		ID: id, ChannelID: req.ChannelID, Title: req.Title,
		Link: req.Link, Emoji: req.Emoji, Type: req.Type,
		CreatedAt: now, UpdatedAt: now,
	}
	h.store.Bookmarks.Set(id, bm)
	slackOK(w, map[string]any{"bookmark": bm})
}

func (h *Handler) BookmarksEdit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BookmarkID string `json:"bookmark_id"`
		ChannelID  string `json:"channel_id"`
		Title      string `json:"title,omitempty"`
		Link       string `json:"link,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	bm, ok := h.channelBookmark(w, req.ChannelID, req.BookmarkID)
	if !ok {
		return
	}
	if req.Link != "" && !strings.HasPrefix(req.Link, "http://") && !strings.HasPrefix(req.Link, "https://") {
		slackError(w, "invalid_link")
		return
	}
	if req.Title != "" {
		bm.Title = req.Title
	}
	if req.Link != "" {
		bm.Link = req.Link
	}
	bm.UpdatedAt = h.store.Clock.Now().Unix()
	h.store.Bookmarks.Set(req.BookmarkID, bm)
	slackOK(w, map[string]any{"bookmark": bm})
}

func (h *Handler) BookmarksList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChannelID string `json:"channel_id"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if req.ChannelID != "" {
		if _, ok := h.store.Channels.Get(req.ChannelID); !ok {
			slackError(w, "channel_not_found")
			return
		}
	}

	bms := h.store.Bookmarks.Filter(func(_ string, bm store.Bookmark) bool {
		return req.ChannelID == "" || bm.ChannelID == req.ChannelID
	})
	if bms == nil {
		bms = []store.Bookmark{}
	}
	slackOK(w, map[string]any{"bookmarks": bms})
}

func (h *Handler) BookmarksRemove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BookmarkID string `json:"bookmark_id"`
		ChannelID  string `json:"channel_id"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if _, ok := h.channelBookmark(w, req.ChannelID, req.BookmarkID); !ok {
		return
	}
	h.store.Bookmarks.Delete(req.BookmarkID)
	slackOK(w, nil)
}

// channelBookmark finds a bookmark in a channel, answering the error Slack's
// docs list when it cannot: channel_not_found for an unknown channel, and
// not_found for a bookmark that does not exist or is in another channel.
func (h *Handler) channelBookmark(w http.ResponseWriter, channelID, bookmarkID string) (store.Bookmark, bool) {
	if _, ok := h.store.Channels.Get(channelID); !ok {
		slackError(w, "channel_not_found")
		return store.Bookmark{}, false
	}
	bm, ok := h.store.Bookmarks.Get(bookmarkID)
	if !ok || bm.ChannelID != channelID {
		slackError(w, "not_found")
		return store.Bookmark{}, false
	}
	return bm, true
}

// --- emoji.* ---

// EmojiList lists the workspace's custom emoji: each name maps to its image
// URL, or to "alias:" and the name of the emoji it stands for. A workspace has
// none until they are seeded.
func (h *Handler) EmojiList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IncludeCategories bool `json:"include_categories"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	emoji := map[string]string{}
	for _, name := range h.store.Emoji.ListIDs() {
		v, _ := h.store.Emoji.Get(name)
		emoji[name] = v
	}
	slackOK(w, map[string]any{"emoji": emoji})
}

// --- team.* ---

// TeamInfo answers for the caller's workspace. team names a workspace by ID,
// and domain is only for teams on an Enterprise organization, which this one
// is not.
func (h *Handler) TeamInfo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Team   string `json:"team"`
		Domain string `json:"domain"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if req.Team != "" && req.Team != h.store.Team.ID {
		slackError(w, "team_not_found")
		return
	}
	if req.Team == "" && req.Domain != "" {
		slackError(w, "team_not_on_enterprise")
		return
	}
	slackOK(w, map[string]any{"team": h.store.Team})
}

// --- bots.* ---

// BotsInfo answers for the bot that bot names, by its bot ID.
func (h *Handler) BotsInfo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Bot string `json:"bot"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	b, ok := h.store.Bots.Get(req.Bot)
	if !ok {
		slackError(w, "bot_not_found")
		return
	}
	slackOK(w, map[string]any{"bot": b})
}

// --- usergroups.* (stateful) ---

// UsergroupsList lists enabled user groups, and disabled ones too with
// include_disabled.
func (h *Handler) UsergroupsList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IncludeDisabled bool `json:"include_disabled"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	ugs := h.store.Usergroups.Filter(func(_ string, ug store.Usergroup) bool {
		return req.IncludeDisabled || ug.DateDelete == 0
	})
	if ugs == nil {
		ugs = []store.Usergroup{}
	}
	slackOK(w, map[string]any{"usergroups": ugs})
}

// UsergroupsCreate handles POST /api/usergroups.create. A name is required
// and unique among user groups; a handle is unique among channels, users and
// user groups.
func (h *Handler) UsergroupsCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		Handle       string `json:"handle"`
		Description  string `json:"description"`
		Channels     any    `json:"channels"`
		IncludeCount bool   `json:"include_count"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		slackError(w, "missing_subteam_name")
		return
	}
	for _, ug := range h.store.Usergroups.List() {
		if strings.EqualFold(ug.Name, req.Name) {
			slackError(w, "name_already_exists")
			return
		}
	}
	if req.Handle != "" && h.handleTaken(req.Handle) {
		slackError(w, "handle_already_exists")
		return
	}
	channels := idList(req.Channels)
	for _, c := range channels {
		if _, ok := h.store.Channels.Get(c); !ok {
			slackError(w, "invalid_channel_provided")
			return
		}
	}

	now := h.store.Clock.Now().Unix()
	id := h.store.Usergroups.NextID()
	ug := store.Usergroup{
		ID: id, TeamID: h.store.Team.ID, Name: req.Name, Handle: req.Handle, Description: req.Description,
		IsUsergroup: true, CreatedBy: callerUserID(r), UpdatedBy: callerUserID(r), DateCreate: now, DateUpdate: now,
		Prefs: store.UsergroupPrefs{Channels: channels},
	}
	h.store.Usergroups.Set(id, ug)
	out := map[string]any{"usergroup": ug}
	if req.IncludeCount {
		out["usergroup"] = withUserCount(ug)
	}
	slackOK(w, out)
}

// handleTaken reports whether a mention handle is already a channel's name,
// a user's name or a user group's handle.
func (h *Handler) handleTaken(handle string) bool {
	for _, ch := range h.store.Channels.List() {
		if strings.EqualFold(ch.Name, handle) {
			return true
		}
	}
	for _, u := range h.store.Users.List() {
		if strings.EqualFold(u.Name, handle) {
			return true
		}
	}
	for _, ug := range h.store.Usergroups.List() {
		if strings.EqualFold(ug.Handle, handle) {
			return true
		}
	}
	return false
}

// withUserCount renders a user group with the user_count include_count asks
// for.
func withUserCount(ug store.Usergroup) map[string]any {
	raw, _ := json.Marshal(ug)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	out["user_count"] = len(ug.Users)
	return out
}

func (h *Handler) UsergroupsUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Usergroup   string `json:"usergroup"`
		Name        string `json:"name,omitempty"`
		Handle      string `json:"handle,omitempty"`
		Description string `json:"description,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	ug, ok := h.store.Usergroups.Get(req.Usergroup)
	if !ok {
		slackError(w, "not_found")
		return
	}
	if req.Name != "" {
		ug.Name = req.Name
	}
	if req.Handle != "" {
		ug.Handle = req.Handle
	}
	if req.Description != "" {
		ug.Description = req.Description
	}
	ug.DateUpdate = h.store.Clock.Now().Unix()
	h.store.Usergroups.Set(req.Usergroup, ug)
	slackOK(w, map[string]any{"usergroup": ug})
}

func (h *Handler) UsergroupsDisable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Usergroup string `json:"usergroup"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	ug, ok := h.store.Usergroups.Get(req.Usergroup)
	if !ok {
		slackError(w, "not_found")
		return
	}
	ug.DateDelete = h.store.Clock.Now().Unix()
	h.store.Usergroups.Set(req.Usergroup, ug)
	slackOK(w, map[string]any{"usergroup": ug})
}

func (h *Handler) UsergroupsEnable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Usergroup string `json:"usergroup"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	ug, ok := h.store.Usergroups.Get(req.Usergroup)
	if !ok {
		slackError(w, "not_found")
		return
	}
	ug.DateDelete = 0
	h.store.Usergroups.Set(req.Usergroup, ug)
	slackOK(w, map[string]any{"usergroup": ug})
}

func (h *Handler) UsergroupsUsersList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Usergroup string `json:"usergroup"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	ug, ok := h.store.Usergroups.Get(req.Usergroup)
	if !ok {
		slackError(w, "not_found")
		return
	}
	users := ug.Users
	if users == nil {
		users = []string{}
	}
	slackOK(w, map[string]any{"users": users})
}

func (h *Handler) UsergroupsUsersUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Usergroup string `json:"usergroup"`
		Users     string `json:"users"` // comma-separated
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	ug, ok := h.store.Usergroups.Get(req.Usergroup)
	if !ok {
		slackError(w, "not_found")
		return
	}
	ug.Users = splitCSV(req.Users)
	ug.DateUpdate = h.store.Clock.Now().Unix()
	h.store.Usergroups.Set(req.Usergroup, ug)
	slackOK(w, map[string]any{"usergroup": ug})
}

// idList reads a list of IDs given as a comma-separated string or a JSON
// array.
func idList(v any) []string {
	switch l := v.(type) {
	case string:
		return splitCSV(l)
	case []any:
		var out []string
		for _, item := range l {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
