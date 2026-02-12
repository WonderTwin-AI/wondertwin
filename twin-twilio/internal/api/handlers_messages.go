package api

import (
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-twilio/internal/store"
)

// otpRegex matches 4-6 digit OTP codes in message bodies.
var otpRegex = regexp.MustCompile(`\b(\d{4,6})\b`)

// CreateMessage handles POST /2010-04-01/Accounts/{AccountSid}/Messages.json
// Twilio sends form-encoded requests and returns JSON.
func (h *Handler) CreateMessage(w http.ResponseWriter, r *http.Request) {
	accountSID := chi.URLParam(r, "AccountSid")

	if err := r.ParseForm(); err != nil {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"code":    21601,
			"message": "Unable to parse form data: " + err.Error(),
			"status":  400,
		})
		return
	}

	to := r.FormValue("To")
	from := r.FormValue("From")
	body := r.FormValue("Body")

	if to == "" {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"code":    21604,
			"message": "A 'To' phone number is required.",
			"status":  400,
		})
		return
	}

	if from == "" && r.FormValue("MessagingServiceSid") == "" {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"code":    21603,
			"message": "A 'From' phone number is required.",
			"status":  400,
		})
		return
	}

	if body == "" {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"code":    21602,
			"message": "Message body is required.",
			"status":  400,
		})
		return
	}

	now := h.store.Clock.Now()
	sid := h.store.Messages.NextID()

	msg := store.Message{
		SID:         sid,
		AccountSID:  accountSID,
		To:          to,
		From:        from,
		Body:        body,
		Status:      store.MessageStatusDelivered, // Sim: instantly delivered
		Direction:   "outbound-api",
		NumSegments: "1",
		NumMedia:    "0",
		PriceUnit:   "USD",
		ErrorCode:   nil,
		DateCreated: now.Format(time.RFC1123Z),
		DateUpdated: now.Format(time.RFC1123Z),
		DateSent:    now.Format(time.RFC1123Z),
		URI:         fmt.Sprintf("/2010-04-01/Accounts/%s/Messages/%s.json", accountSID, sid),
	}

	h.store.Messages.Set(sid, msg)

	twincore.JSON(w, http.StatusCreated, msg)
}

// GetMessage handles GET /2010-04-01/Accounts/{AccountSid}/Messages/{MessageSid}.json
func (h *Handler) GetMessage(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "MessageSid")

	msg, ok := h.store.Messages.Get(sid)
	if !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"code":    20404,
			"message": fmt.Sprintf("The requested resource /Messages/%s was not found", sid),
			"status":  404,
		})
		return
	}

	twincore.JSON(w, http.StatusOK, msg)
}

// ListMessages handles GET /2010-04-01/Accounts/{AccountSid}/Messages.json
func (h *Handler) ListMessages(w http.ResponseWriter, r *http.Request) {
	to := r.URL.Query().Get("To")
	from := r.URL.Query().Get("From")

	messages := h.store.Messages.List()

	// Filter if query params provided
	if to != "" || from != "" {
		var filtered []store.Message
		for _, msg := range messages {
			if to != "" && msg.To != to {
				continue
			}
			if from != "" && msg.From != from {
				continue
			}
			filtered = append(filtered, msg)
		}
		messages = filtered
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"messages":         messages,
		"end":              len(messages) - 1,
		"first_page_uri":  "/2010-04-01/Accounts/Messages.json?PageSize=50&Page=0",
		"next_page_uri":   nil,
		"page":            0,
		"page_size":       50,
		"previous_page_uri": nil,
		"start":           0,
		"uri":             "/2010-04-01/Accounts/Messages.json?PageSize=50&Page=0",
	})
}

// AdminListMessages handles GET /admin/messages
// Supports ?to={phone} query parameter for filtering.
func (h *Handler) AdminListMessages(w http.ResponseWriter, r *http.Request) {
	to := r.URL.Query().Get("to")

	messages := h.store.Messages.List()

	if to != "" {
		var filtered []store.Message
		for _, msg := range messages {
			if msg.To == to {
				filtered = append(filtered, msg)
			}
		}
		messages = filtered
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"messages": messages,
		"total":    len(messages),
	})
}

// AdminGetOTP handles GET /admin/otp?to={phone}
// Returns the last OTP code sent to that phone number.
func (h *Handler) AdminGetOTP(w http.ResponseWriter, r *http.Request) {
	to := r.URL.Query().Get("to")
	if to == "" {
		twincore.Error(w, http.StatusBadRequest, "'to' query parameter is required")
		return
	}

	// Search messages in reverse order (most recent first)
	messages := h.store.Messages.List()
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.To != to {
			continue
		}

		// Extract OTP code from body
		matches := otpRegex.FindAllString(msg.Body, -1)
		if len(matches) > 0 {
			// Return the last match in the most recent matching message
			code := matches[len(matches)-1]
			twincore.JSON(w, http.StatusOK, map[string]any{
				"to":          to,
				"code":        code,
				"message_sid": msg.SID,
				"body":        msg.Body,
				"found":       true,
			})
			return
		}
	}

	// No OTP found
	twincore.JSON(w, http.StatusOK, map[string]any{
		"to":    to,
		"found": false,
	})
}

// extractOTP finds OTP codes (4-6 digit numbers) in a message body.
func extractOTP(body string) []string {
	return otpRegex.FindAllString(body, -1)
}
