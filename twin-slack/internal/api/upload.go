package api

import (
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// uploadPath is where files.getUploadURLExternal points a client. Slack's own
// upload URL has this shape on its upload host.
const uploadPath = "/upload/v1/"

// maxUploadBytes bounds an uploaded file.
const maxUploadBytes = 64 << 20

// origin is the scheme and host the client called, so a URL handed back to it
// reaches this emulator however it is fronted.
func origin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = strings.TrimSpace(strings.Split(p, ",")[0])
	}
	host := r.Host
	if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
		host = strings.TrimSpace(strings.Split(fh, ",")[0])
	}
	return scheme + "://" + host
}

// UploadFileBytes receives the bytes of a file announced by
// files.getUploadURLExternal. Slack accepts raw bytes or a multipart form, and
// answers HTTP 200 on success and anything else on failure. The URL is the
// credential, so no token is read.
func (h *Handler) UploadFileBytes(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "fileID")
	file, ok := h.store.Files.Get(id)
	if !ok {
		http.Error(w, "upload ticket not found", http.StatusNotFound)
		return
	}
	data, err := readUpload(r)
	if err != nil {
		http.Error(w, "unreadable upload", http.StatusBadRequest)
		return
	}
	file.Content = data
	file.Size = len(data)
	h.store.Files.Set(id, file)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "OK")
}

// readUpload returns the file's bytes: the first file part of a multipart form,
// or the whole body otherwise.
func readUpload(r *http.Request) ([]byte, error) {
	body := io.LimitReader(r.Body, maxUploadBytes)
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		return io.ReadAll(body)
	}
	mr := multipart.NewReader(body, params["boundary"])
	var fallback []byte
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return fallback, nil
		}
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(part)
		if err != nil {
			return nil, err
		}
		if part.FileName() != "" {
			return data, nil
		}
		if fallback == nil {
			fallback = data
		}
	}
}
