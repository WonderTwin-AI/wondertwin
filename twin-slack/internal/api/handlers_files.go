package api

import (
	"fmt"
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// FilesGetUploadURLExternal handles POST /api/files.getUploadURLExternal
func (h *Handler) FilesGetUploadURLExternal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename string `json:"filename"`
		Length   int    `json:"length"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}

	id := h.store.Files.NextID()
	file := store.File{
		ID:       id,
		Name:     req.Filename,
		Title:    req.Filename,
		Size:     req.Length,
		User:     "U_BOT",
		Created:  h.store.Clock.Now().Unix(),
	}
	h.store.Files.Set(id, file)

	slackOK(w, map[string]any{
		"upload_url": fmt.Sprintf("https://files.slack.com/upload/v1/%s", id),
		"file_id":    id,
	})
}

// FilesCompleteUploadExternal handles POST /api/files.completeUploadExternal
func (h *Handler) FilesCompleteUploadExternal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Files   []struct {
			ID    string `json:"id"`
			Title string `json:"title,omitempty"`
		} `json:"files"`
		ChannelID string `json:"channel_id,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}

	var files []store.File
	for _, f := range req.Files {
		file, ok := h.store.Files.Get(f.ID)
		if !ok {
			slackError(w, "file_not_found")
			return
		}
		if f.Title != "" {
			file.Title = f.Title
		}
		if req.ChannelID != "" {
			file.Channels = []string{req.ChannelID}
		}
		file.IsPublic = true
		file.Permalink = fmt.Sprintf("https://files.slack.com/%s/%s", h.store.Team.ID, f.ID)
		h.store.Files.Set(f.ID, file)
		files = append(files, file)
	}

	slackOK(w, map[string]any{"files": files})
}

// FilesList handles POST /api/files.list
func (h *Handler) FilesList(w http.ResponseWriter, r *http.Request) {
	files := h.store.Files.List()
	slackOK(w, map[string]any{
		"files": files,
		"paging": map[string]any{
			"count": len(files),
			"total": len(files),
			"page":  1,
			"pages": 1,
		},
	})
}

// FilesInfo handles POST /api/files.info
func (h *Handler) FilesInfo(w http.ResponseWriter, r *http.Request) {
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
	slackOK(w, map[string]any{"file": file})
}

// FilesDelete handles POST /api/files.delete
func (h *Handler) FilesDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		File string `json:"file"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}

	if _, ok := h.store.Files.Get(req.File); !ok {
		slackError(w, "file_not_found")
		return
	}
	h.store.Files.Delete(req.File)
	slackOK(w, nil)
}
