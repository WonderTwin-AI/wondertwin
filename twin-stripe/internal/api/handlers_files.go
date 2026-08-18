package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// --- Files ---

// Stripe's per-file limit is 10 MB. MaxBytesReader bounds the entire multipart
// envelope — part boundaries, part headers and any other form fields — so the
// body cap carries headroom above the file limit. Capping the body at exactly
// 10 MB would reject a 10 MB file that real Stripe accepts, purely because of
// envelope overhead.
const (
	maxFileBytes       = 10 << 20
	maxUploadBodyBytes = maxFileBytes + (1 << 20)
)

// writeUploadParseError reports a file-upload parse failure. An over-cap body is
// reported as a size error rather than surfacing the raw reader message, so the
// multipart and urlencoded paths answer the same shape for the same condition.
func writeUploadParseError(w http.ResponseWriter, err error) {
	msg := err.Error()
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		msg = fmt.Sprintf("Request exceeds the maximum size of %d bytes.", maxUploadBodyBytes)
	}
	twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", msg)
}

func (h *Handler) CreateFile(w http.ResponseWriter, r *http.Request) {
	// Bound the body before any parsing so an oversized upload cannot exhaust
	// memory, whatever content type it claims (G120).
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBodyBytes)

	// Stripe files API uses multipart/form-data; parse it but discard file bytes.
	//nolint:gosec // G120: the body is already bounded by the MaxBytesReader above,
	// and this call passes an explicit in-memory limit.
	if err := r.ParseMultipartForm(maxFileBytes); err != nil {
		// Report an over-cap body here. Without this the fallback below calls
		// r.ParseForm(), which for multipart/form-data parses only the URL query
		// and returns nil — swallowing the size failure and answering "Missing
		// required param: purpose." for a request that did send one.
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeUploadParseError(w, err)
			return
		}
		// Fall back to regular form parsing
		if err2 := parseFormOrJSON(r); err2 != nil {
			writeUploadParseError(w, err2)
			return
		}
	}

	purpose := r.FormValue("purpose")
	if purpose == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required param: purpose.")
		return
	}

	id := h.store.Files.NextID()

	var filename string
	var size int64
	var fileType string
	if _, fh, err := r.FormFile("file"); err == nil {
		filename = fh.Filename
		size = fh.Size
		ct := fh.Header.Get("Content-Type")
		switch {
		case ct == "application/pdf":
			fileType = "pdf"
		case ct == "image/jpeg":
			fileType = "jpg"
		case ct == "image/png":
			fileType = "png"
		default:
			fileType = "csv"
		}
	}

	f := store.File{
		ID:       id,
		Object:   "file",
		Purpose:  purpose,
		Filename: filename,
		Size:     size,
		Type:     fileType,
		URL:      "https://files.stripe.com/v1/files/" + id,
		Livemode: false,
		Created:  h.store.Now(),
	}

	h.store.Files.Set(id, f)
	twincore.JSON(w, http.StatusOK, f)
}

func (h *Handler) GetFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	f, ok := h.store.Files.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such file: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, f)
}

func (h *Handler) ListFiles(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.Files.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/files", "has_more": page.HasMore, "data": page.Data,
	})
}

// --- File Links ---

func (h *Handler) CreateFileLink(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	fileID := r.FormValue("file")
	if fileID == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required param: file.")
		return
	}

	id := h.store.FileLinks.NextID()
	fl := store.FileLink{
		ID:       id,
		Object:   "file_link",
		File:     fileID,
		URL:      "https://files.stripe.com/links/" + id,
		Expired:  false,
		Livemode: false,
		Metadata: parseMetadata(r),
		Created:  h.store.Now(),
	}
	if v := r.FormValue("expires_at"); v != "" {
		fl.ExpiresAt, _ = strconv.ParseInt(v, 10, 64)
	}

	h.store.FileLinks.Set(id, fl)
	twincore.JSON(w, http.StatusOK, fl)
}

func (h *Handler) GetFileLink(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	fl, ok := h.store.FileLinks.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such file_link: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, fl)
}

func (h *Handler) UpdateFileLink(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	fl, ok := h.store.FileLinks.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such file_link: "+id)
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if v := r.FormValue("expires_at"); v != "" {
		fl.ExpiresAt, _ = strconv.ParseInt(v, 10, 64)
	}
	if meta := parseMetadata(r); len(meta) > 0 {
		fl.Metadata = meta
	}

	h.store.FileLinks.Set(id, fl)
	twincore.JSON(w, http.StatusOK, fl)
}

func (h *Handler) ListFileLinks(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.FileLinks.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/file_links", "has_more": page.HasMore, "data": page.Data,
	})
}
