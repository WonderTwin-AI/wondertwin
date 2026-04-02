package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// ReactionsAdd handles POST /api/reactions.add
func (h *Handler) ReactionsAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel   string `json:"channel"`
		Timestamp string `json:"timestamp"`
		Name      string `json:"name"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}

	msg, id, ok := h.store.GetMessageByTS(req.Channel, req.Timestamp)
	if !ok {
		slackError(w, "message_not_found")
		return
	}

	// Check for existing reaction from same user
	for _, rx := range msg.Reactions {
		if rx.Name == req.Name {
			for _, u := range rx.Users {
				if u == "U_BOT" {
					slackError(w, "already_reacted")
					return
				}
			}
		}
	}

	// Add or update reaction
	found := false
	for i, rx := range msg.Reactions {
		if rx.Name == req.Name {
			msg.Reactions[i].Users = append(rx.Users, "U_BOT")
			msg.Reactions[i].Count++
			found = true
			break
		}
	}
	if !found {
		msg.Reactions = append(msg.Reactions, store.Reaction{
			Name:  req.Name,
			Users: []string{"U_BOT"},
			Count: 1,
		})
	}

	h.store.Messages.Set(id, *msg)
	slackOK(w, nil)
}

// ReactionsRemove handles POST /api/reactions.remove
func (h *Handler) ReactionsRemove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel   string `json:"channel"`
		Timestamp string `json:"timestamp"`
		Name      string `json:"name"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}

	msg, id, ok := h.store.GetMessageByTS(req.Channel, req.Timestamp)
	if !ok {
		slackError(w, "message_not_found")
		return
	}

	removed := false
	reactions := make([]store.Reaction, 0, len(msg.Reactions))
	for _, rx := range msg.Reactions {
		if rx.Name == req.Name {
			users := make([]string, 0, len(rx.Users))
			for _, u := range rx.Users {
				if u != "U_BOT" {
					users = append(users, u)
				} else {
					removed = true
				}
			}
			if len(users) > 0 {
				rx.Users = users
				rx.Count = len(users)
				reactions = append(reactions, rx)
			}
		} else {
			reactions = append(reactions, rx)
		}
	}

	if !removed {
		slackError(w, "no_reaction")
		return
	}

	msg.Reactions = reactions
	h.store.Messages.Set(id, *msg)
	slackOK(w, nil)
}

// ReactionsGet handles POST /api/reactions.get
func (h *Handler) ReactionsGet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel   string `json:"channel"`
		Timestamp string `json:"timestamp"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackError(w, "invalid_json")
		return
	}

	msg, _, ok := h.store.GetMessageByTS(req.Channel, req.Timestamp)
	if !ok {
		slackError(w, "message_not_found")
		return
	}

	slackOK(w, map[string]any{
		"type":    "message",
		"message": msg,
	})
}

// ReactionsList handles POST /api/reactions.list
func (h *Handler) ReactionsList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User string `json:"user"`
	}
	parseJSON(r, &req)

	userID := req.User
	if userID == "" {
		userID = "U_BOT"
	}

	var items []any
	for _, msg := range h.store.Messages.List() {
		for _, rx := range msg.Reactions {
			for _, u := range rx.Users {
				if u == userID {
					items = append(items, map[string]any{
						"type":    "message",
						"message": msg,
					})
					break
				}
			}
		}
	}

	slackOK(w, map[string]any{
		"items": items,
		"response_metadata": map[string]any{
			"next_cursor": "",
		},
	})
}
