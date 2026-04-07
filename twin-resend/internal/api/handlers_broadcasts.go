package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// ListBroadcasts handles GET /broadcasts
func (h *Handler) ListBroadcasts(w http.ResponseWriter, r *http.Request) {
	broadcasts := h.store.Broadcasts.Filter(func(_ string, _ store.Broadcast) bool { return true })
	twincore.JSON(w, http.StatusOK, map[string]any{"object": "list", "data": broadcasts})
}

// GetBroadcast handles GET /broadcasts/{id}
func (h *Handler) GetBroadcast(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	bc, ok := h.store.Broadcasts.Get(id)
	if !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "Broadcast not found", "name": "not_found",
		})
		return
	}
	twincore.JSON(w, http.StatusOK, bc)
}

// CreateBroadcast handles POST /broadcasts
func (h *Handler) CreateBroadcast(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string   `json:"name"`
		AudienceID string   `json:"audience_id"`
		From       string   `json:"from"`
		Subject    string   `json:"subject"`
		ReplyTo    []string `json:"reply_to"`
		HTML       string   `json:"html"`
		Text       string   `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AudienceID == "" || req.From == "" || req.Subject == "" {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"statusCode": 400, "message": "audience_id, from, and subject are required", "name": "validation_error",
		})
		return
	}

	if _, ok := h.store.Audiences.Get(req.AudienceID); !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "Audience not found", "name": "not_found",
		})
		return
	}

	id := resendID()
	bc := store.Broadcast{
		ID:         id,
		Object:     "broadcast",
		Name:       req.Name,
		AudienceID: req.AudienceID,
		From:       req.From,
		Subject:    req.Subject,
		ReplyTo:    req.ReplyTo,
		HTML:       req.HTML,
		Text:       req.Text,
		Status:     "draft",
		CreatedAt:  h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z"),
	}

	h.store.Broadcasts.Set(id, bc)
	twincore.JSON(w, http.StatusCreated, bc)
}

// SendBroadcast handles POST /broadcasts/{id}/send
func (h *Handler) SendBroadcast(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	bc, ok := h.store.Broadcasts.Get(id)
	if !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "Broadcast not found", "name": "not_found",
		})
		return
	}

	if bc.Status != "draft" {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"statusCode": 400, "message": "Broadcast has already been sent", "name": "validation_error",
		})
		return
	}

	now := h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z")
	bc.Status = "sent"
	bc.SentAt = now
	h.store.Broadcasts.Set(id, bc)
	twincore.JSON(w, http.StatusOK, bc)
}

// DeleteBroadcast handles DELETE /broadcasts/{id}
func (h *Handler) DeleteBroadcast(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := h.store.Broadcasts.Get(id); !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "Broadcast not found", "name": "not_found",
		})
		return
	}
	h.store.Broadcasts.Delete(id)
	twincore.JSON(w, http.StatusOK, map[string]any{"object": "broadcast", "id": id, "deleted": true})
}
