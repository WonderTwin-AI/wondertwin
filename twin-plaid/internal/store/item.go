package store

import "time"

// Item is a login at an institution, the unit an access token is bound to.
// An empty string stands for a value Plaid sends as null or leaves out; the
// API layer decides which, per field (quirk ENVELOPE-004).
type Item struct {
	ID                string   `json:"item_id"`
	InstitutionID     string   `json:"institution_id,omitempty"`
	Webhook           string   `json:"webhook,omitempty"`
	Products          []string `json:"products,omitempty"`
	BilledProducts    []string `json:"billed_products,omitempty"`
	ConsentedProducts []string `json:"consented_products,omitempty"`
	UpdateType        string   `json:"update_type,omitempty"`
	ErrorCode         string   `json:"error_code,omitempty"`
	// SyncCalled is set once /transactions/sync has run on the Item: Plaid sends
	// SYNC_UPDATES_AVAILABLE only after that (quirk DELIVERY-006).
	SyncCalled bool `json:"sync_called,omitempty"`
	// NotifiedChanges is how many entries of the Item's change log the last
	// refresh webhook covered; a refresh tells the receiver only about newer ones.
	NotifiedChanges int       `json:"notified_changes,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// PutItem stores an item under its ID.
func (s *MemoryStore) PutItem(it Item) { s.Items.Set(it.ID, it) }

// GetItem returns the item with the given ID.
func (s *MemoryStore) GetItem(id string) (Item, bool) { return s.Items.Get(id) }

// RemoveItem deletes an item, its accounts, processor tokens and transactions.
func (s *MemoryStore) RemoveItem(id string) bool {
	if !s.Items.Delete(id) {
		return false
	}
	for _, a := range s.AccountsForItem(id) {
		s.Accounts.Delete(a.ID)
	}
	s.removeProcessorTokens(id)
	s.removeTransactions(id)
	return true
}

// UpdateItem applies fn to the stored Item under a lock and returns the result.
// Handlers use it instead of reading an Item, changing it and calling PutItem,
// which writes back every field and can undo a change another request made in
// between.
func (s *MemoryStore) UpdateItem(id string, fn func(*Item)) (Item, bool) {
	s.itemMu.Lock()
	defer s.itemMu.Unlock()
	it, ok := s.Items.Get(id)
	if !ok {
		return Item{}, false
	}
	fn(&it)
	s.Items.Set(id, it)
	return it, true
}

// SetItemError puts an Item into an error state, by code; an empty code clears it.
func (s *MemoryStore) SetItemError(id, code string) bool {
	_, ok := s.UpdateItem(id, func(it *Item) { it.ErrorCode = code })
	return ok
}
