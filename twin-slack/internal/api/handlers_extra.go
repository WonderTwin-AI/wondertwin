package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// --- chat.unfurl ---

// unfurlSources are the documented values of source.
var unfurlSources = map[string]bool{"composer": true, "conversations_history": true}

// ChatUnfurl handles POST /api/chat.unfurl. A call names a posted message with
// channel and ts, or a link in the composer with unfurl_id and source, and
// gives unfurls: a JSON map from each URL in the message to its unfurl. The
// emulator checks the call as the docs describe and acknowledges it; it does
// not show the unfurl on the message.
func (h *Handler) ChatUnfurl(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel  string `json:"channel"`
		TS       string `json:"ts"`
		Unfurls  any    `json:"unfurls"`
		UnfurlID string `json:"unfurl_id"`
		Source   string `json:"source"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if req.UnfurlID != "" || req.Source != "" {
		switch {
		case req.UnfurlID == "":
			slackError(w, "missing_unfurl_id")
		case req.Source == "":
			slackError(w, "missing_source")
		case !unfurlSources[req.Source]:
			slackError(w, "invalid_source")
		default:
			// The emulator sends no link_shared events, so it has issued
			// no unfurl ID for a call to name.
			slackError(w, "invalid_unfurl_id")
		}
		return
	}
	switch {
	case req.Channel == "":
		slackError(w, "missing_channel")
		return
	case req.TS == "":
		slackError(w, "missing_ts")
		return
	case req.Unfurls == nil || req.Unfurls == "":
		slackError(w, "missing_unfurls")
		return
	}
	if _, ok := h.store.Channels.Get(req.Channel); !ok {
		slackError(w, "cannot_find_channel")
		return
	}
	msg, _, ok := h.store.GetMessageByTS(req.Channel, req.TS)
	if !ok {
		slackError(w, "cannot_find_message")
		return
	}
	unfurls, ok := req.Unfurls.(map[string]any)
	if !ok {
		slackError(w, "invalid_unfurls_format")
		return
	}
	for link, unfurl := range unfurls {
		if _, ok := unfurl.(map[string]any); !ok {
			slackError(w, "invalid_unfurls_format")
			return
		}
		if !strings.Contains(msg.Text, link) {
			slackError(w, "cannot_unfurl_message")
			return
		}
	}
	slackOK(w, nil)
}

// --- files extra ---

// FilesSharedPublicURL handles POST /api/files.sharedPublicURL
func (h *Handler) FilesSharedPublicURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		File string `json:"file"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	file, ok := h.store.Files.Get(req.File)
	if !ok {
		slackError(w, "file_not_found")
		return
	}
	file.IsPublic = true
	file.Permalink = fmt.Sprintf("https://files.slack.com/%s/%s", h.store.Team.ID, req.File)
	h.store.Files.Set(req.File, file)
	slackOK(w, map[string]any{"file": file})
}

// FilesRevokePublicURL handles POST /api/files.revokePublicURL
func (h *Handler) FilesRevokePublicURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		File string `json:"file"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	file, ok := h.store.Files.Get(req.File)
	if !ok {
		slackError(w, "file_not_found")
		return
	}
	file.IsPublic = false
	file.Permalink = ""
	h.store.Files.Set(req.File, file)
	slackOK(w, map[string]any{"file": file})
}

// --- users extra ---

// UsersIdentity handles POST /api/users.identity, which Sign in with Slack
// calls with the user's own token. It answers the identity.basic fields: the
// user's name and ID, and the workspace ID.
func (h *Handler) UsersIdentity(w http.ResponseWriter, r *http.Request) {
	t := principal(r)
	if t.Type != store.TokenUser {
		slackError(w, "not_allowed_token_type")
		return
	}
	// A seeded token can speak for a user with no record; it is still that
	// user's identity.
	name := h.store.UserName(t)
	if u, ok := h.store.Users.Get(t.UserID); ok && u.RealName != "" {
		name = u.RealName
	}
	slackOK(w, map[string]any{
		"user": map[string]any{"name": name, "id": t.UserID},
		"team": map[string]any{"id": t.TeamID},
	})
}
