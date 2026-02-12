// Package api implements the Clerk-compatible HTTP API handlers for the twin.
package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-clerk/internal/store"
)

// Handler holds all API handler state.
type Handler struct {
	store    *store.MemoryStore
	mw       *twincore.Middleware
	jwtMgr   *JWTManager
}

// NewHandler creates a new API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware, jwtMgr *JWTManager) *Handler {
	return &Handler{store: s, mw: mw, jwtMgr: jwtMgr}
}

// Routes mounts the Clerk API routes.
func (h *Handler) Routes(r chi.Router) {
	// Public endpoints (no auth required)
	r.Get("/.well-known/jwks.json", h.GetJWKS)

	// Clerk Backend API (requires Bearer auth with sk_test_* key)
	r.Route("/v1", func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Use(h.mw.FaultInjection)

		// Users
		r.Post("/users", h.CreateUser)
		r.Get("/users", h.ListUsers)
		r.Get("/users/{id}", h.GetUser)
		r.Patch("/users/{id}", h.UpdateUser)
		r.Delete("/users/{id}", h.DeleteUser)

		// Sessions
		r.Get("/sessions", h.ListSessions)
		r.Get("/sessions/{id}", h.GetSession)
		r.Post("/sessions/{id}/verify", h.VerifySession)
		r.Post("/sessions/{id}/revoke", h.RevokeSession)

		// Organizations
		r.Post("/organizations", h.CreateOrganization)
		r.Get("/organizations", h.ListOrganizations)
		r.Get("/organizations/{id}", h.GetOrganization)
		r.Patch("/organizations/{id}", h.UpdateOrganization)
		r.Delete("/organizations/{id}", h.DeleteOrganization)
	})

	// Admin-only JWT generation (not part of real Clerk API, used by tests)
	r.Post("/admin/jwt/generate", h.GenerateJWT)
	// Admin session creation (for seeding sessions tied to users)
	r.Post("/admin/sessions", h.AdminCreateSession)
}

// authMiddleware validates Clerk-style Bearer token authentication.
// Clerk uses "Authorization: Bearer sk_test_..." or "Authorization: Bearer sk_live_..."
func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			clerkError(w, http.StatusUnauthorized, "authentication_invalid",
				"No authorization header provided.",
				"You must provide a valid Bearer token in the Authorization header.")
			return
		}

		key := strings.TrimPrefix(auth, "Bearer ")
		if key == auth || key == "" {
			clerkError(w, http.StatusUnauthorized, "authentication_invalid",
				"Invalid authorization header format.",
				"The Authorization header must use the Bearer scheme.")
			return
		}

		// Accept any sk_test_*, sk_live_*, or sk_sim_* key
		if !strings.HasPrefix(key, "sk_test_") &&
			!strings.HasPrefix(key, "sk_live_") &&
			!strings.HasPrefix(key, "sk_sim_") {
			clerkError(w, http.StatusUnauthorized, "authentication_invalid",
				"Invalid API key.",
				"The provided API key is not valid. Use a key starting with sk_test_ or sk_live_.")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// clerkError writes an error response in Clerk's error format.
func clerkError(w http.ResponseWriter, status int, code, message, longMessage string) {
	twincore.JSON(w, status, store.ClerkError{
		Errors: []store.ClerkErrorEntry{
			{
				Code:        code,
				Message:     message,
				LongMessage: longMessage,
			},
		},
	})
}
