// Package api implements the Slack Web API-compatible HTTP handlers for the twin.
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// Handler holds Slack API state.
type Handler struct {
	store *store.MemoryStore
	mw    *twincore.Middleware
}

// NewHandler creates a new Slack API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
	return &Handler{store: s, mw: mw}
}

// Routes mounts the Slack Web API-compatible routes.
// Methods live at /api/{method}, for example /api/chat.postMessage, and answer
// both GET and POST. Arguments are decoded once, by argsMiddleware, whichever
// way the client sent them; a request that cannot be decoded is reported after
// authentication. A name that is not a method, whether Slack retired it or never
// had it, answers unknown_method before authentication, so it sits outside the
// group that carries the authentication middleware.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/api", func(r chi.Router) {
		r.NotFound(unknownMethod)
		// api.test and oauth.v2.access answer without a token, so they sit
		// outside the group that authenticates.
		r.Group(func(r chi.Router) {
			r.Use(argsMiddleware)
			r.Use(argsErrorMiddleware)
			route(r, "api.test", h.APITest)
			// oauth.v2.access is how an app gets its first token, so it is
			// called without one; the client credentials authenticate it.
			route(r.With(h.mw.FaultInjection), "oauth.v2.access", h.OAuthV2Access)
		})
		r.Group(func(r chi.Router) {
			r.Use(argsMiddleware)
			r.Use(h.authMiddleware)
			r.Use(argsErrorMiddleware)
			r.Use(h.mw.FaultInjection)

			// auth.*
			route(r, "auth.test", h.AuthTest)
			route(r, "auth.revoke", h.AuthRevoke)

			// chat.*
			route(r, "chat.postMessage", h.ChatPostMessage)
			route(r, "chat.postEphemeral", h.ChatPostEphemeral)
			route(r, "chat.update", h.ChatUpdate)
			route(r, "chat.delete", h.ChatDelete)
			route(r, "chat.getPermalink", h.ChatGetPermalink)
			route(r, "chat.scheduleMessage", h.ChatScheduleMessage)
			route(r, "chat.deleteScheduledMessage", h.ChatDeleteScheduledMessage)
			route(r, "chat.scheduledMessages.list", h.ChatScheduledMessagesList)
			route(r, "chat.meMessage", h.ChatMeMessage)
			route(r, "chat.unfurl", h.ChatUnfurl)

			// conversations.*
			route(r, "conversations.list", h.ConversationsList)
			route(r, "conversations.info", h.ConversationsInfo)
			route(r, "conversations.history", h.ConversationsHistory)
			route(r, "conversations.replies", h.ConversationsReplies)
			route(r, "conversations.members", h.ConversationsMembers)
			route(r, "conversations.create", h.ConversationsCreate)
			route(r, "conversations.archive", h.ConversationsArchive)
			route(r, "conversations.unarchive", h.ConversationsUnarchive)
			route(r, "conversations.rename", h.ConversationsRename)
			route(r, "conversations.setPurpose", h.ConversationsSetPurpose)
			route(r, "conversations.setTopic", h.ConversationsSetTopic)
			route(r, "conversations.invite", h.ConversationsInvite)
			route(r, "conversations.kick", h.ConversationsKick)
			route(r, "conversations.join", h.ConversationsJoin)
			route(r, "conversations.leave", h.ConversationsLeave)
			route(r, "conversations.open", h.ConversationsOpen)
			route(r, "conversations.close", h.ConversationsClose)
			route(r, "conversations.mark", h.ConversationsMark)

			// users.*
			route(r, "users.list", h.UsersList)
			route(r, "users.info", h.UsersInfo)
			route(r, "users.lookupByEmail", h.UsersLookupByEmail)
			route(r, "users.conversations", h.UsersConversations)
			route(r, "users.profile.get", h.UsersProfileGet)
			route(r, "users.profile.set", h.UsersProfileSet)
			route(r, "users.getPresence", h.UsersGetPresence)
			route(r, "users.setPresence", h.UsersSetPresence)
			route(r, "users.identity", h.UsersIdentity)
			route(r, "users.setPhoto", h.UsersSetPhoto)
			route(r, "users.deletePhoto", h.UsersDeletePhoto)

			// reactions.*
			route(r, "reactions.add", h.ReactionsAdd)
			route(r, "reactions.remove", h.ReactionsRemove)
			route(r, "reactions.get", h.ReactionsGet)
			route(r, "reactions.list", h.ReactionsList)

			// pins.*
			route(r, "pins.add", h.PinsAdd)
			route(r, "pins.remove", h.PinsRemove)
			route(r, "pins.list", h.PinsList)

			// files.*
			route(r, "files.getUploadURLExternal", h.FilesGetUploadURLExternal)
			route(r, "files.completeUploadExternal", h.FilesCompleteUploadExternal)
			route(r, "files.list", h.FilesList)
			route(r, "files.info", h.FilesInfo)
			route(r, "files.delete", h.FilesDelete)
			route(r, "files.sharedPublicURL", h.FilesSharedPublicURL)
			route(r, "files.revokePublicURL", h.FilesRevokePublicURL)
			route(r, "files.upload", h.FilesUploadLegacy)

			// bookmarks.*
			route(r, "bookmarks.add", h.BookmarksAdd)
			route(r, "bookmarks.edit", h.BookmarksEdit)
			route(r, "bookmarks.list", h.BookmarksList)
			route(r, "bookmarks.remove", h.BookmarksRemove)

			// reminders.*
			route(r, "reminders.add", h.RemindersAdd)
			route(r, "reminders.complete", h.RemindersComplete)
			route(r, "reminders.delete", h.RemindersDelete)
			route(r, "reminders.info", h.RemindersInfo)
			route(r, "reminders.list", h.RemindersList)

			// views.*
			route(r, "views.open", h.ViewsOpen)
			route(r, "views.push", h.ViewsPush)
			route(r, "views.update", h.ViewsUpdate)
			route(r, "views.publish", h.ViewsPublish)

			// emoji.*
			route(r, "emoji.list", h.EmojiList)

			// team.*
			route(r, "team.info", h.TeamInfo)
			route(r, "team.accessLogs", h.TeamAccessLogs)
			route(r, "team.billableInfo", h.TeamBillableInfo)
			route(r, "team.integrationLogs", h.TeamIntegrationLogs)
			route(r, "team.profile.get", h.TeamProfileGet)

			// bots.*
			route(r, "bots.info", h.BotsInfo)

			// usergroups.*
			route(r, "usergroups.list", h.UsergroupsList)
			route(r, "usergroups.create", h.UsergroupsCreate)
			route(r, "usergroups.update", h.UsergroupsUpdate)
			route(r, "usergroups.disable", h.UsergroupsDisable)
			route(r, "usergroups.enable", h.UsergroupsEnable)
			route(r, "usergroups.users.list", h.UsergroupsUsersList)
			route(r, "usergroups.users.update", h.UsergroupsUsersUpdate)

			// dnd.*
			route(r, "dnd.info", h.DndInfo)
			route(r, "dnd.setSnooze", h.DndSetSnooze)
			route(r, "dnd.endSnooze", h.DndEndSnooze)
			route(r, "dnd.endDnd", h.DndEndDnd)
			route(r, "dnd.teamInfo", h.DndTeamInfo)

			// search.*
			route(r, "search.messages", h.SearchMessages)
			route(r, "search.files", h.SearchFiles)
			route(r, "search.all", h.SearchAll)

			// stars.*
			route(r, "stars.add", h.StarsAdd)
			route(r, "stars.remove", h.StarsRemove)
			route(r, "stars.list", h.StarsList)

			// dialog.*
			route(r, "dialog.open", h.DialogOpen)
		})
	})

	// The upload step of the external file upload. It lives outside /api,
	// as it does on Slack's upload host, and the URL itself is the credential.
	r.Post(uploadPath+"{fileID}", h.UploadFileBytes)

	// Admin extras (no auth required)
	r.Get("/admin/messages", h.AdminListMessages)
	r.Get("/admin/channels", h.AdminListChannels)
	r.Get("/admin/users", h.AdminListUsers)
}

