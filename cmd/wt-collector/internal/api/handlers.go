package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/wondertwin-ai/wondertwin/cmd/wt-collector/internal/store"
)

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleIngest(w http.ResponseWriter, r *http.Request) {
	var events []store.RawEvent
	if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if len(events) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty batch"})
		return
	}

	inserted, err := h.events.InsertBatch(events)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"accepted": inserted,
		"total":    len(events),
	})
}

func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	total, err := h.events.Count()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	byTwin, err := h.events.CountByTwin()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total_events": total,
		"by_twin":      byTwin,
	})
}

func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != h.ingestKey {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid ingest key"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
