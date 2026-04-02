package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
)

// GetOrg handles GET /orgs/{org}
func (h *Handler) GetOrg(w http.ResponseWriter, r *http.Request) {
	orgLogin := chi.URLParam(r, "org")
	org, ok := h.store.Orgs.Get(orgLogin)
	if !ok {
		// Generate a default org
		org = store.Organization{
			ID:      h.store.NextID(),
			Login:   orgLogin,
			Type:    "Organization",
			HTMLURL: h.store.BaseURL() + "/" + orgLogin,
		}
	}
	ghJSON(w, 200, org)
}

// ListOrgMembers handles GET /orgs/{org}/members
func (h *Handler) ListOrgMembers(w http.ResponseWriter, r *http.Request) {
	// Return seeded users
	ghJSON(w, 200, []any{})
}

// ListOrgRepos handles GET /orgs/{org}/repos
func (h *Handler) ListOrgRepos(w http.ResponseWriter, r *http.Request) {
	orgLogin := chi.URLParam(r, "org")
	repos := h.store.Repos.Filter(func(_ string, rp store.Repository) bool {
		return rp.Owner.Login == orgLogin
	})
	ghJSON(w, 200, repos)
}

// ListOrgTeams handles GET /orgs/{org}/teams
func (h *Handler) ListOrgTeams(w http.ResponseWriter, r *http.Request) {
	orgLogin := chi.URLParam(r, "org")
	teams := h.store.ListOrgTeams(orgLogin)
	ghJSON(w, 200, teams)
}

// CreateTeam handles POST /orgs/{org}/teams
func (h *Handler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	orgLogin := chi.URLParam(r, "org")

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Permission  string `json:"permission"`
		Privacy     string `json:"privacy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ghError(w, 400, "Problems parsing JSON")
		return
	}
	if req.Permission == "" {
		req.Permission = "pull"
	}
	if req.Privacy == "" {
		req.Privacy = "secret"
	}

	team := store.Team{
		ID:          h.store.NextID(),
		Name:        req.Name,
		Slug:        strings.ToLower(strings.ReplaceAll(req.Name, " ", "-")),
		Description: req.Description,
		Permission:  req.Permission,
		Privacy:     req.Privacy,
		HTMLURL:     fmt.Sprintf("%s/orgs/%s/teams/%s", h.store.BaseURL(), orgLogin, strings.ToLower(req.Name)),
		OrgLogin:    orgLogin,
	}

	id := h.store.Teams.NextID()
	h.store.Teams.Set(id, team)
	ghJSON(w, 201, team)
}

// GetTeam handles GET /orgs/{org}/teams/{team_slug}
func (h *Handler) GetTeam(w http.ResponseWriter, r *http.Request) {
	orgLogin := chi.URLParam(r, "org")
	slug := chi.URLParam(r, "team_slug")

	teams := h.store.Teams.Filter(func(_ string, t store.Team) bool {
		return t.OrgLogin == orgLogin && t.Slug == slug
	})
	if len(teams) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	ghJSON(w, 200, teams[0])
}

// UpdateTeam handles PATCH /orgs/{org}/teams/{team_slug}
func (h *Handler) UpdateTeam(w http.ResponseWriter, r *http.Request) {
	orgLogin := chi.URLParam(r, "org")
	slug := chi.URLParam(r, "team_slug")

	ids, teams := h.store.Teams.FilterWithIDs(func(_ string, t store.Team) bool {
		return t.OrgLogin == orgLogin && t.Slug == slug
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}

	var req map[string]any
	json.NewDecoder(r.Body).Decode(&req)

	team := teams[0]
	if name, ok := req["name"].(string); ok {
		team.Name = name
		team.Slug = strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	}
	if desc, ok := req["description"].(string); ok {
		team.Description = desc
	}

	h.store.Teams.Set(ids[0], team)
	ghJSON(w, 200, team)
}

// DeleteTeam handles DELETE /orgs/{org}/teams/{team_slug}
func (h *Handler) DeleteTeam(w http.ResponseWriter, r *http.Request) {
	orgLogin := chi.URLParam(r, "org")
	slug := chi.URLParam(r, "team_slug")

	ids, _ := h.store.Teams.FilterWithIDs(func(_ string, t store.Team) bool {
		return t.OrgLogin == orgLogin && t.Slug == slug
	})
	if len(ids) == 0 {
		ghError(w, 404, "Not Found")
		return
	}
	h.store.Teams.Delete(ids[0])
	w.WriteHeader(204)
}
