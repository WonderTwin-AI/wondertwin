package api

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
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
	// Both are required by the docs.
	if req.Filename == "" {
		invalidArgument(w, "filename")
		return
	}
	if req.Length <= 0 {
		invalidArgument(w, "length")
		return
	}

	id := h.store.Files.NextID()
	mimeType, fileType := fileTypes(req.Filename)
	file := store.File{
		ID:       id,
		Name:     req.Filename,
		Title:    req.Filename,
		MimeType: mimeType,
		FileType: fileType,
		Size:     req.Length,
		User:     callerUserID(r),
		Created:  h.store.Clock.Now().Unix(),
	}
	// timestamp is the file's creation time, like created (the file object).
	file.Timestamp = file.Created
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
		ChannelID      string `json:"channel_id,omitempty"`
		Channels       string `json:"channels,omitempty"`
		ThreadTS       string `json:"thread_ts,omitempty"`
		InitialComment string `json:"initial_comment,omitempty"`
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
		// A file whose bytes never reached the upload URL cannot be completed.
		// The code is a guess from the documented list (see divergences.json).
		if !ok || (file.Content == nil && file.URLPrivate == "") {
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
		// The private URLs serve the bytes to a caller with a token, on the
		// host the client called, in the shape Slack's file object shows.
		base := origin(r) + filesPath + h.store.Team.ID + "-" + file.ID + "/"
		file.URLPrivate = base + url.PathEscape(file.Name)
		file.URLPrivateDownload = base + "download/" + url.PathEscape(file.Name)
		h.store.Files.Set(f.ID, file)
		files[i] = file
		completed = append(completed, map[string]any{"id": file.ID, "title": file.Title})
	}

	// Sharing posts a file_share message to each channel, carrying the files
	// and the initial comment as its text.
	for _, c := range share {
		msg := store.Message{
			Type:     "message",
			Subtype:  "file_share",
			Channel:  c,
			User:     callerUserID(r),
			Text:     req.InitialComment,
			TS:       h.store.NextTS(),
			ThreadTS: req.ThreadTS,
			Team:     h.store.Team.ID,
			Files:    files,
		}
		h.store.Messages.Set(h.store.Messages.NextID(), msg)
		h.emitMessage(msg)
	}

	slackOK(w, map[string]any{"files": completed})
}

// FilesList handles POST /api/files.list
func (h *Handler) FilesList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		User    string `json:"user"`
		Types   string `json:"types"`
		TSFrom  string `json:"ts_from"`
		TSTo    string `json:"ts_to"`
		Count   int    `json:"count"`
		Page    int    `json:"page"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if req.User != "" && !h.store.KnownUser(req.User) {
		slackError(w, "user_not_found")
		return
	}
	match, ok := fileTypeFilter(req.Types)
	if !ok {
		slackError(w, "unknown_type")
		return
	}
	from, fromErr := optionalUnix(req.TSFrom)
	to, toErr := optionalUnix(req.TSTo)
	if fromErr != nil || toErr != nil {
		slackError(w, "invalid_arguments")
		return
	}

	files := h.store.Files.Filter(func(_ string, f store.File) bool {
		switch {
		// A file whose upload was never completed is not listed: only
		// files.completeUploadExternal gives a file its private URL.
		case f.URLPrivate == "",
			req.Channel != "" && !slices.Contains(f.Channels, req.Channel),
			req.User != "" && f.User != req.User,
			!match(f),
			from > 0 && f.Created < from,
			to > 0 && f.Created > to:
			return false
		}
		return true
	})
	// Pages of count files, default 100, numbered from 1 (the files.list
	// docs); paging.count is the page size, as the docs' example shows.
	count, page := req.Count, req.Page
	if count <= 0 {
		count = 100
	}
	if page <= 0 {
		page = 1
	}
	total := len(files)
	pages := (total + count - 1) / count
	if pages == 0 {
		pages = 1
	}
	start := min((page-1)*count, total)
	end := min(start+count, total)
	pageFiles := append([]store.File{}, files[start:end]...)
	slackOK(w, map[string]any{
		"files": pageFiles,
		"paging": map[string]any{
			"count": count,
			"total": total,
			"page":  page,
			"pages": pages,
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

// fileTypes derives a file's mimetype and filetype from its name's extension.
func fileTypes(name string) (string, string) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if ext == "" {
		return "application/octet-stream", "binary"
	}
	mimeType := mime.TypeByExtension("." + ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	mimeType, _, _ = strings.Cut(mimeType, ";")
	if ext == "txt" {
		ext = "text"
	}
	return mimeType, ext
}

// fileTypeFilter matches files against files.list's types argument: a
// comma-separated list of the documented types, all by default.
func fileTypeFilter(types string) (func(store.File) bool, bool) {
	if types == "" {
		types = "all"
	}
	var wanted []func(store.File) bool
	for _, t := range strings.Split(types, ",") {
		switch strings.TrimSpace(t) {
		case "all":
			return func(store.File) bool { return true }, true
		case "images":
			wanted = append(wanted, func(f store.File) bool { return strings.HasPrefix(f.MimeType, "image/") })
		case "pdfs":
			wanted = append(wanted, func(f store.File) bool { return f.FileType == "pdf" })
		case "zips":
			wanted = append(wanted, func(f store.File) bool { return f.FileType == "zip" })
		case "spaces", "snippets", "gdocs":
			// Posts, snippets and Google docs are not created by the app emulator.
			wanted = append(wanted, func(store.File) bool { return false })
		default:
			return nil, false
		}
	}
	return func(f store.File) bool {
		for _, w := range wanted {
			if w(f) {
				return true
			}
		}
		return false
	}, true
}

// optionalUnix parses a unix timestamp argument, 0 when absent.
func optionalUnix(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	return int64(f), err
}
