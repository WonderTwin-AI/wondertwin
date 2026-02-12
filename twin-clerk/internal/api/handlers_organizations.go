package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-clerk/internal/store"
)

// createOrganizationRequest is the JSON body for POST /v1/organizations.
type createOrganizationRequest struct {
	Name                  string         `json:"name"`
	Slug                  string         `json:"slug,omitempty"`
	MaxAllowedMemberships int            `json:"max_allowed_memberships,omitempty"`
	CreatedBy             string         `json:"created_by,omitempty"`
	PublicMetadata        map[string]any `json:"public_metadata,omitempty"`
	PrivateMetadata       map[string]any `json:"private_metadata,omitempty"`
}

// updateOrganizationRequest is the JSON body for PATCH /v1/organizations/{id}.
type updateOrganizationRequest struct {
	Name                  *string        `json:"name,omitempty"`
	Slug                  *string        `json:"slug,omitempty"`
	MaxAllowedMemberships *int           `json:"max_allowed_memberships,omitempty"`
	PublicMetadata        map[string]any `json:"public_metadata,omitempty"`
	PrivateMetadata       map[string]any `json:"private_metadata,omitempty"`
}

// CreateOrganization handles POST /v1/organizations.
func (h *Handler) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	var req createOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		clerkError(w, http.StatusBadRequest, "form_param_invalid",
			"Invalid request body.", err.Error())
		return
	}

	if req.Name == "" {
		clerkError(w, http.StatusUnprocessableEntity, "form_param_missing",
			"name is required.", "You must provide a name for the organization.")
		return
	}

	id := h.store.Organizations.NextID()
	now := store.Now()

	slug := req.Slug
	if slug == "" {
		slug = strings.ToLower(strings.ReplaceAll(req.Name, " ", "-"))
	}

	maxMembers := req.MaxAllowedMemberships
	if maxMembers == 0 {
		maxMembers = 5
	}

	publicMeta := req.PublicMetadata
	if publicMeta == nil {
		publicMeta = make(map[string]any)
	}

	org := store.Organization{
		ID:                    id,
		Object:                "organization",
		Name:                  req.Name,
		Slug:                  slug,
		MembersCount:          0,
		MaxAllowedMemberships: maxMembers,
		PublicMetadata:        publicMeta,
		PrivateMetadata:       req.PrivateMetadata,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	// If created_by is specified, add the creator as an admin member
	if req.CreatedBy != "" {
		if _, ok := h.store.Users.Get(req.CreatedBy); ok {
			memID := h.store.OrgMembers.NextID()
			membership := store.OrgMembership{
				ID:           memID,
				Object:       "organization_membership",
				Role:         "admin",
				Organization: &org,
				CreatedAt:    now,
				UpdatedAt:    now,
			}
			h.store.OrgMembers.Set(memID, membership)
			org.MembersCount = 1
		}
	}

	h.store.Organizations.Set(id, org)
	twincore.JSON(w, http.StatusOK, org)
}

// GetOrganization handles GET /v1/organizations/{id}.
func (h *Handler) GetOrganization(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Look up by ID first, then by slug
	org, ok := h.store.Organizations.Get(id)
	if !ok {
		// Try finding by slug
		orgs := h.store.Organizations.Filter(func(oid string, o store.Organization) bool {
			return o.Slug == id
		})
		if len(orgs) > 0 {
			org = orgs[0]
			ok = true
		}
	}

	if !ok {
		clerkError(w, http.StatusNotFound, "resource_not_found",
			"Organization not found.",
			fmt.Sprintf("No organization was found with id or slug %s.", id))
		return
	}

	twincore.JSON(w, http.StatusOK, org)
}

// UpdateOrganization handles PATCH /v1/organizations/{id}.
func (h *Handler) UpdateOrganization(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	org, ok := h.store.Organizations.Get(id)
	if !ok {
		clerkError(w, http.StatusNotFound, "resource_not_found",
			"Organization not found.",
			fmt.Sprintf("No organization was found with id %s.", id))
		return
	}

	var req updateOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		clerkError(w, http.StatusBadRequest, "form_param_invalid",
			"Invalid request body.", err.Error())
		return
	}

	if req.Name != nil {
		org.Name = *req.Name
	}
	if req.Slug != nil {
		org.Slug = *req.Slug
	}
	if req.MaxAllowedMemberships != nil {
		org.MaxAllowedMemberships = *req.MaxAllowedMemberships
	}
	if req.PublicMetadata != nil {
		org.PublicMetadata = req.PublicMetadata
	}
	if req.PrivateMetadata != nil {
		org.PrivateMetadata = req.PrivateMetadata
	}

	org.UpdatedAt = store.Now()
	h.store.Organizations.Set(id, org)

	twincore.JSON(w, http.StatusOK, org)
}

// DeleteOrganization handles DELETE /v1/organizations/{id}.
func (h *Handler) DeleteOrganization(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if !h.store.Organizations.Delete(id) {
		clerkError(w, http.StatusNotFound, "resource_not_found",
			"Organization not found.",
			fmt.Sprintf("No organization was found with id %s.", id))
		return
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"id":      id,
		"object":  "organization",
		"deleted": true,
	})
}

// ListOrganizations handles GET /v1/organizations.
func (h *Handler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	limit := 10
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}

	allOrgs := h.store.Organizations.List()

	// Filter by query if provided
	query := r.URL.Query().Get("query")
	var filtered []store.Organization
	for _, org := range allOrgs {
		if query != "" {
			if !strings.Contains(strings.ToLower(org.Name), strings.ToLower(query)) &&
				!strings.Contains(strings.ToLower(org.Slug), strings.ToLower(query)) {
				continue
			}
		}
		filtered = append(filtered, org)
	}

	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := filtered[offset:end]

	if page == nil {
		page = []store.Organization{}
	}

	twincore.JSON(w, http.StatusOK, store.ClerkList[store.Organization]{
		Data:       page,
		TotalCount: total,
	})
}
