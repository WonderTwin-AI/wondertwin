package api

import (
	"bytes"
	"errors"
	"net/http"
)

// Legacy capture (/capture, /e, /batch) answers in the capture service's own
// forms, not the JSON error object the docs show: a 200 {"status":"Ok"} on
// success and short text/plain bodies on rejection (ph-br-004, ph-ee-014,
// ph-ee-007). Only these routes use them. /decide and /flags keep the JSON
// error object (noKeyError), which is what the vendor's feature-flag service
// answers.
const (
	msgNoAPIKey     = "event submitted without an api_key"
	msgNoPayload    = "request missing data payload"
	msgNoEventName  = "failed to hydrate events from request: non-engage request missing event name attribute"
	msgParsePrefix  = "failed to parse request: "
	msgBodyTooLarge = "request body exceeds the maximum ingest size"
)

// ingestText writes a text/plain capture rejection.
func ingestText(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg))
}

// ingestOK writes the capture success body.
func ingestOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"Ok"}`))
}

// ingestBodyError maps a body read or decode failure to the capture service's
// text answer: an oversized body keeps the size message, and anything else is
// a parse failure. An empty body is caught separately by ingestEmptyBody.
func ingestBodyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errIngestBodyTooLarge):
		ingestText(w, http.StatusBadRequest, msgBodyTooLarge)
	default:
		ingestText(w, http.StatusBadRequest, msgParsePrefix+err.Error())
	}
}

// ingestEmptyBody reports whether a decoded body holds no payload at all.
func ingestEmptyBody(body []byte) bool {
	return len(bytes.TrimSpace(body)) == 0
}
