package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// adminEmailEvents are the events POST /admin/emails/{id}/events/{event}
// can trigger: the ones a send never produces on its own in the emulator
// (architecture-spec.md admin namespace).
var adminEmailEvents = []string{
	store.EmailStatusOpened, store.EmailStatusClicked,
	store.EmailStatusDeliveryDelayed, store.EmailStatusFailed,
}

// AdminEmailEvent handles POST /admin/emails/{id}/events/{event}: it sets the
// email's last_event and delivers email.{event} to subscribed webhooks. A
// clicked event takes an optional body {"link": "..."}. It needs no API key.
func (h *Handler) AdminEmailEvent(w http.ResponseWriter, r *http.Request) {
	event := chi.URLParam(r, "event")
	if !slices.Contains(adminEmailEvents, event) {
		writeError(w, http.StatusBadRequest, "validation_error",
			"event must be one of opened, clicked, delivery_delayed or failed")
		return
	}
	e, ok := h.store.Emails.Get(chi.URLParam(r, "id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "Email not found")
		return
	}
	e = h.settledEmail(e)
	if e.LastEvent == store.EmailStatusScheduled || e.LastEvent == store.EmailStatusCanceled {
		writeError(w, http.StatusConflict, "validation_error", "The email has not been sent")
		return
	}
	var body struct {
		Link string `json:"link"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "validation_error", "Request body must be valid JSON.")
		return
	}

	e.LastEvent = event
	h.store.Emails.Set(e.ID, e)
	data := emailEventData(e, "email."+event)
	if event == store.EmailStatusClicked {
		data["click"] = map[string]any{
			"link":      body.Link,
			"timestamp": h.store.Clock.Now().UTC().Format(eventTimeLayout),
		}
	}
	h.emit("email."+event, data)
	twincore.JSON(w, http.StatusOK, map[string]any{"id": e.ID, "last_event": e.LastEvent})
}
