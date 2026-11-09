// Package api implements the Resend-compatible HTTP API handlers for the twin.
package api

import (
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// Handler holds all API handler state.
type Handler struct {
	store *store.MemoryStore
	mw    *twincore.Middleware

	// idemMu makes the idempotency check and reservation one step, so two
	// concurrent requests with one key cannot both send.
	idemMu sync.Mutex

	// strictSenders turns on Resend's full sender-domain rules; see
	// senderCheck. Off by default.
	strictSenders atomic.Bool

	// verifyMode is where domain verification lands: verified, pending or
	// failed; empty means verified.
	verifyMode atomic.Value

	// limiter is the opt-in rate limit; off by default.
	limiter rateLimiter
}

// NewHandler creates a new API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
	h := &Handler{store: s, mw: mw}
	h.limiter.nowFunc = s.Clock.Now
	return h
}

// Routes mounts the Resend API routes and admin extras.
func (h *Handler) Routes(r chi.Router) {
	r.Use(h.frontHeaders)
	r.NotFound(h.notFound)
	r.MethodNotAllowed(h.methodNotAllowed)

	// Resend API routes (Bearer token auth required)
	r.Route("/emails", func(r chi.Router) {
		r.Use(h.bearerAuthMiddleware)
		r.Use(h.mw.FaultInjection)

		r.With(h.idempotency).Post("/", h.SendEmail)
		r.Get("/", h.ListEmails)
		r.Get("/{id}", h.GetEmail)
		r.Patch("/{id}", h.UpdateEmail)
		r.Post("/{id}/cancel", h.CancelEmail)
		r.With(h.idempotency).Post("/batch", h.SendBatch)
	})

	// Domains
	r.Route("/domains", func(r chi.Router) {
		r.Use(h.bearerAuthMiddleware)
		r.Use(h.mw.FaultInjection)

		r.Get("/", h.ListDomains)
		r.Post("/", h.CreateDomain)
		r.Get("/{id}", h.GetDomain)
		r.Patch("/{id}", h.UpdateDomain)
		r.Delete("/{id}", h.DeleteDomain)
		r.Post("/{id}/verify", h.VerifyDomain)
	})

	// API Keys
	r.Route("/api-keys", func(r chi.Router) {
		r.Use(h.bearerAuthMiddleware)
		r.Use(h.mw.FaultInjection)

		r.Get("/", h.ListAPIKeys)
		r.Post("/", h.CreateAPIKey)
		r.Patch("/{id}", h.UpdateAPIKey)
		r.Delete("/{id}", h.DeleteAPIKey)
	})

	// Webhooks
	r.Route("/webhooks", func(r chi.Router) {
		r.Use(h.bearerAuthMiddleware)
		r.Use(h.mw.FaultInjection)

		r.Get("/", h.ListWebhooks)
		r.Post("/", h.CreateWebhook)
		r.Get("/{id}", h.GetWebhook)
		r.Patch("/{id}", h.UpdateWebhook)
		r.Delete("/{id}", h.DeleteWebhook)
	})

	// Audiences + Contacts
	r.Route("/audiences", func(r chi.Router) {
		r.Use(h.bearerAuthMiddleware)
		r.Use(h.mw.FaultInjection)

		r.Get("/", h.ListAudiences)
		r.Post("/", h.CreateAudience)
		r.Get("/{id}", h.GetAudience)
		r.Delete("/{id}", h.DeleteAudience)

		r.Get("/{audience_id}/contacts", h.ListContacts)
		r.Post("/{audience_id}/contacts", h.CreateContact)
		r.Get("/{audience_id}/contacts/{id}", h.GetContact)
		r.Patch("/{audience_id}/contacts/{id}", h.UpdateContact)
		r.Delete("/{audience_id}/contacts/{id}", h.DeleteContact)
	})

	// Broadcasts
	r.Route("/broadcasts", func(r chi.Router) {
		r.Use(h.bearerAuthMiddleware)
		r.Use(h.mw.FaultInjection)

		r.Get("/", h.ListBroadcasts)
		r.Post("/", h.CreateBroadcast)
		r.Get("/{id}", h.GetBroadcast)
		r.Post("/{id}/send", h.SendBroadcast)
		r.Delete("/{id}", h.DeleteBroadcast)
	})

	// Admin extras (no auth required)
	r.Get("/admin/emails", h.AdminListEmails)
}

// bearerAuthMiddleware validates Resend-style Bearer token auth.
func (h *Handler) bearerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.authenticate(w, r) {
			next.ServeHTTP(w, r)
		}
	})
}

// restrictedKeyMsg is Resend's answer when a sending_access key calls
// anything but a send (quirk auth-004).
const restrictedKeyMsg = "This API key is restricted to only send emails"

// authenticate checks the Authorization header and answers the error itself
// when it fails. Any non-empty Bearer token is accepted: the emulator does
// not know the account's keys (OD-3). A token the emulator issued through
// POST /api-keys is honoured: its use is recorded, and a sending_access key
// may only send email.
func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		writeError(w, http.StatusUnauthorized, "missing_api_key", "Missing API Key")
		return false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == auth || token == "" {
		writeError(w, http.StatusUnauthorized, "invalid_api_key",
			"Invalid authorization header format. Use 'Authorization: Bearer re_123'.")
		return false
	}
	if r.Header.Get("User-Agent") == "" {
		// Resend's edge blocks a keyed request with no User-Agent before it
		// reaches the API (quirk auth-006).
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("error code: 1010"))
		return false
	}
	if key, ok := h.issuedKey(token); ok {
		if key.Permission == store.PermissionSendingAccess && !isSendRequest(r) {
			writeError(w, http.StatusUnauthorized, "restricted_api_key", restrictedKeyMsg)
			return false
		}
		used := h.store.Clock.Now().UTC().Format(pgTimestamp)
		key.LastUsedAt = &used
		h.store.APIKeys.Set(key.ID, key)
	}
	return h.limiter.admit(w)
}

// isSendRequest reports whether r is POST /emails or POST /emails/batch, the
// only calls a sending_access key may make.
func isSendRequest(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch strings.TrimSuffix(r.URL.Path, "/") {
	case "/emails", "/emails/batch":
		return true
	}
	return false
}
