package api

import (
	"net/http"
	"slices"

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

// accountIDs reads options.account_ids, the optional filter both account routes
// take. ok is false for a malformed options object or ids.
func accountIDs(body map[string]any) (ids []string, ok bool) {
	opts, isObj := body["options"].(map[string]any)
	if body["options"] != nil && !isObj {
		return nil, false
	}
	return asStrings(opts["account_ids"])
}

// writeAccounts answers an Item's accounts with the Item, in Position order, or
// only those named in options.account_ids. An id the Item does not have is
// INVALID_ACCOUNT_ID, documented.
func (h *Handler) writeAccounts(w http.ResponseWriter, r *http.Request) {
	call, _ := CallFrom(r)
	ids, ok := accountIDs(call.Body)
	if !ok {
		h.invalidBody(w)
		return
	}
	it, ok := h.liveItemFor(w, r)
	if !ok {
		return
	}
	accounts := []accountJSON{}
	for _, a := range h.store.AccountsForItem(it.ID) {
		if len(ids) == 0 || slices.Contains(ids, a.ID) {
			accounts = append(accounts, newAccountJSON(a))
		}
	}
	if len(ids) > 0 && len(accounts) < len(slices.Compact(slices.Sorted(slices.Values(ids)))) {
		h.writeError(w, http.StatusBadRequest, "INVALID_INPUT", "INVALID_ACCOUNT_ID",
			"failed to find requested account ID for requested item")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts, "item": newItemJSON(it)})
}

// accountsGet is POST /accounts/get.
func (h *Handler) accountsGet(w http.ResponseWriter, r *http.Request) { h.writeAccounts(w, r) }

// accountsBalanceGet is POST /accounts/balance/get. Plaid fetches balances live
// where /accounts/get may serve them cached; the emulator has one balance per
// account, so the two agree, which is the consistency Plaid's Sandbox lacks
// between products (quirk LIFECYCLE-006, a declared improvement).
func (h *Handler) accountsBalanceGet(w http.ResponseWriter, r *http.Request) { h.writeAccounts(w, r) }
