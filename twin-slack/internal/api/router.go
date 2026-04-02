// Package api implements the Slack Web API-compatible HTTP handlers for the twin.
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/quirks"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// Handler holds Slack API state.
type Handler struct {
	store   *store.MemoryStore
	mw      *twincore.Middleware
	emitter *telemetry.Emitter
	quirks  *quirks.Engine
}

// NewHandler creates a new Slack API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware, em *telemetry.Emitter, qe *quirks.Engine) *Handler {
	return &Handler{store: s, mw: mw, emitter: em, quirks: qe}
}

// Routes mounts the Slack Web API-compatible routes.
// Slack uses POST to method-name paths: /api/chat.postMessage, /api/conversations.list, etc.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/api", func(r chi.Router) {
		r.Use(h.bearerAuthMiddleware)
		r.Use(h.mw.FaultInjection)
		r.Use(quirks.Middleware(h.quirks))
		r.Use(telemetry.Middleware(h.emitter))

		// auth.*
		r.Post("/auth.test", h.AuthTest)
		r.Post("/auth.revoke", h.AuthRevoke)

		// chat.*
		r.Post("/chat.postMessage", h.ChatPostMessage)
		r.Post("/chat.postEphemeral", h.ChatPostEphemeral)
		r.Post("/chat.update", h.ChatUpdate)
		r.Post("/chat.delete", h.ChatDelete)
		r.Post("/chat.getPermalink", h.ChatGetPermalink)
		r.Post("/chat.scheduleMessage", h.ChatScheduleMessage)
		r.Post("/chat.deleteScheduledMessage", h.ChatDeleteScheduledMessage)
		r.Post("/chat.scheduledMessages.list", h.ChatScheduledMessagesList)
		r.Post("/chat.meMessage", h.ChatMeMessage)

		// conversations.*
		r.Post("/conversations.list", h.ConversationsList)
		r.Post("/conversations.info", h.ConversationsInfo)
		r.Post("/conversations.history", h.ConversationsHistory)
		r.Post("/conversations.replies", h.ConversationsReplies)
		r.Post("/conversations.members", h.ConversationsMembers)
		r.Post("/conversations.create", h.ConversationsCreate)
		r.Post("/conversations.archive", h.ConversationsArchive)
		r.Post("/conversations.unarchive", h.ConversationsUnarchive)
		r.Post("/conversations.rename", h.ConversationsRename)
		r.Post("/conversations.setPurpose", h.ConversationsSetPurpose)
		r.Post("/conversations.setTopic", h.ConversationsSetTopic)
		r.Post("/conversations.invite", h.ConversationsInvite)
		r.Post("/conversations.kick", h.ConversationsKick)
		r.Post("/conversations.join", h.ConversationsJoin)
		r.Post("/conversations.leave", h.ConversationsLeave)
		r.Post("/conversations.open", h.ConversationsOpen)
		r.Post("/conversations.close", h.ConversationsClose)
		r.Post("/conversations.mark", h.ConversationsMark)

		// users.*
		r.Post("/users.list", h.UsersList)
		r.Post("/users.info", h.UsersInfo)
		r.Post("/users.lookupByEmail", h.UsersLookupByEmail)
		r.Post("/users.conversations", h.UsersConversations)
		r.Post("/users.profile.get", h.UsersProfileGet)
		r.Post("/users.profile.set", h.UsersProfileSet)
		r.Post("/users.getPresence", h.UsersGetPresence)
		r.Post("/users.setPresence", h.UsersSetPresence)

		// reactions.*
		r.Post("/reactions.add", h.ReactionsAdd)
		r.Post("/reactions.remove", h.ReactionsRemove)
		r.Post("/reactions.get", h.ReactionsGet)
		r.Post("/reactions.list", h.ReactionsList)

		// pins.*
		r.Post("/pins.add", h.PinsAdd)
		r.Post("/pins.remove", h.PinsRemove)
		r.Post("/pins.list", h.PinsList)

		// files.*
		r.Post("/files.getUploadURLExternal", h.FilesGetUploadURLExternal)
		r.Post("/files.completeUploadExternal", h.FilesCompleteUploadExternal)
		r.Post("/files.list", h.FilesList)
		r.Post("/files.info", h.FilesInfo)
		r.Post("/files.delete", h.FilesDelete)

		// bookmarks.*
		r.Post("/bookmarks.add", h.BookmarksAdd)
		r.Post("/bookmarks.edit", h.BookmarksEdit)
		r.Post("/bookmarks.list", h.BookmarksList)
		r.Post("/bookmarks.remove", h.BookmarksRemove)

		// reminders.*
		r.Post("/reminders.add", h.RemindersAdd)
		r.Post("/reminders.complete", h.RemindersComplete)
		r.Post("/reminders.delete", h.RemindersDelete)
		r.Post("/reminders.info", h.RemindersInfo)
		r.Post("/reminders.list", h.RemindersList)

		// views.*
		r.Post("/views.open", h.ViewsOpen)
		r.Post("/views.push", h.ViewsPush)
		r.Post("/views.update", h.ViewsUpdate)
		r.Post("/views.publish", h.ViewsPublish)

		// emoji.*
		r.Post("/emoji.list", h.EmojiList)

		// team.*
		r.Post("/team.info", h.TeamInfo)

		// bots.*
		r.Post("/bots.info", h.BotsInfo)

		// usergroups.*
		r.Post("/usergroups.list", h.UsergroupsList)
		r.Post("/usergroups.create", h.UsergroupsCreate)
		r.Post("/usergroups.update", h.UsergroupsUpdate)
		r.Post("/usergroups.disable", h.UsergroupsDisable)
		r.Post("/usergroups.enable", h.UsergroupsEnable)
		r.Post("/usergroups.users.list", h.UsergroupsUsersList)
		r.Post("/usergroups.users.update", h.UsergroupsUsersUpdate)

		// dnd.*
		r.Post("/dnd.info", h.DndInfo)
		r.Post("/dnd.setSnooze", h.DndSetSnooze)
		r.Post("/dnd.endSnooze", h.DndEndSnooze)
		r.Post("/dnd.endDnd", h.DndEndDnd)
		r.Post("/dnd.teamInfo", h.DndTeamInfo)

		// search.*
		r.Post("/search.messages", h.SearchMessages)
		r.Post("/search.files", h.SearchFiles)
		r.Post("/search.all", h.SearchAll)

		// stars.*
		r.Post("/stars.add", h.StarsAdd)
		r.Post("/stars.remove", h.StarsRemove)
		r.Post("/stars.list", h.StarsList)
	})

	// Admin extras (no auth required)
	r.Get("/admin/messages", h.AdminListMessages)
	r.Get("/admin/channels", h.AdminListChannels)
	r.Get("/admin/users", h.AdminListUsers)
}

// bearerAuthMiddleware validates Slack-style Bearer token auth.
// Accepts any token starting with "xoxb-" or "xoxp-" in sim mode.
func (h *Handler) bearerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			slackError(w, "not_authed")
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth || token == "" {
			slackError(w, "invalid_auth")
			return
		}

		// In sim mode, accept any non-empty token
		next.ServeHTTP(w, r)
	})
}

// slackOK writes a successful Slack API response.
func slackOK(w http.ResponseWriter, fields map[string]any) {
	resp := map[string]any{"ok": true}
	for k, v := range fields {
		resp[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// slackError writes a Slack API error response.
func slackError(w http.ResponseWriter, code string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": code,
	})
}

// parseJSON reads JSON body from the request. Slack accepts both JSON and form-encoded.
func parseJSON(r *http.Request, v any) error {
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		return json.NewDecoder(r.Body).Decode(v)
	}
	// For form-encoded, Slack SDKs still send JSON body with JSON content type.
	// Fall back to trying JSON anyway.
	return json.NewDecoder(r.Body).Decode(v)
}
