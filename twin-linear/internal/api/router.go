package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	gql "github.com/wondertwin-ai/wondertwin/twinkit/graphql"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-linear/internal/store"
)

// Handler holds Linear API state.
type Handler struct {
	store  *store.MemoryStore
	mw     *twincore.Middleware
	schema *gql.Schema
}

func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
	schema := gql.NewSchema()
	RegisterResolvers(schema, s)

	return &Handler{store: s, mw: mw, schema: schema}
}

// Routes mounts the Linear GraphQL API endpoint and admin routes.
func (h *Handler) Routes(r chi.Router) {
	// GraphQL endpoint (auth required)
	r.Group(func(r chi.Router) {
		r.Use(h.linearAuthMiddleware)
		r.Use(h.mw.FaultInjection)
		r.Use(h.rateLimitHeaders)

		r.Post("/graphql", gql.Handler(h.schema))
	})

	// Admin extras (no auth required)
	r.Get("/admin/issues", h.AdminListIssues)
	r.Get("/admin/teams", h.AdminListTeams)
	r.Get("/admin/projects", h.AdminListProjects)
}

// linearAuthMiddleware validates Linear-style auth.
// Linear uses "Authorization: <API_KEY>" (no Bearer prefix) or "Authorization: Bearer <token>" for OAuth.
func (h *Handler) linearAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{
				"errors": []map[string]any{
					{
						"message": "Authentication required",
						"extensions": map[string]any{
							"code": "AUTHENTICATION_ERROR",
							"type": "authentication_error",
						},
					},
				},
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) rateLimitHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.Header().Set("X-Complexity-Limit", "250000")
		w.Header().Set("X-Complexity-Remaining", "249946")
		w.Header().Set("X-Complexity-Cost", "54")
		next.ServeHTTP(w, r)
	})
}

// AdminListIssues returns all issues for admin inspection.
func (h *Handler) AdminListIssues(w http.ResponseWriter, r *http.Request) {
	issues := h.store.Issues.List()
	twincore.JSON(w, 200, map[string]any{"issues": issues, "total": len(issues)})
}

func (h *Handler) AdminListTeams(w http.ResponseWriter, r *http.Request) {
	teams := h.store.Teams.List()
	twincore.JSON(w, 200, map[string]any{"teams": teams, "total": len(teams)})
}

func (h *Handler) AdminListProjects(w http.ResponseWriter, r *http.Request) {
	projects := h.store.Projects.List()
	twincore.JSON(w, 200, map[string]any{"projects": projects, "total": len(projects)})
}
