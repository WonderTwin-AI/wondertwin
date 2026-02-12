// Package api implements the Twilio-compatible HTTP API handlers for the twin.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-twilio/internal/store"
)

// Handler holds all API handler state.
type Handler struct {
	store *store.MemoryStore
	mw    *twincore.Middleware
}

// NewHandler creates a new API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
	return &Handler{store: s, mw: mw}
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
		r.Get("/Messages.json", h.ListMessages)
	})

	// Twilio Verify API (Basic Auth required)
	r.Route("/v2/Services/{ServiceSid}", func(r chi.Router) {
		r.Use(h.basicAuthMiddleware)
		r.Use(h.mw.FaultInjection)

		r.Post("/Verifications", h.CreateVerification)
		r.Get("/Verifications/{Sid}", h.GetVerification)
		r.Post("/VerificationCheck", h.CheckVerification)
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
				"code":     20003,
				"message":  "Authenticate",
				"more_info": "https://www.twilio.com/docs/errors/20003",
				"status":   401,
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
