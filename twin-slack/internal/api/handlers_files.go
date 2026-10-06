package api

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// FilesGetUploadURLExternal handles POST /api/files.getUploadURLExternal
func (h *Handler) FilesGetUploadURLExternal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename string `json:"filename"`
		Length   int    `json:"length"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	id := h.store.Files.NextID()
	file := store.File{
		ID:      id,
		Name:    req.Filename,
		Title:   req.Filename,
		Size:    req.Length,
		User:    callerUserID(r),
		Created: h.store.Clock.Now().Unix(),
	}
	h.store.Files.Set(id, file)

	// Slack hands out a URL on its upload host. The emulator serves that step
	// itself, so the URL points back at whichever host the client called.
	slackOK(w, map[string]any{
		"upload_url": origin(r) + uploadPath + id,
		"file_id":    id,
	})
}

// FilesCompleteUploadExternal handles POST /api/files.completeUploadExternal
func (h *Handler) FilesCompleteUploadExternal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Files []struct {
			ID    string `json:"id"`
			Title string `json:"title,omitempty"`
		} `json:"files"`
		ChannelID string `json:"channel_id,omitempty"`
		Channels  string `json:"channels,omitempty"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	var share []string
	if req.ChannelID != "" {
		if _, ok := h.store.Channels.Get(req.ChannelID); !ok {
			slackError(w, "channel_not_found")
			return
		}
		share = append(share, req.ChannelID)
	}
	for _, c := range strings.Split(req.Channels, ",") {
		if c = strings.TrimSpace(c); c == "" {
			continue
		}
		if _, ok := h.store.Channels.Get(c); !ok {
			slackError(w, "invalid_channel")
			return
		}
		share = append(share, c)
	}

	files := make([]store.File, len(req.Files))
	for i, f := range req.Files {
		file, ok := h.store.Files.Get(f.ID)
		if !ok {
			slackError(w, "file_not_found")
			return
		}
		files[i] = file
	}

	completed := []map[string]any{}
	for i, f := range req.Files {
		file := files[i]
		if f.Title != "" {
			file.Title = f.Title
		}
		for _, c := range share {
			if !slices.Contains(file.Channels, c) {
				file.Channels = append(file.Channels, c)
			}
		}
		file.IsPublic = len(file.Channels) > 0
		file.Permalink = fmt.Sprintf("https://files.slack.com/%s/%s", h.store.Team.ID, f.ID)
		h.store.Files.Set(f.ID, file)
		completed = append(completed, map[string]any{"id": file.ID, "title": file.Title})
	}

	slackOK(w, map[string]any{"files": completed})
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
		slackArgsError(w, err)
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
		slackArgsError(w, err)
		return
	}

	if _, ok := h.store.Files.Get(req.File); !ok {
		slackError(w, "file_not_found")
		return
	}
	h.store.Files.Delete(req.File)
	slackOK(w, nil)
}
