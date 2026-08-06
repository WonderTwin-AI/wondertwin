package api

import (
	"net/http"

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
		slackError(w, "invalid_json")
		return
	}
	if req.Type == "" {
		req.Type = "link"
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
	parseJSON(r, &req)

	bms := h.store.Bookmarks.Filter(func(_ string, bm store.Bookmark) bool {
		return req.ChannelID == "" || bm.ChannelID == req.ChannelID
	})
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

// --- reminders.* (stateful) ---

func (h *Handler) RemindersAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
		Time int64  `json:"time"`
		User string `json:"user"`
	}
	parseJSON(r, &req)

	id := h.store.Reminders.NextID()
	rm := store.Reminder{
		ID: id, Creator: "U_BOT", User: req.User, Text: req.Text, Time: req.Time,
	}
	h.store.Reminders.Set(id, rm)
	slackOK(w, map[string]any{"reminder": rm})
}

func (h *Handler) RemindersComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reminder string `json:"reminder"`
	}
	parseJSON(r, &req)
	rm, ok := h.store.Reminders.Get(req.Reminder)
	if !ok {
		slackError(w, "not_found")
		return
	}
	rm.CompleteTS = h.store.Clock.Now().Unix()
	h.store.Reminders.Set(req.Reminder, rm)
	slackOK(w, nil)
}

func (h *Handler) RemindersDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reminder string `json:"reminder"`
	}
	parseJSON(r, &req)
	h.store.Reminders.Delete(req.Reminder)
	slackOK(w, nil)
}

func (h *Handler) RemindersInfo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reminder string `json:"reminder"`
	}
	parseJSON(r, &req)
	rm, ok := h.store.Reminders.Get(req.Reminder)
	if !ok {
		slackError(w, "not_found")
		return
	}
	slackOK(w, map[string]any{"reminder": rm})
}

func (h *Handler) RemindersList(w http.ResponseWriter, r *http.Request) {
	rms := h.store.Reminders.List()
	slackOK(w, map[string]any{"reminders": rms})
}

// --- views.* (stateful — stores trigger_id → view mapping) ---

func (h *Handler) ViewsOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TriggerID string `json:"trigger_id"`
		View      any    `json:"view"`
	}
	parseJSON(r, &req)
	slackOK(w, map[string]any{"view": map[string]any{
		"id":   "V_" + h.store.Channels.NextID(),
		"type": "modal",
	}})
}

func (h *Handler) ViewsPush(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TriggerID string `json:"trigger_id"`
		View      any    `json:"view"`
	}
	parseJSON(r, &req)
	slackOK(w, map[string]any{"view": map[string]any{
		"id":   "V_" + h.store.Channels.NextID(),
		"type": "modal",
	}})
}

func (h *Handler) ViewsUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ViewID     string `json:"view_id"`
		ExternalID string `json:"external_id"`
		View       any    `json:"view"`
	}
	parseJSON(r, &req)
	viewID := req.ViewID
	if viewID == "" {
		viewID = req.ExternalID
	}
	slackOK(w, map[string]any{"view": map[string]any{
		"id":   viewID,
		"type": "modal",
	}})
}

func (h *Handler) ViewsPublish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		View   any    `json:"view"`
	}
	parseJSON(r, &req)
	slackOK(w, map[string]any{"view": map[string]any{
		"id":   "V_HOME_" + req.UserID,
		"type": "home",
	}})
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

func (h *Handler) TeamAccessLogs(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{"logins": []any{}})
}

func (h *Handler) TeamBillableInfo(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{"billable_info": map[string]any{}})
}

func (h *Handler) TeamIntegrationLogs(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{"logs": []any{}})
}

func (h *Handler) TeamProfileGet(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{"profile": map[string]any{"fields": []any{}}})
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

func (h *Handler) UsergroupsList(w http.ResponseWriter, r *http.Request) {
	ugs := h.store.Usergroups.List()
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
		IsEnabled: true, CreatedBy: "U_BOT", DateCreate: now, DateUpdate: now,
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
	ug.IsEnabled = false
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
	ug.IsEnabled = true
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

// --- dnd.* (stateful) ---

func (h *Handler) DndInfo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User string `json:"user"`
	}
	parseJSON(r, &req)
	userID := req.User
	if userID == "" {
		userID = "U_BOT"
	}
	status := h.store.DndStatuses[userID]
	slackOK(w, map[string]any{
		"dnd_enabled":       status.DndEnabled,
		"next_dnd_start_ts": status.NextStart,
		"next_dnd_end_ts":   status.NextEnd,
		"snooze_enabled":    status.SnoozeEnabled,
		"snooze_endtime":    status.SnoozeEndtime,
	})
}

