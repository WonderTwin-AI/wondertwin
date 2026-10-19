package api

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// contentType is what Plaid answers with, success or error.
const contentType = "application/json; charset=utf-8"

// requestID is 15 lowercase hex characters, in the body only (quirks
// ENVELOPE-001, envelope-009). It comes from the store's seeded Rand so a
// seeded run reissues the same ids.
func (h *Handler) requestID() string {
	return h.store.NewHex(15)
}

// writeJSON answers with v as a JSON object plus request_id, pretty-printed
// with 2-space indentation and alphabetical keys, and no trailing newline
// (quirk ENVELOPE-005). Webhook body hashing relies on the same layout.
//
// v is marshalled first, so the caller's struct tags decide which fields are
// present: optional non-nullable fields omitted, required nullable fields
// written as null (quirk ENVELOPE-004). Going through a map is what sorts the
// keys; numbers are kept as written so large ids and amounts survive.
func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "API_ERROR", "INTERNAL_SERVER_ERROR",
			"an unexpected error occurred")
		return
	}
	obj := map[string]any{}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&obj); err != nil {
		h.writeError(w, http.StatusInternalServerError, "API_ERROR", "INTERNAL_SERVER_ERROR",
			"an unexpected error occurred")
		return
	}
	obj["request_id"] = h.requestID()
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		out = []byte(`{"request_id":"` + h.requestID() + `"}`)
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(out)
}
