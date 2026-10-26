package api

import (
	"net/http"
	"slices"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

// identityAccountJSON is an account with its owners, the shape /identity/get
// returns. owners is always an array.
type identityAccountJSON struct {
	accountJSON
	Owners []store.Owner `json:"owners"`
}

// accountsForRequest resolves the Item and the accounts the request names with
// options.account_ids, in Position order. It writes the error and returns ok
// false when it cannot: a bad Item, bad options, or an account id the Item lacks.
func (h *Handler) accountsForRequest(w http.ResponseWriter, r *http.Request, product string) (store.Item, []store.Account, bool) {
	call, _ := CallFrom(r)
	ids, ok := accountIDs(call.Body)
	if !ok {
		h.invalidBody(w)
		return store.Item{}, nil, false
	}
	return h.accountsForRequestIDs(w, r, ids, product)
}

// accountsForRequestIDs is accountsForRequest for ids the caller already read.
func (h *Handler) accountsForRequestIDs(w http.ResponseWriter, r *http.Request, ids []string, product string) (store.Item, []store.Account, bool) {
	it, ok := h.liveItemWith(w, r, product)
	if !ok {
		return it, nil, false
	}
	var out []store.Account
	for _, a := range h.store.AccountsForItem(it.ID) {
		if len(ids) == 0 || slices.Contains(ids, a.ID) {
			out = append(out, a)
		}
	}
	if len(ids) > 0 && len(out) < len(slices.Compact(slices.Sorted(slices.Values(ids)))) {
		h.writeError(w, http.StatusBadRequest, "INVALID_INPUT", "INVALID_ACCOUNT_ID",
			"failed to find requested account ID for requested item")
		return it, nil, false
	}
	return it, out, true
}

// identityGet is POST /identity/get: each account with its holders.
func (h *Handler) identityGet(w http.ResponseWriter, r *http.Request) {
	it, accounts, ok := h.accountsForRequest(w, r, "identity")
	if !ok {
		return
	}
	rows := []identityAccountJSON{}
	for _, a := range accounts {
		owners := a.Owners
		if owners == nil {
			owners = []store.Owner{}
		}
		rows = append(rows, identityAccountJSON{newAccountJSON(a), owners})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"accounts": rows, "item": newItemJSON(it)})
}

// matchUser is the user object of /identity/match. A field left out scores null.
type matchUser struct {
	legalName, phone, email string
	address                 store.AddressData
}

// matchAccountJSON is an account in an /identity/match response. Its balances
// are always null, as Plaid documents. Every score is null when either side has
// no data for it.
type matchAccountJSON struct {
	accountJSON
	LegalName    nameMatch    `json:"legal_name"`
	PhoneNumber  scoreOnly    `json:"phone_number"`
	EmailAddress scoreOnly    `json:"email_address"`
	Address      addressMatch `json:"address"`
}

// identityMatch is POST /identity/match.
func (h *Handler) identityMatch(w http.ResponseWriter, r *http.Request) {
	call, _ := CallFrom(r)
	user, ok := parseMatchUser(call.Body["user"])
	if !ok {
		h.invalidBody(w)
		return
	}
	if call.Body["user"] == nil {
		h.writeMissing(w, []string{"user"})
		return
	}
	it, accounts, ok := h.accountsForRequest(w, r, "identity")
	if !ok {
		return
	}
	rows := []matchAccountJSON{}
	for _, a := range accounts {
		base := newAccountJSON(a)
		base.Balances = balancesJSON{}
		rows = append(rows, matchAccountJSON{base, scoreName(user.legalName, a.Owners), scorePhone(user.phone, a.Owners),
			scoreEmail(user.email, a.Owners), scoreAddress(user.address, a.Owners)})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"accounts": rows, "item": newItemJSON(it)})
}

// parseMatchUser reads the user object; ok is false for a wrong type anywhere in it.
func parseMatchUser(v any) (u matchUser, ok bool) {
	if v == nil {
		return u, true
	}
	m, isObj := v.(map[string]any)
	if !isObj {
		return u, false
	}
	var ok1, ok2, ok3 bool
	u.legalName, ok1 = asString(m["legal_name"])
	u.phone, ok2 = asString(m["phone_number"])
	u.email, ok3 = asString(m["email_address"])
	addr, isAddr := m["address"].(map[string]any)
	if m["address"] != nil && !isAddr {
		return u, false
	}
	var a1, a2, a3, a4, a5 bool
	u.address.Street, a1 = asString(addr["street"])
	u.address.City, a2 = asString(addr["city"])
	u.address.Region, a3 = asString(addr["region"])
	u.address.PostalCode, a4 = asString(addr["postal_code"])
	u.address.Country, a5 = asString(addr["country"])
	return u, ok1 && ok2 && ok3 && a1 && a2 && a3 && a4 && a5
}