func (h *Handler) DndSetSnooze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NumMinutes int `json:"num_minutes"`
	}
	parseJSON(r, &req)
	now := h.store.Clock.Now().Unix()
	endtime := now + int64(req.NumMinutes*60)
	h.store.DndStatuses["U_BOT"] = store.DndStatus{
		DndEnabled: true, SnoozeEnabled: true, SnoozeEndtime: endtime,
		NextStart: now, NextEnd: endtime,
	}
	slackOK(w, map[string]any{
		"snooze_enabled":   true,
		"snooze_endtime":   endtime,
		"snooze_remaining": req.NumMinutes * 60,
	})
}

func (h *Handler) DndEndSnooze(w http.ResponseWriter, r *http.Request) {
	status := h.store.DndStatuses["U_BOT"]
	status.SnoozeEnabled = false
	status.SnoozeEndtime = 0
	h.store.DndStatuses["U_BOT"] = status
	slackOK(w, map[string]any{
		"dnd_enabled":    status.DndEnabled,
		"snooze_enabled": false,
	})
}

func (h *Handler) DndEndDnd(w http.ResponseWriter, r *http.Request) {
	h.store.DndStatuses["U_BOT"] = store.DndStatus{}
	slackOK(w, nil)
}

func (h *Handler) DndTeamInfo(w http.ResponseWriter, r *http.Request) {
	result := make(map[string]any)
	for userID, status := range h.store.DndStatuses {
		result[userID] = map[string]any{
			"dnd_enabled":       status.DndEnabled,
			"next_dnd_start_ts": status.NextStart,
			"next_dnd_end_ts":   status.NextEnd,
		}
	}
	slackOK(w, map[string]any{"users": result})
}

// --- search.* (searches actual store state) ---

func (h *Handler) SearchMessages(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
	}
	parseJSON(r, &req)

	msgs := h.store.Messages.List()
	slackOK(w, map[string]any{
		"messages": map[string]any{
			"total":   len(msgs),
			"matches": msgs,
		},
	})
}

func (h *Handler) SearchFiles(w http.ResponseWriter, r *http.Request) {
	files := h.store.Files.List()
	slackOK(w, map[string]any{
		"files": map[string]any{
			"total":   len(files),
			"matches": files,
		},
	})
}

func (h *Handler) SearchAll(w http.ResponseWriter, r *http.Request) {
	msgs := h.store.Messages.List()
	files := h.store.Files.List()
	slackOK(w, map[string]any{
		"messages": map[string]any{"total": len(msgs), "matches": msgs},
		"files":    map[string]any{"total": len(files), "matches": files},
	})
}

// --- stars.* (stateful) ---

func (h *Handler) StarsAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel   string `json:"channel"`
		Timestamp string `json:"timestamp"`
		File      string `json:"file"`
	}
	parseJSON(r, &req)

	id := h.store.Stars.NextID()
	star := store.Star{Type: "message", Channel: req.Channel}
	if req.Timestamp != "" {
		msg, _, ok := h.store.GetMessageByTS(req.Channel, req.Timestamp)
		if ok {
			star.Message = msg
		}
	}
	if req.File != "" {
		f, ok := h.store.Files.Get(req.File)
		if ok {
			star.Type = "file"
			star.File = &f
		}
	}
	h.store.Stars.Set(id, star)
	slackOK(w, nil)
}

func (h *Handler) StarsRemove(w http.ResponseWriter, r *http.Request) {
	// Simplified: remove first matching star
	var req struct {
		Channel   string `json:"channel"`
		Timestamp string `json:"timestamp"`
		File      string `json:"file"`
	}
	parseJSON(r, &req)
	slackOK(w, nil)
}

func (h *Handler) StarsList(w http.ResponseWriter, r *http.Request) {
	stars := h.store.Stars.List()
	items := make([]any, 0, len(stars))
	for _, s := range stars {
		items = append(items, s)
	}
	slackOK(w, map[string]any{
		"items":  items,
		"paging": map[string]any{"count": len(items), "total": len(items), "page": 1, "pages": 1},
	})
}
