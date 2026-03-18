package api

import (
	"encoding/json"
	"net/http"
)

// xeroJSON writes a JSON response with the Xero envelope style.
func xeroJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

// xeroError writes a Xero-formatted error response.
func xeroError(w http.ResponseWriter, status int, errType, message string) {
	xeroJSON(w, status, map[string]any{
		"ErrorNumber": status,
		"Type":        errType,
		"Message":     message,
		"Elements":    []any{},
	})
}
