package api

import (
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
	parseJSON(r, &req)

	bm, ok := h.store.Bookmarks.Get(req.BookmarkID)
	if !ok {
		slackError(w, "bookmark_not_found")
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
	parseJSON(r, &req)
	h.store.Bookmarks.Delete(req.BookmarkID)
	slackOK(w, nil)
}

// --- emoji.* ---

func (h *Handler) EmojiList(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{"emoji": map[string]any{
		"thumbsup": "alias:+1",
		"shipit":   "alias:squirrel",
	}})
}

// --- team.* (expanded) ---

func (h *Handler) TeamInfo(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{
		"team": map[string]any{
			"id":     h.store.Team.ID,
			"name":   h.store.Team.Name,
			"domain": h.store.Team.Domain,
		},
	})
}

// --- bots.* ---

func (h *Handler) BotsInfo(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{
		"bot": map[string]any{
			"id": "B_BOT", "name": "wondertwin-bot", "deleted": false,
		},
	})
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

func (h *Handler) UsergroupsCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Handle      string `json:"handle"`
		Description string `json:"description"`
	}
	parseJSON(r, &req)

	now := h.store.Clock.Now().Unix()
	id := h.store.Usergroups.NextID()
	ug := store.Usergroup{
		ID: id, Name: req.Name, Handle: req.Handle, Description: req.Description,
		IsUsergroup: true, CreatedBy: callerUserID(r), DateCreate: now, DateUpdate: now,
	}
	h.store.Usergroups.Set(id, ug)
	slackOK(w, map[string]any{"usergroup": ug})
}

func (h *Handler) UsergroupsUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Usergroup   string `json:"usergroup"`
		Name        string `json:"name,omitempty"`
		Handle      string `json:"handle,omitempty"`
		Description string `json:"description,omitempty"`
	}
	parseJSON(r, &req)

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
	parseJSON(r, &req)
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
	parseJSON(r, &req)
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
	parseJSON(r, &req)
	ug, ok := h.store.Usergroups.Get(req.Usergroup)
	if !ok {
		slackError(w, "not_found")
		return
	}
	slackOK(w, map[string]any{"users": ug.Users})
}

func (h *Handler) UsergroupsUsersUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Usergroup string `json:"usergroup"`
		Users     string `json:"users"` // comma-separated
	}
	parseJSON(r, &req)
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
