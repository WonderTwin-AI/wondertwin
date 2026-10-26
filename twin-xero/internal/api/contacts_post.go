package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// UpdateOrCreateContacts serves POST /api.xro/2.0/Contacts (crud-009): a contact
// that carries the ContactID of a stored contact updates it, and one that has no
// ContactID is created by the rules of PUT /Contacts. An update is partial: only
// the fields the body carries change, so a contact read earlier can be sent back
// with one field changed. It is never an upsert by Name: a Name that belongs to
// another contact is refused as PUT refuses it. If any contact is invalid nothing
// is written and the answer is the 400 ValidationException PUT gives; a ContactID
// that matches nothing is a 404 whose body is a guess. Each write sends a CONTACT
// event, CREATE or UPDATE.
func (h *Handler) UpdateOrCreateContacts(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	var in struct{ Contacts []json.RawMessage }
	if err == nil {
		err = json.Unmarshal(raw, &in)
	}
	if err == nil && in.Contacts == nil {
		in.Contacts = []json.RawMessage{raw}
	}
	if err != nil {
		h.writeBadBody(w, r, raw, err)
		return
	}

	tenant := tenantID(r)
	contacts := h.store.Contacts.For(tenant)
	stored := contacts.List()
	byID := make(map[string]store.Contact, len(stored))
	for _, c := range stored {
		byID[c.ContactID] = c
	}

	type write struct {
		c       contact
		created bool
	}
	writes := make([]write, len(in.Contacts))
	for i, one := range in.Contacts {
		var sent contact
		if err := json.Unmarshal(one, &sent); err != nil {
			h.writeBadBody(w, r, one, err)
			return
		}
		if sent.ContactID == "" {
			writes[i] = write{c: sent, created: true}
			continue
		}
		old, ok := byID[sent.ContactID]
		if !ok {
			h.writeProblem(w, r, http.StatusNotFound, "Not Found", "The resource you have specified cannot be found")
			return
		}
		var copied store.Contact
		kept, _ := json.Marshal(old)
		_ = json.Unmarshal(kept, &copied)
		merged := contact{copied, Msdate(old.UpdatedDateUTC)}
		_ = json.Unmarshal(one, &merged) // only the fields the body carries replace the stored ones
		merged.Contact.ContactID = old.ContactID
		writes[i] = write{c: merged}
	}

	var elements []validationElement
	invalid := false
	for i, wr := range writes {
		var others []store.Contact
		for _, c := range stored {
			if c.ContactID != wr.c.ContactID {
				others = append(others, c)
			}
		}
		for _, prior := range writes[:i] {
			others = append(others, prior.c.Contact)
		}
		msgs := validateContact(wr.c.Contact, others)
		invalid = invalid || len(msgs) > 0
		elements = append(elements, validationElement{Object: wr.c, Messages: msgs})
	}
	if invalid {
		writeValidationException(w, elements...)
		return
	}

	now, out := h.store.Clock.Now().UTC(), make([]contact, len(writes))
	for i, wr := range writes {
		c, eventType := wr.c, "UPDATE"
		if wr.created {
			c.Contact.ContactID, eventType = h.store.NewID(), "CREATE"
			if c.Contact.ContactStatus == "" {
				c.Contact.ContactStatus = "ACTIVE"
			}
		}
		c.Contact.UpdatedDateUTC = now
		contacts.Set(c.Contact.ContactID, c.Contact)
		h.emit(r, tenant, "CONTACT", eventType, c.Contact.ContactID, nil)
		out[i] = wireContact(c.Contact)
	}
	h.writeEnvelope(w, http.StatusOK, "Contacts", out)
}
