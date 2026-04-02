package api

// Stub handlers for methods that return valid Slack responses with minimal state.
// These will be fleshed out in follow-up PRs with full state management.

import "net/http"

// --- bookmarks.* ---

func (h *Handler) BookmarksAdd(w http.ResponseWriter, r *http.Request)    { slackOK(w, map[string]any{"bookmark": map[string]any{}}) }
func (h *Handler) BookmarksEdit(w http.ResponseWriter, r *http.Request)   { slackOK(w, map[string]any{"bookmark": map[string]any{}}) }
func (h *Handler) BookmarksList(w http.ResponseWriter, r *http.Request)   { slackOK(w, map[string]any{"bookmarks": []any{}}) }
func (h *Handler) BookmarksRemove(w http.ResponseWriter, r *http.Request) { slackOK(w, nil) }

// --- reminders.* ---

func (h *Handler) RemindersAdd(w http.ResponseWriter, r *http.Request)      { slackOK(w, map[string]any{"reminder": map[string]any{}}) }
func (h *Handler) RemindersComplete(w http.ResponseWriter, r *http.Request)  { slackOK(w, nil) }
func (h *Handler) RemindersDelete(w http.ResponseWriter, r *http.Request)    { slackOK(w, nil) }
func (h *Handler) RemindersInfo(w http.ResponseWriter, r *http.Request)      { slackOK(w, map[string]any{"reminder": map[string]any{}}) }
func (h *Handler) RemindersList(w http.ResponseWriter, r *http.Request)      { slackOK(w, map[string]any{"reminders": []any{}}) }

// --- views.* ---

func (h *Handler) ViewsOpen(w http.ResponseWriter, r *http.Request)    { slackOK(w, map[string]any{"view": map[string]any{"id": "V001"}}) }
func (h *Handler) ViewsPush(w http.ResponseWriter, r *http.Request)    { slackOK(w, map[string]any{"view": map[string]any{"id": "V001"}}) }
func (h *Handler) ViewsUpdate(w http.ResponseWriter, r *http.Request)  { slackOK(w, map[string]any{"view": map[string]any{"id": "V001"}}) }
func (h *Handler) ViewsPublish(w http.ResponseWriter, r *http.Request) { slackOK(w, map[string]any{"view": map[string]any{"id": "V001"}}) }

// --- emoji.* ---

func (h *Handler) EmojiList(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{"emoji": map[string]any{}})
}

// --- team.* ---

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
			"id":      "B_BOT",
			"name":    "wondertwin-bot",
			"deleted": false,
		},
	})
}

// --- usergroups.* ---

func (h *Handler) UsergroupsList(w http.ResponseWriter, r *http.Request)       { slackOK(w, map[string]any{"usergroups": []any{}}) }
func (h *Handler) UsergroupsCreate(w http.ResponseWriter, r *http.Request)     { slackOK(w, map[string]any{"usergroup": map[string]any{}}) }
func (h *Handler) UsergroupsUpdate(w http.ResponseWriter, r *http.Request)     { slackOK(w, map[string]any{"usergroup": map[string]any{}}) }
func (h *Handler) UsergroupsDisable(w http.ResponseWriter, r *http.Request)    { slackOK(w, map[string]any{"usergroup": map[string]any{}}) }
func (h *Handler) UsergroupsEnable(w http.ResponseWriter, r *http.Request)     { slackOK(w, map[string]any{"usergroup": map[string]any{}}) }
func (h *Handler) UsergroupsUsersList(w http.ResponseWriter, r *http.Request)  { slackOK(w, map[string]any{"users": []any{}}) }
func (h *Handler) UsergroupsUsersUpdate(w http.ResponseWriter, r *http.Request) { slackOK(w, map[string]any{"usergroup": map[string]any{}}) }

// --- dnd.* ---

func (h *Handler) DndInfo(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{
		"dnd_enabled":    false,
		"next_dnd_start_ts": 0,
		"next_dnd_end_ts":   0,
		"snooze_enabled":    false,
	})
}
func (h *Handler) DndSetSnooze(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{"snooze_enabled": true})
}
func (h *Handler) DndEndSnooze(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{"dnd_enabled": false, "snooze_enabled": false})
}
func (h *Handler) DndEndDnd(w http.ResponseWriter, r *http.Request) {
	slackOK(w, nil)
}
func (h *Handler) DndTeamInfo(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{"users": map[string]any{}})
}

// --- search.* ---

func (h *Handler) SearchMessages(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{
		"messages": map[string]any{
			"total":   0,
			"matches": []any{},
		},
	})
}
func (h *Handler) SearchFiles(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{
		"files": map[string]any{
			"total":   0,
			"matches": []any{},
		},
	})
}
func (h *Handler) SearchAll(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{
		"messages": map[string]any{"total": 0, "matches": []any{}},
		"files":    map[string]any{"total": 0, "matches": []any{}},
	})
}

// --- stars.* ---

func (h *Handler) StarsAdd(w http.ResponseWriter, r *http.Request)    { slackOK(w, nil) }
func (h *Handler) StarsRemove(w http.ResponseWriter, r *http.Request) { slackOK(w, nil) }
func (h *Handler) StarsList(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{
		"items": []any{},
		"paging": map[string]any{"count": 0, "total": 0, "page": 1, "pages": 1},
	})
}
