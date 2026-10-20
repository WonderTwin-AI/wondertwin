package api

import (
	"net/http"
	"strings"
)

// writeError answers with Plaid's flat error object: the keys at the root, not
// under an "error" key (quirk ENVELOPE-002). Seven keys are sent. The others
// the spec lists, error_code_reason, causes and status, are left out
// (quirk ENVELOPE-006); display_message and suggested_action are null.
func (h *Handler) writeError(w http.ResponseWriter, status int, errType, code, message string) {
	h.writeJSON(w, status, map[string]any{
		"display_message":   nil,
		"documentation_url": documentationURL(errType, code),
		"error_code":        code,
		"error_message":     message,
		"error_type":        errType,
		"suggested_action":  nil,
	})
}

// documentationURL is https://plaid.com/docs/errors/{type-slug}/#{code-lowercase},
// as every probed error carried it.
func documentationURL(errType, code string) string {
	slug := strings.ToLower(strings.ReplaceAll(errType, "_", "-"))
	return "https://plaid.com/docs/errors/" + slug + "/#" + strings.ToLower(code)
}

// notFound answers a route that does not exist. A wrong method, an unknown
// path and a trailing slash are all this 404, never a 405 (quirk ENVELOPE-007).
func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	h.writeError(w, http.StatusNotFound, "INVALID_REQUEST", "NOT_FOUND",
		"invalid route: "+r.URL.Path)
}

// writeMissing answers 400 MISSING_FIELDS naming the missing fields.
func (h *Handler) writeMissing(w http.ResponseWriter, fields []string) {
	h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "MISSING_FIELDS",
		"the following required fields are missing: "+strings.Join(fields, ", "))
}