// jsonContentType is the content type of every Web API answer.
const jsonContentType = "application/json; charset=utf-8"

// slackOK writes a successful Slack API response.
func slackOK(w http.ResponseWriter, fields map[string]any) {
	resp := map[string]any{"ok": true}
	for k, v := range fields {
		resp[k] = v
	}
	w.Header().Set("Content-Type", jsonContentType)
	json.NewEncoder(w).Encode(resp)
}

// slackError writes a Slack API error response. Slack answers API errors with
// HTTP 200, and repeats the error code in the x-slack-failure header.
func slackError(w http.ResponseWriter, code string) {
	slackErrorWith(w, code, nil)
}

// slackErrorWith writes an error response with extra top-level fields, such as
// the req_method of unknown_method.
func slackErrorWith(w http.ResponseWriter, code string, extra map[string]any) {
	resp := map[string]any{"ok": false, "error": code}
	for k, v := range extra {
		resp[k] = v
	}
	w.Header().Set("Content-Type", jsonContentType)
	w.Header().Set("X-Slack-Failure", code)
	json.NewEncoder(w).Encode(resp)
}

// unknownMethod answers a path under /api that is not a method this emulator
// serves. Slack answers a retired method and one that never existed the same
// way, with HTTP 200, and does so before it looks at any token or argument.
func unknownMethod(w http.ResponseWriter, r *http.Request) {
	slackErrorWith(w, "unknown_method", map[string]any{
		"req_method": strings.TrimPrefix(r.URL.Path, "/api/"),
	})
}
