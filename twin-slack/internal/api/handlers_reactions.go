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
		slackArgsError(w, err)
		return
	}

	if req.Channel == "" || req.Timestamp == "" {
		slackError(w, "no_item_specified")
		return
	}
	if req.Name == "" {
		slackError(w, "invalid_name")
		return
	}
	msg, id, ok := h.messageIn(w, req.Channel, req.Timestamp)
	if !ok {
		return
	}
	reactor := callerUserID(r)

	// Check for existing reaction from same user
	for _, rx := range msg.Reactions {
		if rx.Name == req.Name {
			for _, u := range rx.Users {
				if u == reactor {
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
			msg.Reactions[i].Users = append(rx.Users, reactor)
			msg.Reactions[i].Count++
			found = true
			break
		}
	}
	if !found {
		msg.Reactions = append(msg.Reactions, store.Reaction{
			Name:  req.Name,
			Users: []string{reactor},
			Count: 1,
		})
	}

	h.store.Messages.Set(id, *msg)
	h.emitReactionAdded(reactor, req.Name, *msg)
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
		slackArgsError(w, err)
		return
	}

	msg, id, ok := h.store.GetMessageByTS(req.Channel, req.Timestamp)
	if !ok {
		slackError(w, "message_not_found")
		return
	}

	remover := callerUserID(r)
	removed := false
	reactions := make([]store.Reaction, 0, len(msg.Reactions))
	for _, rx := range msg.Reactions {
		if rx.Name == req.Name {
			users := make([]string, 0, len(rx.Users))
			for _, u := range rx.Users {
				if u != remover {
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
		slackArgsError(w, err)
		return
	}

	msg, _, ok := h.messageIn(w, req.Channel, req.Timestamp)
	if !ok {
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
		userID = callerUserID(r)
	}

	items := []any{}
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

// messageIn finds the message at ts in channel, answering channel_not_found
// for a channel that does not exist and message_not_found for a ts that is
// not in it.
func (h *Handler) messageIn(w http.ResponseWriter, channel, ts string) (*store.Message, string, bool) {
	if _, ok := h.store.Channels.Get(channel); !ok {
		slackError(w, "channel_not_found")
		return nil, "", false
	}
	msg, id, ok := h.store.GetMessageByTS(channel, ts)
	if !ok {
		slackError(w, "message_not_found")
		return nil, "", false
	}
	return msg, id, true
}

// callerUserID is the user the call's token speaks for: the one who reacts
// or pins.
func callerUserID(r *http.Request) string {
	if u := principal(r).UserID; u != "" {
		return u
	}
	return store.DefaultBotUserID
}
