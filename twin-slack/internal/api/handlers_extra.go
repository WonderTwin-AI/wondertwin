package api

import (
	"fmt"
	"net/http"
)

// --- chat.unfurl ---

// ChatUnfurl handles POST /api/chat.unfurl
func (h *Handler) ChatUnfurl(w http.ResponseWriter, r *http.Request) {
	// Accept the unfurl payload and acknowledge
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

// UsersIdentity handles POST /api/users.identity (Sign in with Slack)
func (h *Handler) UsersIdentity(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{
		"user": map[string]any{
			"name":  "twin-bot",
			"id":    "U_BOT",
			"email": "bot@wondertwin.dev",
		},
		"team": map[string]any{
			"id": h.store.Team.ID,
		},
	})
}
