package store

import "sort"

// Balances are an account's amounts. A nil amount is one Plaid sends as null.
type Balances struct {
	Available       *float64 `json:"available"`
	Current         *float64 `json:"current"`
	Limit           *float64 `json:"limit"`
	ISOCurrencyCode string   `json:"iso_currency_code,omitempty"`
}

// AccountNumbers are the US bank numbers Auth returns for an account.
type AccountNumbers struct {
	Account     string `json:"account"`
	Routing     string `json:"routing"`
	WireRouting string `json:"wire_routing,omitempty"`
}

// Account is one account on an Item. PersistentID is left empty for most
// institutions, and an empty value is omitted from responses, never sent as
// null (quirk IDENTITY-003).
type Account struct {
	ID           string   `json:"account_id"`
	ItemID       string   `json:"item_id"`
	Position     int      `json:"position"`
	Name         string   `json:"name"`
	OfficialName string   `json:"official_name,omitempty"`
	Mask         string   `json:"mask,omitempty"`
	Type         string   `json:"type"`
	Subtype      string   `json:"subtype,omitempty"`
	PersistentID string   `json:"persistent_account_id,omitempty"`
	Balances     Balances `json:"balances"`
	// Numbers are set only on accounts Auth serves: checking, savings and cash management.
	Numbers *AccountNumbers `json:"numbers,omitempty"`
}

// PutAccount stores an account under its ID.
func (s *MemoryStore) PutAccount(a Account) { s.Accounts.Set(a.ID, a) }

// GetAccount returns the account with the given ID.
func (s *MemoryStore) GetAccount(id string) (Account, bool) { return s.Accounts.Get(id) }

// AccountsForItem returns an Item's accounts in Position order. A state file
// loads in ID order, so Position, not insertion, keeps the order stable.
func (s *MemoryStore) AccountsForItem(itemID string) []Account {
	out := s.Accounts.Filter(func(_ string, a Account) bool { return a.ItemID == itemID })
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Position != out[j].Position {
			return out[i].Position < out[j].Position
		}
		return out[i].ID < out[j].ID
	})
	return out
}
