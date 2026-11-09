package api

import (
	"net/http"
	"regexp"
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
	if msg := h.senderCheck(req); msg != "" {
		writeError(w, http.StatusForbidden, "validation_error", msg)
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
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "The `id` must be a valid UUID.")
		return
	}

	email, ok := h.store.Emails.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "Email not found")
		return
	}

	twincore.JSON(w, http.StatusOK, retrievedEmail(email))
}

// emailsPageSize is GET /emails' page size when the request names no limit;
// the newer Resend lists always paginate (quirk pagination-002).
const emailsPageSize = 20

// ListEmails handles GET /emails: sent emails newest first, 20 to a page
// unless limit says otherwise (quirk pagination-001).
func (h *Handler) ListEmails(w http.ResponseWriter, r *http.Request) {
	pq, ok := readPageQuery(w, r)
	if !ok {
		return
	}
	emails := h.store.Emails.List()
	items := make([]map[string]any, len(emails))
	for i, e := range emails {
		items[i] = listedEmail(e)
	}
	twincore.JSON(w, http.StatusOK, paginate(items, pq, emailsPageSize, func(m map[string]any) string { return m["id"].(string) }))
}

// listedEmail is an email as GET /emails lists it: the retrieved shape
// without object, html, text and tags (probes g01 of 2026-11-04 and l06 of
// 2026-11-08).
func listedEmail(e store.Email) map[string]any {
	out := retrievedEmail(e)
	for _, k := range []string{"object", "html", "text", "tags"} {
		delete(out, k)
	}
	return out
}

// pgTimestamp is the form Resend reports timestamps in on a read:
// '2026-11-05 03:21:53.923000+00'.
const pgTimestamp = "2006-01-02 15:04:05.000000+00"

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// retrievedEmail is the email as GET /emails/{id} answers it (quirks
// envelope-003, envelope-004): absent fields are null, bcc and cc come back
// null even when the send set them, tags appear only when the send had tags,
// and there is no status field. The stored values stay visible through
// /admin/emails.
func retrievedEmail(e store.Email) map[string]any {
	out := map[string]any{
		"object":       "email",
		"id":           e.ID,
		"to":           e.To,
		"from":         e.From,
		"created_at":   e.CreatedAt,
		"subject":      e.Subject,
		"bcc":          nil,
		"cc":           nil,
		"reply_to":     nilIfEmpty(e.ReplyTo),
		"last_event":   e.LastEvent,
		"scheduled_at": e.ScheduledAt,
		"message_id":   e.MessageID,
		"html":         nilIfBlank(e.HTML),
		"text":         nilIfBlank(e.Text),
	}
	if len(e.Tags) > 0 {
		out["tags"] = e.Tags
	}
	return out
}

func nilIfEmpty(v []string) any {
	if len(v) == 0 {
		return nil
	}
	return v
}

func nilIfBlank(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// maxBatch is Resend's cap on messages in one batch.
const maxBatch = 100

// SendBatch handles POST /emails/batch (quirk lifecycle-005). In strict mode,
// the default, one invalid item fails the whole batch with that item's error.
// With x-batch-validation: permissive, valid items are sent and the answer is
// 202 with the invalid items' errors by index.
func (h *Handler) SendBatch(w http.ResponseWriter, r *http.Request) {
	var reqs []sendEmailRequest
	if !decodeBody(w, r, &reqs) {
		return
	}
	if len(reqs) > maxBatch {
		writeError(w, http.StatusUnprocessableEntity, "validation_error",
			"Your batch request contains more than 100 messages.")
		return
	}
	permissive := strings.EqualFold(r.Header.Get("x-batch-validation"), "permissive")

	var errs []map[string]any
	valid := make([]sendEmailRequest, 0, len(reqs))
	for i, req := range reqs {
		status, msg := http.StatusUnprocessableEntity, validateSend(req)
		if msg == "" && len(req.Attachments) > 0 {
			msg = "Attachments are not supported in batch emails."
		}
		if msg == "" {
			status, msg = http.StatusForbidden, h.senderCheck(req)
		}
		if msg == "" {
			valid = append(valid, req)
			continue
		}
		if !permissive {
			writeError(w, status, "validation_error", msg)
			return
		}
		errs = append(errs, map[string]any{"index": i, "message": msg})
	}

	data := make([]map[string]any, 0, len(valid))
	for _, req := range valid {
		data = append(data, map[string]any{"id": h.createEmail(req).ID})
	}
	if permissive && len(errs) > 0 {
		twincore.JSON(w, http.StatusAccepted, map[string]any{"data": data, "errors": errs})
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]any{"data": data})
}

// createEmail is a shared helper that creates and stores an email from a request.
func (h *Handler) createEmail(req sendEmailRequest) store.Email {
	now := h.store.Clock.Now()
	id := h.store.NewEmailID()
	outcome := testAddressOutcome(req.To)

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
		Status:    outcome,
		CreatedAt: now.UTC().Format(pgTimestamp),
		MessageID: h.store.NewMessageID(),
		LastEvent: outcome,
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

// testAddressOutcome is the terminal event a send settles on at once (quirks
// lifecycle-001, lifecycle-002). Resend's test addresses delivered@, bounced@,
// complained@ and suppressed@resend.dev settle on that event; all but
// suppressed@ also accept a +label. The first test address among the
// recipients decides; any other send is delivered.
func testAddressOutcome(to []string) string {
	for _, addr := range to {
		local, domain, ok := strings.Cut(strings.ToLower(strings.TrimSpace(addr)), "@")
		if !ok || domain != "resend.dev" {
			continue
		}
		if local == "suppressed" {
			return store.EmailStatusSuppressed
		}
		base, _, _ := strings.Cut(local, "+")
		switch base {
		case "delivered":
			return store.EmailStatusDelivered
		case "bounced":
			return store.EmailStatusBounced
		case "complained":
			return store.EmailStatusComplained
		}
	}
	return store.EmailStatusDelivered
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
