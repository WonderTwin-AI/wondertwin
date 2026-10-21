package store

import "time"

// Item is a login at an institution, the unit an access token is bound to.
// An empty string stands for a value Plaid sends as null or leaves out; the
// API layer decides which, per field (quirk ENVELOPE-004).
type Item struct {
	ID                string    `json:"item_id"`
	InstitutionID     string    `json:"institution_id,omitempty"`
	Webhook           string    `json:"webhook,omitempty"`
	Products          []string  `json:"products,omitempty"`
	BilledProducts    []string  `json:"billed_products,omitempty"`
	ConsentedProducts []string  `json:"consented_products,omitempty"`
	UpdateType        string    `json:"update_type,omitempty"`
	ErrorCode         string    `json:"error_code,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

// PutItem stores an item under its ID.
func (s *MemoryStore) PutItem(it Item) { s.Items.Set(it.ID, it) }

// GetItem returns the item with the given ID.
func (s *MemoryStore) GetItem(id string) (Item, bool) { return s.Items.Get(id) }

// RemoveItem deletes an item and the accounts that belong to it.
func (s *MemoryStore) RemoveItem(id string) bool {
	if !s.Items.Delete(id) {
		return false
	}
	for _, a := range s.AccountsForItem(id) {
		s.Accounts.Delete(a.ID)
	}
	return true
}

// SetItemError puts an Item into an error state, by code; an empty code clears it.
func (s *MemoryStore) SetItemError(id, code string) bool {
	it, ok := s.Items.Get(id)
	if ok {
		it.ErrorCode = code
		s.Items.Set(id, it)
	}
	return ok
}
