package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// AdminListMessages handles GET /admin/messages
func (h *Handler) AdminListMessages(w http.ResponseWriter, r *http.Request) {
	messages := h.store.Messages.List()
	// Filter out deleted
	var active []any
	for _, m := range messages {
		if !m.IsDeleted {
			active = append(active, m)
		}
	}
	twincore.JSON(w, 200, map[string]any{
		"messages": active,
		"total":    len(active),
	})
}

// AdminListChannels handles GET /admin/channels
func (h *Handler) AdminListChannels(w http.ResponseWriter, r *http.Request) {
	channels := h.store.Channels.List()
	twincore.JSON(w, 200, map[string]any{
		"channels": channels,
		"total":    len(channels),
	})
}

// AdminListUsers handles GET /admin/users
func (h *Handler) AdminListUsers(w http.ResponseWriter, r *http.Request) {
	users := h.store.Users.List()
	twincore.JSON(w, 200, map[string]any{
		"users": users,
		"total": len(users),
	})
}
