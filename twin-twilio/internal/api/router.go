// Package api implements the Twilio-compatible HTTP API handlers for the twin.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-twilio/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/messaging"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// Handler holds all API handler state.
type Handler struct {
	store     *store.MemoryStore
	mw        *twincore.Middleware
	msgEngine *messaging.Engine
}

// NewHandler creates a new API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware, me *messaging.Engine) *Handler {
	return &Handler{store: s, mw: mw, msgEngine: me}
}

// Routes mounts the Twilio API routes and admin extras.
func (h *Handler) Routes(r chi.Router) {
	// Twilio REST API routes (Basic Auth required)
	r.Route("/2010-04-01/Accounts/{AccountSid}", func(r chi.Router) {
		r.Use(h.basicAuthMiddleware)
		r.Use(h.mw.FaultInjection)

		// Messages
		r.Post("/Messages.json", h.CreateMessage)
		r.Get("/Messages/{MessageSid}.json", h.GetMessage)
		r.Post("/Messages/{MessageSid}.json", h.UpdateMessage)
		r.Delete("/Messages/{MessageSid}.json", h.DeleteMessage)
		r.Get("/Messages.json", h.ListMessages)
		r.Get("/Messages/{MessageSid}/Media.json", h.ListMessageMedia)
		r.Post("/Messages/{MessageSid}/Feedback.json", h.CreateMessageFeedback)
	})

	// Twilio Messaging Services API (Basic Auth required)
	r.Route("/v1/Services", func(r chi.Router) {
		r.Use(h.basicAuthMiddleware)
		r.Use(h.mw.FaultInjection)

		r.Get("/", h.ListMessagingServices)
		r.Post("/", h.CreateMessagingService)
	})

	// Twilio Verify Services API (Basic Auth required)
	r.Route("/v2/Services", func(r chi.Router) {
		r.Use(h.basicAuthMiddleware)
		r.Use(h.mw.FaultInjection)

		r.Get("/", h.ListVerifyServices)
		r.Post("/", h.CreateVerifyService)
		r.Get("/{ServiceSid}", h.GetVerifyService)
		r.Post("/{ServiceSid}", h.UpdateVerifyService)
		r.Delete("/{ServiceSid}", h.DeleteVerifyService)

		r.Post("/{ServiceSid}/Verifications", h.CreateVerification)
		r.Get("/{ServiceSid}/Verifications/{Sid}", h.GetVerification)
		r.Post("/{ServiceSid}/Verifications/{Sid}", h.UpdateVerification)
		r.Post("/{ServiceSid}/VerificationCheck", h.CheckVerification)
	})

	// Admin extras (no auth required, same as other twins)
	r.Get("/admin/messages", h.AdminListMessages)
	r.Get("/admin/otp", h.AdminGetOTP)
	r.Get("/admin/verifications", h.AdminListVerifications)
	r.Post("/admin/verifications/{sid}/expire", h.AdminExpireVerification)
}

// basicAuthMiddleware validates Twilio-style HTTP Basic Auth (AccountSID:AuthToken).
// In sim mode, we accept any non-empty credentials.
func (h *Handler) basicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user == "" || pass == "" {
			w.Header().Set("WWW-Authenticate", `Basic realm="Twilio API"`)
			twincore.JSON(w, http.StatusUnauthorized, map[string]any{
				"code":      20003,
				"message":   "Authenticate",
				"more_info": "https://www.twilio.com/docs/errors/20003",
				"status":    401,
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
