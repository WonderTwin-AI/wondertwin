package api

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// uploadPath is where files.getUploadURLExternal points a client. Slack's own
// upload URL has this shape on its upload host.
const uploadPath = "/upload/v1/"

// maxUploadBytes bounds an uploaded file. It matches the request body cap
// twincore's server puts on every request, which would refuse a larger body
// first.
var maxUploadBytes int64 = 10 << 20

var (
	errUploadTooLarge = errors.New("upload too large")
	errNoFilePart     = errors.New("no file part")
)

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
	var maxBytesErr *http.MaxBytesError
	switch {
	case errors.Is(err, errUploadTooLarge), errors.As(err, &maxBytesErr):
		http.Error(w, "upload too large", http.StatusRequestEntityTooLarge)
		return
	case errors.Is(err, errNoFilePart):
		// Our status code; how Slack's upload host answers this is unverified.
		http.Error(w, "no file part in form", http.StatusBadRequest)
		return
	case err != nil:
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
	body, err := io.ReadAll(io.LimitReader(r.Body, maxUploadBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxUploadBytes {
		return nil, errUploadTooLarge
	}
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		return body, nil
	}
	mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return nil, errNoFilePart
		}
		if err != nil {
			return nil, err
		}
		if part.FileName() != "" {
			return io.ReadAll(part)
		}
	}
}

// filesPath is where a file's private URLs point, as on Slack's files host:
// /files-pri/{team}-{file}/{name}, and .../download/{name} to download it.
const filesPath = "/files-pri/"

// ServeFile answers a file's url_private or url_private_download with its
// bytes. Like Slack's, the URLs need a token in the Authorization header; a
// call without a usable one is refused with HTTP 403 (how Slack's files host
// answers it is not in the references). A download is sent as an attachment.
func (h *Handler) ServeFile(download bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { h.serveFile(w, r, download) }
}

func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, download bool) {
	token, present, malformed := requestToken(r)
	if !present || malformed {
		http.Error(w, "a token is required", http.StatusForbidden)
		return
	}
	if _, status := h.store.ResolveToken(token); status != store.TokenOK {
		http.Error(w, "the token cannot read this file", http.StatusForbidden)
		return
	}
	id, ok := strings.CutPrefix(chi.URLParam(r, "teamFile"), h.store.Team.ID+"-")
	file, found := h.store.Files.Get(id)
	name, err := url.PathUnescape(chi.URLParam(r, "name"))
	if !ok || !found || err != nil || file.URLPrivate == "" || file.Name != name {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	if file.MimeType != "" {
		w.Header().Set("Content-Type", file.MimeType)
	}
	if download {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
	}
	_, _ = w.Write(file.Content)
}
