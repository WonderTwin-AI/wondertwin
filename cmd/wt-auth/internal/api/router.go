package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-auth/internal/store"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-auth/internal/token"
)

// Handler holds API handler state.
type Handler struct {
	licenses    *store.LicenseStore
	tokens      *store.TokenStore
	signer      *token.Signer
	internalKey string
}

// NewHandler creates a new API handler.
func NewHandler(licenses *store.LicenseStore, tokens *store.TokenStore, signer *token.Signer, internalKey string) *Handler {
	return &Handler{
		licenses:    licenses,
		tokens:      tokens,
		signer:      signer,
		internalKey: internalKey,
	}
}

// Routes mounts all API routes.
func (h *Handler) Routes(r chi.Router) {
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health (public).
	r.Get("/health", h.handleHealth)

	// Public token endpoints.
	r.Post("/v1/tokens", h.handleIssueToken)
	r.Post("/v1/tokens/verify", h.handleVerifyToken)

	// Internal endpoints (admin only).
	r.Route("/internal", func(r chi.Router) {
		r.Use(h.internalAuth)
		r.Post("/licenses", h.handleCreateLicense)
		r.Delete("/licenses/{id}", h.handleRevokeLicense)
		r.Get("/orgs/{orgID}/licenses", h.handleListLicenses)
	})
}
