package api

import (
	"encoding/json"
	"net/http"
	"time"
)

// qboJSON writes a QBO-style JSON response with a top-level time field.
func qboJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

// entityResponse wraps a single entity in QBO's response envelope.
// QBO returns {"Invoice": {...}, "time": "..."} for single entities.
func entityResponse(entityName string, entity any) map[string]any {
	return map[string]any{
		entityName: entity,
		"time":     time.Now().Format(time.RFC3339),
	}
}

// queryResponse wraps a query result in QBO's QueryResponse envelope.
func queryResponse(entityName string, entities any, start, max, total int) map[string]any {
	qr := map[string]any{
		entityName:      entities,
		"startPosition": start,
		"maxResults":    max,
	}
	if total >= 0 {
		qr["totalCount"] = total
	}
	return map[string]any{
		"QueryResponse": qr,
		"time":          time.Now().Format(time.RFC3339),
	}
}

// qboFault writes a QBO Fault error response.
func qboFault(w http.ResponseWriter, status int, faultType, code, message, detail string) {
	qboJSON(w, status, map[string]any{
		"Fault": map[string]any{
			"type": faultType,
			"Error": []map[string]any{{
				"Message": message,
				"Detail":  detail,
				"code":    code,
				"element": "",
			}},
		},
		"time": time.Now().Format(time.RFC3339),
	})
}

// validationFault is a shorthand for validation errors.
func validationFault(w http.ResponseWriter, code, message, detail string) {
	qboFault(w, http.StatusBadRequest, "ValidationFault", code, message, detail)
}

// staleSyncTokenFault returns the standard stale object error.
func staleSyncTokenFault(w http.ResponseWriter) {
	qboFault(w, http.StatusBadRequest, "ValidationFault", "5010",
		"Stale Object Error",
		"Object has been modified. Please refresh and try again.")
}

// notFoundFault returns a 404 with QBO error format.
func notFoundFault(w http.ResponseWriter, entityType, id string) {
	qboFault(w, http.StatusBadRequest, "ValidationFault", "610",
		"Object Not Found",
		entityType+" with Id "+id+" not found.")
}
