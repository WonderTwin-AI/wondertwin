package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-clerk/internal/store"
)

// createUserRequest is the JSON body for POST /v1/users.
type createUserRequest struct {
	ExternalID      string         `json:"external_id,omitempty"`
	EmailAddress    []string       `json:"email_address,omitempty"`
	PhoneNumber     []string       `json:"phone_number,omitempty"`
	Username        string         `json:"username,omitempty"`
	FirstName       string         `json:"first_name,omitempty"`
	LastName        string         `json:"last_name,omitempty"`
	Password        string         `json:"password,omitempty"`
	PublicMetadata  map[string]any `json:"public_metadata,omitempty"`
	PrivateMetadata map[string]any `json:"private_metadata,omitempty"`
	UnsafeMetadata  map[string]any `json:"unsafe_metadata,omitempty"`
}

// updateUserRequest is the JSON body for PATCH /v1/users/{id}.
type updateUserRequest struct {
	ExternalID      *string        `json:"external_id,omitempty"`
	Username        *string        `json:"username,omitempty"`
	FirstName       *string        `json:"first_name,omitempty"`
	LastName        *string        `json:"last_name,omitempty"`
	Password        *string        `json:"password,omitempty"`
	PublicMetadata  map[string]any `json:"public_metadata,omitempty"`
	PrivateMetadata map[string]any `json:"private_metadata,omitempty"`
	UnsafeMetadata  map[string]any `json:"unsafe_metadata,omitempty"`
}

// CreateUser handles POST /v1/users.
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		clerkError(w, http.StatusBadRequest, "form_param_invalid",
			"Invalid request body.", err.Error())
		return
	}

	id := h.store.Users.NextID()
	now := store.Now()

	// Build email addresses
	emails := make([]store.EmailAddress, 0, len(req.EmailAddress))
	var primaryEmailID string
	for i, email := range req.EmailAddress {
		emailID := fmt.Sprintf("idn_%s_%d", id, i)
		emails = append(emails, store.EmailAddress{
			ID:           emailID,
			Object:       "email_address",
			EmailAddress: email,
			Verification: &store.Verification{
				Status:   "verified",
				Strategy: "from_oauth_google",
			},
			LinkedTo: []store.LinkedTo{
				{ID: id, Type: "user"},
			},
		})
		if i == 0 {
			primaryEmailID = emailID
		}
	}

	// Build phone numbers
	phones := make([]store.PhoneNumber, 0, len(req.PhoneNumber))
	var primaryPhoneID string
	for i, phone := range req.PhoneNumber {
		phoneID := fmt.Sprintf("phn_%s_%d", id, i)
		phones = append(phones, store.PhoneNumber{
			ID:          phoneID,
			Object:      "phone_number",
			PhoneNumber: phone,
			Verification: &store.Verification{
				Status:   "verified",
				Strategy: "phone_code",
			},
		})
		if i == 0 {
			primaryPhoneID = phoneID
		}
	}

	publicMeta := req.PublicMetadata
	if publicMeta == nil {
		publicMeta = make(map[string]any)
	}

	user := store.User{
		ID:                  id,
		Object:              "user",
		ExternalID:          req.ExternalID,
		PrimaryEmailAddress: primaryEmailID,
		PrimaryPhoneNumber:  primaryPhoneID,
		Username:            req.Username,
		FirstName:           req.FirstName,
		LastName:            req.LastName,
		EmailAddresses:      emails,
		PhoneNumbers:        phones,
		PublicMetadata:      publicMeta,
		PrivateMetadata:     req.PrivateMetadata,
		UnsafeMetadata:      req.UnsafeMetadata,
		PasswordEnabled:     req.Password != "",
		PasswordHash:        req.Password, // stored in plaintext — twin only, never production
		TwoFactorEnabled:    false,
		Banned:              false,
		Locked:              false,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	h.store.Users.Set(id, user)
	twincore.JSON(w, http.StatusOK, user)
}

// GetUser handles GET /v1/users/{id}.
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	user, ok := h.store.Users.Get(id)
	if !ok {
		clerkError(w, http.StatusNotFound, "resource_not_found",
			"User not found.",
			fmt.Sprintf("No user was found with id %s.", id))
		return
	}

	twincore.JSON(w, http.StatusOK, user)
}

// UpdateUser handles PATCH /v1/users/{id}.
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	user, ok := h.store.Users.Get(id)
	if !ok {
		clerkError(w, http.StatusNotFound, "resource_not_found",
			"User not found.",
			fmt.Sprintf("No user was found with id %s.", id))
		return
	}

	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		clerkError(w, http.StatusBadRequest, "form_param_invalid",
			"Invalid request body.", err.Error())
		return
	}

	if req.ExternalID != nil {
		user.ExternalID = *req.ExternalID
	}
	if req.Username != nil {
		user.Username = *req.Username
	}
	if req.FirstName != nil {
		user.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		user.LastName = *req.LastName
	}
	if req.Password != nil {
		user.PasswordEnabled = *req.Password != ""
		user.PasswordHash = *req.Password
	}
	if req.PublicMetadata != nil {
		user.PublicMetadata = req.PublicMetadata
	}
	if req.PrivateMetadata != nil {
		user.PrivateMetadata = req.PrivateMetadata
	}
	if req.UnsafeMetadata != nil {
		user.UnsafeMetadata = req.UnsafeMetadata
	}

	user.UpdatedAt = store.Now()
	h.store.Users.Set(id, user)

	twincore.JSON(w, http.StatusOK, user)
}

// DeleteUser handles DELETE /v1/users/{id}.
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if !h.store.Users.Delete(id) {
		clerkError(w, http.StatusNotFound, "resource_not_found",
			"User not found.",
			fmt.Sprintf("No user was found with id %s.", id))
		return
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"id":      id,
		"object":  "user",
		"deleted": true,
	})
}

// ListUsers handles GET /v1/users.
// Supports query params: limit, offset, email_address[], user_id[]
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	limit := 10
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}

	// Filter by email_address[]
	emailFilters := r.URL.Query()["email_address"]
	// Filter by user_id[]
	userIDFilters := r.URL.Query()["user_id"]

	allUsers := h.store.Users.List()

	// Apply filters
	var filtered []store.User
	for _, user := range allUsers {
		if len(emailFilters) > 0 {
			found := false
			for _, email := range user.EmailAddresses {
				for _, filter := range emailFilters {
					if strings.EqualFold(email.EmailAddress, filter) {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if !found {
				continue
			}
		}
		if len(userIDFilters) > 0 {
			found := false
			for _, uid := range userIDFilters {
				if user.ID == uid {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		filtered = append(filtered, user)
	}

	// Apply pagination
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
		page = []store.User{}
	}

	twincore.JSON(w, http.StatusOK, store.ClerkList[store.User]{
		Data:       page,
		TotalCount: total,
	})
}
