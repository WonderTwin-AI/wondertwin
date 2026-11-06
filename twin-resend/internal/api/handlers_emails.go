package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// sendEmailRequest matches the Resend send email API request body.
type sendEmailRequest struct {
	From        string            `json:"from"`
	To          recipients        `json:"to"`
	Subject     string            `json:"subject"`
	HTML        string            `json:"html,omitempty"`
	Text        *string           `json:"text,omitempty"`
	CC          recipients        `json:"cc,omitempty"`
	BCC         recipients        `json:"bcc,omitempty"`
	ReplyTo     recipients        `json:"reply_to,omitempty"`
	Tags        []store.EmailTag  `json:"tags,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Attachments []struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
	} `json:"attachments,omitempty"`
	ScheduledAt string `json:"scheduled_at,omitempty"`
}

// maxRecipients is Resend's cap on the to field.
const maxRecipients = 50

// validateSend checks a send request the way Resend does and returns the
// error message, or "" when the request is valid.
func validateSend(req sendEmailRequest) string {
	switch {
	case req.From == "":
		return "Missing `from` field."
	case len(req.To) == 0:
		return "Missing `to` field."
	case req.Subject == "":
		return "Missing `subject` field."
	case len(req.To) > maxRecipients:
		return "Invalid `to` field. The `to` field must contain at most 50 email addresses."
	}
	if req.ScheduledAt != "" {
		if _, err := time.Parse(time.RFC3339, req.ScheduledAt); err != nil {
			return "Invalid `scheduled_at` field. The date must be in ISO 8601 format."
		}
	}
	return ""
}

// SendEmail handles POST /emails
func (h *Handler) SendEmail(w http.ResponseWriter, r *http.Request) {
	var req sendEmailRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if msg := validateSend(req); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", msg)
		return
	}

	email := h.createEmail(req)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"id": email.ID,
	})
}

// GetEmail handles GET /emails/{id}
func (h *Handler) GetEmail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	email, ok := h.store.Emails.Get(id)
	if !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404,
			"message":    "Email not found",
			"name":       "not_found",
		})
		return
	}

	twincore.JSON(w, http.StatusOK, email)
}

// SendBatch handles POST /emails/batch
func (h *Handler) SendBatch(w http.ResponseWriter, r *http.Request) {
	var reqs []sendEmailRequest
	if !decodeBody(w, r, &reqs) {
		return
	}

	var results []map[string]any
	for _, req := range reqs {
		email := h.createEmail(req)
		results = append(results, map[string]any{
			"id": email.ID,
		})
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"data": results,
	})
}

// createEmail is a shared helper that creates and stores an email from a request.
func (h *Handler) createEmail(req sendEmailRequest) store.Email {
	now := h.store.Clock.Now()
	id := h.store.NewEmailID()

	email := store.Email{
		ID:        id,
		Object:    "email",
		From:      req.From,
		To:        req.To,
		Subject:   req.Subject,
		HTML:      req.HTML,
		Text:      textFor(req),
		CC:        req.CC,
		BCC:       req.BCC,
		ReplyTo:   req.ReplyTo,
		Status:    store.EmailStatusDelivered, // Sim: instantly delivered
		CreatedAt: now.Format(time.RFC3339),
		LastEvent: "delivered",
		Tags:      req.Tags,
		Headers:   req.Headers,
	}
	for _, a := range req.Attachments {
		email.Attachments = append(email.Attachments, store.AttachmentMeta{Filename: a.Filename, ContentType: a.ContentType})
	}
	if req.ScheduledAt != "" {
		at := req.ScheduledAt
		email.ScheduledAt = &at
		email.Status = store.EmailStatusScheduled
		email.LastEvent = store.EmailStatusScheduled
	}

	h.store.Emails.Set(id, email)

	return email
}

// AdminListEmails handles GET /admin/emails
// Supports ?to={email} and ?subject={q} query parameters.
func (h *Handler) AdminListEmails(w http.ResponseWriter, r *http.Request) {
	toFilter := r.URL.Query().Get("to")
	subjectFilter := r.URL.Query().Get("subject")

	emails := h.store.Emails.List()

	if toFilter != "" || subjectFilter != "" {
		var filtered []store.Email
		for _, email := range emails {
			if toFilter != "" {
				found := false
				for _, addr := range email.To {
					if addr == toFilter {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			if subjectFilter != "" {
				if !strings.Contains(strings.ToLower(email.Subject), strings.ToLower(subjectFilter)) {
					continue
				}
			}
			filtered = append(filtered, email)
		}
		emails = filtered
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"emails": emails,
		"total":  len(emails),
	})
}
