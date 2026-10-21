package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/seed"
	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

// seedAccounts gives a new Item the default accounts, in their sample order.
func (h *Handler) seedAccounts(itemID string) {
	for i, a := range seed.DefaultAccounts() {
		a.ID, a.ItemID, a.Position = h.store.NewID(), itemID, i
		h.store.PutAccount(a)
	}
}

// balancesJSON carries all five balance fields: Plaid writes each one, as null
// when it has no value (quirk ENVELOPE-004).
type balancesJSON struct {
	Available              *float64 `json:"available"`
	Current                *float64 `json:"current"`
	Limit                  *float64 `json:"limit"`
	ISOCurrencyCode        *string  `json:"iso_currency_code"`
	UnofficialCurrencyCode *string  `json:"unofficial_currency_code"`
}

// accountJSON is an account. mask, official_name and subtype are required
// nullable; persistent_account_id is optional and not nullable, so it is
// omitted unless the institution gives one, never sent as null (quirk
// IDENTITY-003, the field that broke plaid-python against the commercial build).
type accountJSON struct {
	AccountID           string       `json:"account_id"`
	Balances            balancesJSON `json:"balances"`
	Mask                *string      `json:"mask"`
	Name                string       `json:"name"`
	OfficialName        *string      `json:"official_name"`
	Type                string       `json:"type"`
	Subtype             *string      `json:"subtype"`
	PersistentAccountID string       `json:"persistent_account_id,omitempty"`
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func newAccountJSON(a store.Account) accountJSON {
	b := a.Balances
	return accountJSON{
		AccountID: a.ID, Mask: strPtr(a.Mask), Name: a.Name, OfficialName: strPtr(a.OfficialName),
		Type: a.Type, Subtype: strPtr(a.Subtype), PersistentAccountID: a.PersistentID,
		Balances: balancesJSON{Available: b.Available, Current: b.Current, Limit: b.Limit,
			ISOCurrencyCode: strPtr(b.ISOCurrencyCode)},
	}
}

// accountsGet is POST /accounts/get: an Item's accounts, with the Item.
func (h *Handler) accountsGet(w http.ResponseWriter, r *http.Request) {
	it, ok := h.itemFor(w, r)
	if !ok {
		return
	}
	accounts := []accountJSON{}
	for _, a := range h.store.AccountsForItem(it.ID) {
		accounts = append(accounts, newAccountJSON(a))
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts, "item": newItemJSON(it)})
}
