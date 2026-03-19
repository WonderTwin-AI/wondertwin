package api

import "github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"

// updateContactBalance recalculates the balance for a contact by scanning all invoices.
func (h *Handler) updateContactBalance(contactID string) {
	contact, ok := h.store.Contacts.Get(contactID)
	if !ok {
		return
	}

	var arOutstanding, apOutstanding float64
	invoices := h.store.Invoices.Filter(func(_ string, inv store.Invoice) bool {
		return inv.Contact.ContactID == contactID && inv.Status != "DELETED" && inv.Status != "VOIDED" && inv.Status != "DRAFT"
	})
	for _, inv := range invoices {
		if inv.AmountDue > 0 {
			if inv.Type == "ACCREC" {
				arOutstanding += inv.AmountDue
			} else if inv.Type == "ACCPAY" {
				apOutstanding += inv.AmountDue
			}
		}
	}

	contact.Balances = &store.ContactBalances{
		AccountsReceivable: store.ContactBalance{Outstanding: arOutstanding},
		AccountsPayable:    store.ContactBalance{Outstanding: apOutstanding},
	}
	h.store.Contacts.Set(contactID, contact)
}
