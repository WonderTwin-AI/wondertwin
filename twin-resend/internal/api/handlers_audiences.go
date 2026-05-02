package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// --- Audiences ---

// ListAudiences handles GET /audiences
func (h *Handler) ListAudiences(w http.ResponseWriter, r *http.Request) {
	audiences := h.store.Audiences.Filter(func(_ string, _ store.Audience) bool { return true })
	twincore.JSON(w, http.StatusOK, map[string]any{"object": "list", "data": audiences})
}

// GetAudience handles GET /audiences/{id}
func (h *Handler) GetAudience(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	audience, ok := h.store.Audiences.Get(id)
	if !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "Audience not found", "name": "not_found",
		})
		return
	}
	twincore.JSON(w, http.StatusOK, audience)
}

// CreateAudience handles POST /audiences
func (h *Handler) CreateAudience(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"statusCode": 400, "message": "name is required", "name": "validation_error",
		})
		return
	}

	id := h.resendID()
	audience := store.Audience{
		ID:        id,
		Object:    "audience",
		Name:      req.Name,
		CreatedAt: h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z"),
	}

	h.store.Audiences.Set(id, audience)
	twincore.JSON(w, http.StatusCreated, audience)
}

// DeleteAudience handles DELETE /audiences/{id}
func (h *Handler) DeleteAudience(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := h.store.Audiences.Get(id); !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "Audience not found", "name": "not_found",
		})
		return
	}
	// Delete audience and its contacts
	h.store.Contacts.Filter(func(_ string, c store.Contact) bool {
		if c.AudienceID == id {
			h.store.Contacts.Delete(c.ID)
		}
		return false
	})
	h.store.Audiences.Delete(id)
	twincore.JSON(w, http.StatusOK, map[string]any{"object": "audience", "id": id, "deleted": true})
}

// --- Contacts ---

// ListContacts handles GET /audiences/{audience_id}/contacts
func (h *Handler) ListContacts(w http.ResponseWriter, r *http.Request) {
	audienceID := chi.URLParam(r, "audience_id")
	if _, ok := h.store.Audiences.Get(audienceID); !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "Audience not found", "name": "not_found",
		})
		return
	}

	contacts := h.store.Contacts.Filter(func(_ string, c store.Contact) bool {
		return c.AudienceID == audienceID
	})
	twincore.JSON(w, http.StatusOK, map[string]any{"object": "list", "data": contacts})
}

// GetContact handles GET /audiences/{audience_id}/contacts/{id}
func (h *Handler) GetContact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	contact, ok := h.store.Contacts.Get(id)
	if !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "Contact not found", "name": "not_found",
		})
		return
	}
	twincore.JSON(w, http.StatusOK, contact)
}

// CreateContact handles POST /audiences/{audience_id}/contacts
func (h *Handler) CreateContact(w http.ResponseWriter, r *http.Request) {
	audienceID := chi.URLParam(r, "audience_id")
	if _, ok := h.store.Audiences.Get(audienceID); !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "Audience not found", "name": "not_found",
		})
		return
	}

	var req struct {
		Email        string `json:"email"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		Unsubscribed bool   `json:"unsubscribed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		twincore.JSON(w, http.StatusBadRequest, map[string]any{
			"statusCode": 400, "message": "email is required", "name": "validation_error",
		})
		return
	}

	id := h.resendID()
	contact := store.Contact{
		ID:           id,
		Object:       "contact",
		Email:        req.Email,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Unsubscribed: req.Unsubscribed,
		CreatedAt:    h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z"),
		AudienceID:   audienceID,
	}

	h.store.Contacts.Set(id, contact)
	twincore.JSON(w, http.StatusCreated, contact)
}

// UpdateContact handles PATCH /audiences/{audience_id}/contacts/{id}
func (h *Handler) UpdateContact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	contact, ok := h.store.Contacts.Get(id)
	if !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "Contact not found", "name": "not_found",
		})
		return
	}

	var req struct {
		FirstName    *string `json:"first_name,omitempty"`
		LastName     *string `json:"last_name,omitempty"`
		Unsubscribed *bool   `json:"unsubscribed,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
		if req.FirstName != nil {
			contact.FirstName = *req.FirstName
		}
		if req.LastName != nil {
			contact.LastName = *req.LastName
		}
		if req.Unsubscribed != nil {
			contact.Unsubscribed = *req.Unsubscribed
		}
	}

	h.store.Contacts.Set(id, contact)
	twincore.JSON(w, http.StatusOK, contact)
}

// DeleteContact handles DELETE /audiences/{audience_id}/contacts/{id}
func (h *Handler) DeleteContact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := h.store.Contacts.Get(id); !ok {
		twincore.JSON(w, http.StatusNotFound, map[string]any{
			"statusCode": 404, "message": "Contact not found", "name": "not_found",
		})
		return
	}
	h.store.Contacts.Delete(id)
	twincore.JSON(w, http.StatusOK, map[string]any{"object": "contact", "id": id, "deleted": true})
}
