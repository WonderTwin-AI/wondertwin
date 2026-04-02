package api

import (
	"fmt"
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
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
		slackError(w, "invalid_json")
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
		slackError(w, "invalid_json")
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

// FilesUploadLegacy handles POST /api/files.upload (deprecated but still in API)
func (h *Handler) FilesUploadLegacy(w http.ResponseWriter, r *http.Request) {
	// Legacy single-call upload — create file directly
	id := h.store.Files.NextID()
	now := h.store.Clock.Now().Unix()
	file := store.File{
		ID:       id,
		Name:     "upload",
		Title:    "upload",
		User:     "U_BOT",
		Created:  now,
		IsPublic: false,
	}
	h.store.Files.Set(id, file)
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

// UsersSetPhoto handles POST /api/users.setPhoto
func (h *Handler) UsersSetPhoto(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{
		"profile": map[string]any{
			"image_24":  "https://placekitten.com/24/24",
			"image_32":  "https://placekitten.com/32/32",
			"image_48":  "https://placekitten.com/48/48",
			"image_72":  "https://placekitten.com/72/72",
			"image_192": "https://placekitten.com/192/192",
			"image_512": "https://placekitten.com/512/512",
		},
	})
}

// UsersDeletePhoto handles POST /api/users.deletePhoto
func (h *Handler) UsersDeletePhoto(w http.ResponseWriter, r *http.Request) {
	slackOK(w, nil)
}

// --- oauth ---

// OAuthV2Access handles POST /api/oauth.v2.access
func (h *Handler) OAuthV2Access(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{
		"access_token": "xoxb-sim-token",
		"token_type":   "bot",
		"scope":        "chat:write,channels:read,users:read",
		"bot_user_id":  "U_BOT",
		"app_id":       "A_SIM",
		"team":         map[string]any{"name": h.store.Team.Name, "id": h.store.Team.ID},
		"authed_user":  map[string]any{"id": "U_BOT"},
	})
}

// --- dialog ---

// DialogOpen handles POST /api/dialog.open (legacy, prefer views.*)
func (h *Handler) DialogOpen(w http.ResponseWriter, r *http.Request) {
	slackOK(w, nil)
}
