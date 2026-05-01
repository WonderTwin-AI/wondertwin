package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// --- Files ---

func (h *Handler) CreateFile(w http.ResponseWriter, r *http.Request) {
	// Stripe files API uses multipart/form-data; parse it but discard file bytes.
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		// Fall back to regular form parsing
		if err2 := parseFormOrJSON(r); err2 != nil {
			twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err2.Error())
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
