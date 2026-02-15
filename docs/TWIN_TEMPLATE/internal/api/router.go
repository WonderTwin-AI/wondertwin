// Package api implements the TEMPLATE-compatible HTTP API handlers for the twin.
// Replace TEMPLATE with your service name throughout this package.
package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-TEMPLATE/internal/store"
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

// Routes mounts the service API routes.
// Adjust the route structure to match the real service's URL patterns.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/v1/resources", func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Use(h.mw.FaultInjection)

		r.Post("/", h.CreateResource)
		r.Get("/", h.ListResources)
		r.Get("/{id}", h.GetResource)
		r.Patch("/{id}", h.UpdateResource)
		r.Delete("/{id}", h.DeleteResource)
	})
}

// authMiddleware validates authentication.
// Adapt this to match the real service's auth pattern (Bearer token, API key header, etc.).
func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			// Return the same error format the real service uses.
			twincore.JSON(w, http.StatusUnauthorized, map[string]any{
				"error": map[string]any{
					"message": "Missing API key.",
					"type":    "authentication_error",
				},
			})
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth || token == "" {
			twincore.JSON(w, http.StatusUnauthorized, map[string]any{
				"error": map[string]any{
					"message": "Invalid API key format.",
					"type":    "authentication_error",
				},
			})
			return
		}

		// In simulation mode, accept any non-empty token.
		next.ServeHTTP(w, r)
	})
}
