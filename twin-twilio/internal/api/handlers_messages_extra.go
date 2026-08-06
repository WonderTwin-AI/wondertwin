package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-twilio/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// UpdateMessage handles POST /2010-04-01/Accounts/{AccountSid}/Messages/{MessageSid}.json
// Twilio uses POST for updates (not PUT/PATCH). Primary use: redact message body.
func (h *Handler) UpdateMessage(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "MessageSid")
	msg, ok := h.store.Messages.Get(sid)
	if !ok {
		twilioError(w, http.StatusNotFound, 20404, fmt.Sprintf("The requested resource /2010-04-01/Accounts/AC_sim_test/Messages/%s.json was not found", sid))
		return
	}

	if err := r.ParseForm(); err == nil {
		if body := r.FormValue("Body"); body != "" {
			msg.Body = body
		}
		if status := r.FormValue("Status"); status == "canceled" {
			if msg.Status == store.MessageStatusQueued {
				msg.Status = "canceled"
			}
		}
	}

	msg.DateUpdated = h.store.Clock.Now().Format("Mon, 02 Jan 2006 15:04:05 +0000")
	h.store.Messages.Set(sid, msg)
	twincore.JSON(w, http.StatusOK, msg)
}

// DeleteMessage handles DELETE /2010-04-01/Accounts/{AccountSid}/Messages/{MessageSid}.json
func (h *Handler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "MessageSid")
	if _, ok := h.store.Messages.Get(sid); !ok {
		twilioError(w, http.StatusNotFound, 20404, "Message not found")
		return
	}
	h.store.Messages.Delete(sid)
	w.WriteHeader(http.StatusNoContent)
}

// ListMessageMedia handles GET /2010-04-01/Accounts/{AccountSid}/Messages/{MessageSid}/Media.json
// Returns empty media list — MMS media simulation is not implemented.
func (h *Handler) ListMessageMedia(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "MessageSid")
	if _, ok := h.store.Messages.Get(sid); !ok {
		twilioError(w, http.StatusNotFound, 20404, "Message not found")
		return
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"media_list":     []any{},
		"end":            0,
		"first_page_uri": fmt.Sprintf("/2010-04-01/Accounts/AC_sim_test/Messages/%s/Media.json?PageSize=50&Page=0", sid),
		"page":           0,
		"page_size":      50,
		"start":          0,
		"uri":            fmt.Sprintf("/2010-04-01/Accounts/AC_sim_test/Messages/%s/Media.json?PageSize=50&Page=0", sid),
	})
}

// CreateMessageFeedback handles POST /2010-04-01/Accounts/{AccountSid}/Messages/{MessageSid}/Feedback.json
func (h *Handler) CreateMessageFeedback(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "MessageSid")
	msg, ok := h.store.Messages.Get(sid)
	if !ok {
		twilioError(w, http.StatusNotFound, 20404, "Message not found")
		return
	}

	outcome := "confirmed"
	if err := r.ParseForm(); err == nil {
		if o := r.FormValue("Outcome"); o != "" {
			outcome = o
		}
	}

	twincore.JSON(w, http.StatusCreated, map[string]any{
		"account_sid":  "AC_sim_test",
		"message_sid":  msg.SID,
		"outcome":      outcome,
		"date_created": h.store.Clock.Now().Format("Mon, 02 Jan 2006 15:04:05 +0000"),
		"date_updated": h.store.Clock.Now().Format("Mon, 02 Jan 2006 15:04:05 +0000"),
		"uri":          fmt.Sprintf("/2010-04-01/Accounts/AC_sim_test/Messages/%s/Feedback.json", sid),
	})
}

// --- Messaging Services ---

// ListMessagingServices handles GET /v1/Services
func (h *Handler) ListMessagingServices(w http.ResponseWriter, r *http.Request) {
	// Messaging Services are a separate resource — for now return empty list
	// to support SDK initialization flows
	twincore.JSON(w, http.StatusOK, map[string]any{
		"services": []any{},
		"meta": map[string]any{
			"page":      0,
			"page_size": 50,
			"url":       "/v1/Services?PageSize=50&Page=0",
		},
	})
}

// CreateMessagingService handles POST /v1/Services
func (h *Handler) CreateMessagingService(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		twilioError(w, http.StatusBadRequest, 60200, "Unable to parse form data")
		return
	}

	friendlyName := r.FormValue("FriendlyName")
	if friendlyName == "" {
		twilioError(w, http.StatusBadRequest, 60200, "FriendlyName is required")
		return
	}

	now := h.store.Clock.Now()
	sid := fmt.Sprintf("MG%s", h.generateCode(32))

	twincore.JSON(w, http.StatusCreated, map[string]any{
		"sid":           sid,
		"account_sid":   "AC_sim_test",
		"friendly_name": friendlyName,
		"date_created":  now.Format("Mon, 02 Jan 2006 15:04:05 +0000"),
		"date_updated":  now.Format("Mon, 02 Jan 2006 15:04:05 +0000"),
		"url":           fmt.Sprintf("/v1/Services/%s", sid),
	})
}
