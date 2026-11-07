package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// writeError answers Resend's error envelope, {statusCode, message, name},
// with the HTTP status equal to statusCode, as Resend does on every error.
func writeError(w http.ResponseWriter, status int, name, message string) {
	twincore.JSON(w, status, map[string]any{
		"statusCode": status,
		"message":    message,
		"name":       name,
	})
}

// notFound answers an unknown endpoint. Resend authenticates before routing,
// so a request without a key gets 401 missing_api_key, not 404. Admin paths
// are the emulator's own and need no key.
func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	if !isAdminPath(r.URL.Path) && !h.authenticate(w, r) {
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "The requested endpoint does not exist.")
}

// methodNotAllowed answers a known path with a method it does not serve,
// after authentication.
func (h *Handler) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	if !isAdminPath(r.URL.Path) && !h.authenticate(w, r) {
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed",
		"Method "+r.Method+" is not allowed for the requested path.")
}

// missingField answers a missing required field the way Resend does on
// POST /emails: 422 validation_error naming the field (quirk envelope-006).
func missingField(w http.ResponseWriter, field string) {
	writeError(w, http.StatusUnprocessableEntity, "validation_error", "Missing `"+field+"` field.")
}

func isAdminPath(path string) bool {
	return path == "/admin" || strings.HasPrefix(path, "/admin/")
}

// decodeBody decodes a JSON request body into v and answers the error itself
// when it cannot. A body that is not JSON at all answers 400, as Resend does;
// JSON of the wrong shape answers 422 validation_error.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	err := json.NewDecoder(r.Body).Decode(v)
	if err == nil {
		return true
	}
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		writeError(w, http.StatusBadRequest, "validation_error", "Request body must be valid JSON.")
		return false
	}
	writeError(w, http.StatusUnprocessableEntity, "validation_error", "Invalid request body: "+err.Error())
	return false
}
