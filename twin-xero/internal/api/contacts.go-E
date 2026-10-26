package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// contact is a Contact on the wire, in and out: UpdatedDateUTC is /Date(ms+0000)/,
// and a client that reads a contact and sends it back sends that form.
type contact struct {
	store.Contact
	UpdatedDateUTC Msdate `json:"UpdatedDateUTC"`
}

// wireContact readies a stored contact for the wire: the lists Xero always
// returns are [], never null.
func wireContact(c store.Contact) contact {
	if c.ContactPersons == nil {
		c.ContactPersons = []store.ContactPerson{}
	}
	if c.Addresses == nil {
		c.Addresses = []store.Address{}
	}
	if c.Phones == nil {
		c.Phones = []store.Phone{}
	}
	if c.SalesTrackingCategories == nil {
		c.SalesTrackingCategories = []store.TrackingCategoryOf{}
	}
	if c.PurchasesTrackingCategories == nil {
		c.PurchasesTrackingCategories = []store.TrackingCategoryOf{}
	}
	if c.ContactGroups == nil {
		c.ContactGroups = []store.ContactGroup{}
	}
	return contact{c, Msdate(c.UpdatedDateUTC)}
}

// CreateContacts serves PUT /api.xro/2.0/Contacts: it creates the contacts in
// {"Contacts": [...]} or the one contact sent bare (crud-001). If any is invalid
// nothing is created and the answer is a 400 ValidationException that echoes
// every submitted contact (err-001). The emulator assigns the ContactID, so one
// sent is ignored, and a contact with no status is ACTIVE.
func (h *Handler) CreateContacts(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	var in struct{ Contacts []contact }
	if err == nil {
		err = decodeBody(raw, &in)
	}
	if err == nil && in.Contacts == nil {
		var one contact
		if err = decodeBody(raw, &one); err == nil {
			in.Contacts = []contact{one}
		}
	}
	if err != nil {
		h.writeBadBody(w, r, err)
		return
	}

	// An element echoes what was submitted, not the contact with its zero values.
	items := []json.RawMessage{raw}
	var env struct{ Contacts []json.RawMessage }
	if json.Unmarshal(raw, &env) == nil && env.Contacts != nil {
		items = env.Contacts
	}

	contacts := h.store.Contacts.For(tenantID(r))
	existing := contacts.List()
	var elements []validationElement
	invalid := false
	for i, c := range in.Contacts {
		msgs := validateContact(c.Contact, existing)
		invalid = invalid || len(msgs) > 0
		elements = append(elements, validationElement{Object: items[i], Messages: msgs})
		existing = append(existing, c.Contact) // a duplicate within the request is a duplicate
	}
	if invalid {
		writeValidationException(w, elements...)
		return
	}
	now, out := h.store.Clock.Now().UTC(), make([]contact, len(in.Contacts))
	for i, c := range in.Contacts {
		c.Contact.ContactID, c.Contact.UpdatedDateUTC = h.store.NewID(), now
		if c.Contact.ContactStatus == "" {
			c.Contact.ContactStatus = "ACTIVE"
		}
		contacts.Set(c.Contact.ContactID, c.Contact)
		h.emit(r, tenantID(r), "CONTACT", "CREATE", c.Contact.ContactID, nil)
		out[i] = wireContact(c.Contact)
	}
	h.writeEnvelope(w, http.StatusOK, "Contacts", out)
}
