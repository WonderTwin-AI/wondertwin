// Package api implements the Logo.dev-compatible HTTP API handlers for the twin.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/quirks"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-logodev/internal/store"
)

// Handler holds logo API state.
type Handler struct {
	store   *store.MemoryStore
	mw      *twincore.Middleware
	emitter *telemetry.Emitter
	quirks  *quirks.Engine
}

// NewHandler creates a new logo API handler.
func NewHandler(s *store.MemoryStore, mw *twincore.Middleware, em *telemetry.Emitter, qe *quirks.Engine) *Handler {
	return &Handler{store: s, mw: mw, emitter: em, quirks: qe}
}

// Routes mounts the Logo.dev-compatible routes.
func (h *Handler) Routes(r chi.Router) {
	// Logo API routes (token auth required)
	r.Group(func(r chi.Router) {
		r.Use(h.tokenAuthMiddleware)
		r.Use(h.mw.FaultInjection)
		r.Use(quirks.Middleware(h.quirks))
		r.Use(telemetry.Middleware(h.emitter))

		r.Get("/api/v1/search", h.SearchBrands)
		r.Get("/{domain}", h.GetLogo)
	})

	// Admin extras (no auth required)
	r.Get("/admin/logos", h.AdminListLogos)
	r.Get("/admin/logos/{domain}", h.AdminGetLogo)
}

// tokenAuthMiddleware validates Logo.dev-style API token auth.
// Logo.dev uses ?token= query param or Authorization: Bearer header.
// In sim mode, accepts any non-empty token.
func (h *Handler) tokenAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			// Fall back to Authorization header
			auth := r.Header.Get("Authorization")
			if len(auth) > 7 && auth[:7] == "Bearer " {
				token = auth[7:]
			}
		}

		if token == "" {
			twincore.JSON(w, http.StatusUnauthorized, map[string]any{
				"error":   "unauthorized",
				"message": "API token required. Pass ?token=pk_xxx or Authorization: Bearer pk_xxx",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}
