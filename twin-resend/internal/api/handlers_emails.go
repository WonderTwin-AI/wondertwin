package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
)

// sendEmailRequest matches the Resend send email API request body.
type sendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html,omitempty"`
	Text    string   `json:"text,omitempty"`
	CC      []string `json:"cc,omitempty"`
	BCC     []string `json:"bcc,omitempty"`
	ReplyTo []string `json:"reply_to,omitempty"`
}

// SendEmail handles POST /emails
func (h *Handler) SendEmail(w http.ResponseWriter, r *http.Request) {
	var req sendEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"statusCode": 422,
			"message":    "Invalid request body: " + err.Error(),
			"name":       "validation_error",
		})
		return
	}

	if req.From == "" {
		twincore.JSON(w, http.StatusUnprocessableEntity, map[string]any{
			"statusCode": 422,
			"message":    "The 'from' field is required.",
			"name":       "validation_error",
		})
		return
	}

	if len(req.To) == 0 {
		twincore.JSON(w, http.StatusUnprocessableEntity, map[string]any{
			"statusCode": 422,
			"message":    "The 'to' field is required and must contain at least one email address.",
			"name":       "validation_error",
		})
		return
	}

	if req.Subject == "" {
		twincore.JSON(w, http.StatusUnprocessableEntity, map[string]any{
			"statusCode": 422,
			"message":    "The 'subject' field is required.",
			"name":       "validation_error",
		})
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
	if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"statusCode": 422,
			"message":    "Invalid request body: " + err.Error(),
			"name":       "validation_error",
		})
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
	id := h.store.Emails.NextID()

	email := store.Email{
		ID:        id,
		Object:    "email",
		From:      req.From,
		To:        req.To,
		Subject:   req.Subject,
		HTML:      req.HTML,
		Text:      req.Text,
		CC:        req.CC,
		BCC:       req.BCC,
		ReplyTo:   req.ReplyTo,
		Status:    store.EmailStatusDelivered, // Sim: instantly delivered
		CreatedAt: now.Format(time.RFC3339),
		LastEvent: "delivered",
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
