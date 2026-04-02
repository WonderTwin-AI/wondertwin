package api

import "net/http"

// AuthTest handles POST /api/auth.test
func (h *Handler) AuthTest(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{
		"url":     "https://" + h.store.Team.Domain + ".slack.com/",
		"team":    h.store.Team.Name,
		"user":    "bot",
		"team_id": h.store.Team.ID,
		"user_id": "U_BOT",
		"bot_id":  "B_BOT",
	})
}

// AuthRevoke handles POST /api/auth.revoke
func (h *Handler) AuthRevoke(w http.ResponseWriter, r *http.Request) {
	slackOK(w, map[string]any{"revoked": true})
}
