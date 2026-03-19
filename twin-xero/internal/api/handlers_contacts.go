package api

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (h *Handler) CreateContact(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Contacts []store.Contact `json:"Contacts"`
		// Single contact may also be at top level.
		Name         string `json:"Name"`
		EmailAddress string `json:"EmailAddress"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}

	contacts := req.Contacts
	if len(contacts) == 0 && req.Name != "" {
		contacts = []store.Contact{{Name: req.Name, EmailAddress: req.EmailAddress}}
	}
	if len(contacts) == 0 {
		xeroError(w, http.StatusBadRequest, "ValidationException", "At least one contact is required")
		return
	}

	var created []store.Contact
	for _, c := range contacts {
		if c.ContactID == "" {
			c.ContactID = newUUID()
		}
		if c.ContactStatus == "" {
			c.ContactStatus = "ACTIVE"
		}
		c.UpdatedDateUTC = store.XeroDateNow()
		h.store.Contacts.Set(c.ContactID, c)
		created = append(created, c)

		if h.dispatcher != nil {
			h.dispatcher.Enqueue("CONTACT.CREATE", map[string]any{
				"resourceId": c.ContactID,
				"eventType":  "CONTACT.CREATE",
			})
		}
	}

	xeroJSON(w, http.StatusOK, map[string]any{"Contacts": created})
}

func (h *Handler) ListContacts(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	all := h.store.Contacts.List()
	xeroJSON(w, http.StatusOK, map[string]any{"Contacts": paginate(all, page, pageSize)})
}

func (h *Handler) GetContact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ContactID")
	c, ok := h.store.Contacts.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "Contact not found")
		return
	}
	xeroJSON(w, http.StatusOK, map[string]any{"Contacts": []store.Contact{c}})
}

func (h *Handler) UpdateContact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ContactID")
	existing, ok := h.store.Contacts.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "Contact not found")
		return
	}

	var req struct {
		Contacts []store.Contact `json:"Contacts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}
	if len(req.Contacts) == 0 {
		xeroError(w, http.StatusBadRequest, "ValidationException", "At least one contact is required")
		return
	}

	c := req.Contacts[0]
	c.ContactID = id
	if c.ContactStatus == "" {
		c.ContactStatus = existing.ContactStatus
	}
	if c.Name == "" {
		c.Name = existing.Name
	}
	c.UpdatedDateUTC = store.XeroDateNow()
	h.store.Contacts.Set(id, c)

	if h.dispatcher != nil {
		h.dispatcher.Enqueue("CONTACT.UPDATE", map[string]any{
			"resourceId": id, "eventType": "CONTACT.UPDATE",
		})
	}

	xeroJSON(w, http.StatusOK, map[string]any{"Contacts": []store.Contact{c}})
}

func (h *Handler) DeleteContact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ContactID")
	c, ok := h.store.Contacts.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "Contact not found")
		return
	}
	c.ContactStatus = "ARCHIVED"
	c.UpdatedDateUTC = store.XeroDateNow()
	h.store.Contacts.Set(id, c)
	xeroJSON(w, http.StatusOK, map[string]any{"Contacts": []store.Contact{c}})
}
