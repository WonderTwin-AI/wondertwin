package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// scheduledEmail looks up the email a reschedule or cancel names, settled
// against the store clock, and answers Resend's errors for a malformed or
// unknown id itself (as GET /emails/{id} does). It reports false once it has
// answered.
func (h *Handler) scheduledEmail(w http.ResponseWriter, r *http.Request) (store.Email, bool) {
	id := chi.URLParam(r, "id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "The `id` must be a valid UUID.")
		return store.Email{}, false
	}
	e, ok := h.store.Emails.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "Email not found")
		return store.Email{}, false
	}
	return h.settledEmail(e), true
}

// UpdateEmail handles PATCH /emails/{id}: it moves a scheduled email to a
// new scheduled_at, in ISO 8601 or natural language, and answers
// {object: "email", id} (quirks envelope-009, lifecycle-003; probes l03, l05,
// l10 and l16 of 2026-11-08).
func (h *Handler) UpdateEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ScheduledAt string `json:"scheduled_at"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	e, ok := h.scheduledEmail(w, r)
	if !ok {
		return
	}
	if e.LastEvent != store.EmailStatusScheduled {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "You can not edit an email that is not scheduled")
		return
	}
	if req.ScheduledAt == "" {
		missingField(w, "scheduled_at")
		return
	}
	at, msg := parseScheduledAt(req.ScheduledAt, h.store.Clock.Now())
	if msg != "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", msg)
		return
	}
	formatted := at.UTC().Format(pgTimestamp)
	e.ScheduledAt = &formatted
	h.store.Emails.Set(e.ID, e)
	twincore.JSON(w, http.StatusOK, map[string]any{"object": "email", "id": e.ID})
}

// CancelEmail handles POST /emails/{id}/cancel: a scheduled email becomes
// canceled and is never sent; anything else answers 422 'Email is not
// scheduled' (quirk envelope-009; probes l07 to l09 of 2026-11-08, h08 of
// 2026-11-04).
func (h *Handler) CancelEmail(w http.ResponseWriter, r *http.Request) {
	e, ok := h.scheduledEmail(w, r)
	if !ok {
		return
	}
	if e.LastEvent != store.EmailStatusScheduled {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "Email is not scheduled")
		return
	}
	e.Status, e.LastEvent = store.EmailStatusCanceled, store.EmailStatusCanceled
	h.store.Emails.Set(e.ID, e)
	twincore.JSON(w, http.StatusOK, map[string]any{"object": "email", "id": e.ID})
}
