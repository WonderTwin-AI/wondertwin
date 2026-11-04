package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// account is an Account on the wire, with UpdatedDateUTC as /Date(ms+0000)/.
type account struct {
	store.Account
	UpdatedDateUTC Msdate `json:"UpdatedDateUTC"`
}

// Accounts serves GET /api.xro/2.0/Accounts: the tenant's chart, in order.
func (h *Handler) Accounts(w http.ResponseWriter, r *http.Request) {
	out := []account{}
	for _, a := range h.store.Accounts.For(tenantID(r)).List() {
		out = append(out, account{a, Msdate(a.UpdatedDateUTC)})
	}
	h.writeEnvelope(w, http.StatusOK, "Accounts", out)
}

// accountClass is the Class Xero derives from an account's Type. Class is
// read-only, so a Class the client sends is ignored. The mapping is inference
// from the documented class and type lists (crud-021).
var accountClass = map[string]string{
	"BANK": "ASSET", "CURRENT": "ASSET", "FIXED": "ASSET", "INVENTORY": "ASSET", "NONCURRENT": "ASSET", "PREPAYMENT": "ASSET",
	"EQUITY":     "EQUITY",
	"DEPRECIATN": "EXPENSE", "DIRECTCOSTS": "EXPENSE", "EXPENSE": "EXPENSE", "OVERHEADS": "EXPENSE",
	"SUPERANNUATIONEXPENSE": "EXPENSE", "WAGESEXPENSE": "EXPENSE",
	"CURRLIAB": "LIABILITY", "LIABILITY": "LIABILITY", "TERMLIAB": "LIABILITY", "PAYGLIABILITY": "LIABILITY",
	"SUPERANNUATIONLIABILITY": "LIABILITY",
	"OTHERINCOME":             "REVENUE", "REVENUE": "REVENUE", "SALES": "REVENUE",
}

// CreateAccount serves PUT /api.xro/2.0/Accounts: it adds one account to the
// tenant's chart, sent bare as xero-python sends it. Code (at most 10
// characters), Name (at most 150) and Type are required, and Name and Code are
// unique in the chart. Only the duplicate Name message is documented; the
// others are guesses (crud-021). The account is ACTIVE, its Class follows its
// Type, and reset removes it with every other account a client added.
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var in account
	if err == nil {
		err = decodeBody(raw, &in)
	}
	if err != nil {
		h.writeBadBody(w, r, err)
		return
	}
	accounts := h.store.Accounts.For(tenantID(r))
	if msgs := validateAccount(in.Account, accounts.List()); len(msgs) > 0 {
		writeValidationException(w, validationElement{Object: json.RawMessage(raw), Messages: msgs})
		return
	}
	a := in.Account
	a.AccountID, a.UpdatedDateUTC = h.store.NewID(), h.store.Clock.Now().UTC()
	a.Class, a.Status = accountClass[a.Type], "ACTIVE"
	if a.TaxType == "" {
		a.TaxType = "NONE"
	}
	accounts.Set(a.AccountID, a)
	h.writeEnvelope(w, http.StatusOK, "Accounts", []account{{a, Msdate(a.UpdatedDateUTC)}})
}

func validateAccount(a store.Account, chart []store.Account) []string {
	var msgs []string
	switch {
	case strings.TrimSpace(a.Code) == "":
		msgs = append(msgs, "Please enter a Code.")
	case utf8.RuneCountInString(a.Code) > 10:
		msgs = append(msgs, "The Code must not be more than 10 characters long.")
	}
	switch {
	case strings.TrimSpace(a.Name) == "":
		msgs = append(msgs, "Please enter a Name.")
	case utf8.RuneCountInString(a.Name) > 150:
		msgs = append(msgs, "The Name must not be more than 150 characters long.")
	}
	if _, ok := accountClass[a.Type]; !ok {
		msgs = append(msgs, fmt.Sprintf("Account Type '%s' is not valid.", a.Type))
	}
	for _, c := range chart {
		if a.Name != "" && strings.EqualFold(c.Name, a.Name) {
			msgs = append(msgs, "Please enter a unique Name.")
		}
		if a.Code != "" && strings.EqualFold(c.Code, a.Code) {
			msgs = append(msgs, "Please enter a unique Code.")
		}
	}
	return msgs
}
