package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// PinsAdd handles POST /api/pins.add
func (h *Handler) PinsAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel   string `json:"channel"`
		Timestamp string `json:"timestamp"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	if req.Timestamp == "" {
		slackError(w, "no_item_specified")
		return
	}
	msg, _, ok := h.messageIn(w, req.Channel, req.Timestamp)
	if !ok {
		return
	}
	if len(h.store.Pins.Filter(func(_ string, pin store.Pin) bool {
		return pin.Channel == req.Channel && pin.Message.TS == req.Timestamp
	})) > 0 {
		slackError(w, "already_pinned")
		return
	}

	id := h.store.Pins.NextID()
	h.store.Pins.Set(id, store.Pin{
		Type:    "message",
		Channel: req.Channel,
		Message: *msg,
		Created: h.store.Clock.Now().Unix(),
		Creator: callerUserID(r),
	})
	slackOK(w, nil)
}

// PinsRemove handles POST /api/pins.remove
func (h *Handler) PinsRemove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel   string `json:"channel"`
		Timestamp string `json:"timestamp"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}

	ids, pins := h.store.Pins.FilterWithIDs(func(id string, pin store.Pin) bool {
		return pin.Channel == req.Channel && pin.Message.TS == req.Timestamp
	})
	if len(ids) > 0 {
		h.store.Pins.Delete(ids[0])
		_ = pins
		slackOK(w, nil)
		return
	}
	slackError(w, "no_pin")
}

// PinsList handles POST /api/pins.list
func (h *Handler) PinsList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
	}
	parseJSON(r, &req)

	items := []store.Pin{}
	for _, pin := range h.store.Pins.List() {
		if req.Channel == "" || pin.Channel == req.Channel {
			items = append(items, pin)
		}
	}
	slackOK(w, map[string]any{"items": items})
}
