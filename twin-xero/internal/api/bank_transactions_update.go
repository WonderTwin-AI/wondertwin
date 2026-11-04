package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// UpdateBankTransaction serves POST /api.xro/2.0/BankTransactions/{BankTransactionID}.
// The body is a partial update in {"BankTransactions": [...]} or bare: only the
// fields it carries change, and {"Status": "DELETED"} deletes, as Xero documents.
// The answer is the plural envelope; a refused update is a 400
// ValidationException. Xero documents no summarizeErrors on this path. An
// unknown ID is a 404 whose body is a guess.
func (h *Handler) UpdateBankTransaction(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	var env struct{ BankTransactions []json.RawMessage }
	if err == nil {
		err = json.Unmarshal(raw, &env)
	}
	one := json.RawMessage(raw)
	if err == nil && env.BankTransactions != nil {
		if len(env.BankTransactions) == 0 {
			err = fmt.Errorf("no bank transaction")
		} else {
			one = env.BankTransactions[0]
		}
	}
	var sent bankTransaction
	keys := map[string]json.RawMessage{}
	if err == nil {
		err = json.Unmarshal(one, &keys)
	}
	if err == nil {
		err = decodeBody(one, &sent)
	}
	if err != nil {
		h.writeBadBody(w, r, err)
		return
	}
	tenant := tenantID(r)
	saved := h.store.BankTransactions.For(tenant)
	var old store.BankTransaction
	found := false
	for _, bt := range saved.List() {
		if strings.EqualFold(bt.BankTransactionID, chi.URLParam(r, "BankTransactionID")) {
			old, found = bt, true
		}
	}
	if !found {
		h.writeProblem(w, r, http.StatusNotFound, "Not Found", "The resource you have specified cannot be found")
		return
	}
	unitdp := 2
	if r.URL.Query().Get("unitdp") == "4" {
		unitdp = 4
	}
	upd, msgs := h.applyBankTransactionUpdate(tenant, old, inBankTransaction(sent), keys, unitdp)
	if len(msgs) > 0 {
		writeValidationException(w, validationElement{Object: one, Messages: msgs})
		return
	}
	saved.Set(upd.BankTransactionID, upd)
	h.writeEnvelope(w, http.StatusOK, "BankTransactions", []any{h.wireBankTransaction(tenant, upd)})
}

// applyBankTransactionUpdate overlays the fields the update carries on the
// stored bank transaction and returns the result, or the reasons it is refused.
// Xero documents that only SPEND and RECEIVE can be updated or deleted, and that
// a deleted one is gone. Line items are matched by LineItemID: a line with a
// known ID updates that line, one without is added, and a stored line left out
// is removed. Unsettled (crud-019): how VOIDED is reached and whether a
// reconciled transaction may change; this app emulator refuses VOIDED and any
// update once IsReconciled is true. The wording of the refusals is a guess.
func (h *Handler) applyBankTransactionUpdate(tenant string, old, in store.BankTransaction, keys map[string]json.RawMessage, unitdp int) (store.BankTransaction, []string) {
	switch {
	case old.Status == "DELETED":
		return old, []string{"A bank transaction with the status DELETED cannot be updated."}
	case old.Type != "RECEIVE" && old.Type != "SPEND":
		return old, []string{fmt.Sprintf("A bank transaction of Type %s cannot be updated or deleted.", old.Type)}
	case old.IsReconciled:
		return old, []string{"A reconciled bank transaction cannot be updated."}
	}
	out, msgs := old, []string(nil)
	set := func(key string, apply func()) {
		if _, ok := keys[key]; ok {
			apply()
		}
	}
	set("Type", func() {
		if in.Type != old.Type {
			msgs = append(msgs, "The Type of a bank transaction cannot be changed.")
		}
	})
	set("Status", func() {
		switch in.Status {
		case "", "AUTHORISED":
		case "DELETED":
			out.Status = "DELETED"
		default:
			msgs = append(msgs, fmt.Sprintf("A bank transaction cannot change from %s to %s.", old.Status, in.Status))
		}
	})
	set("Reference", func() { out.Reference = in.Reference })
	set("Url", func() { out.URL = in.URL })
	set("Date", func() { out.Date = in.Date })
	set("IsReconciled", func() { out.IsReconciled = in.IsReconciled })
	set("CurrencyRate", func() { out.CurrencyRate = in.CurrencyRate })
	set("LineAmountTypes", func() { out.LineAmountTypes = in.LineAmountTypes })
	set("Contact", func() { out.Contact = in.Contact })
	set("BankAccount", func() { out.BankAccount = in.BankAccount })

	_, linesSent := keys["LineItems"]
	_, typesSent := keys["LineAmountTypes"]
	if linesSent || typesSent {
		known := map[string]bool{}
		for _, l := range old.LineItems {
			known[l.LineItemID] = true
		}
		if linesSent {
			out.LineItems = in.LineItems
		} else {
			out.LineItems = append([]store.LineItem(nil), old.LineItems...)
			for i := range out.LineItems {
				out.LineItems[i].TaxAmount = nil
			}
		}
		for i := range out.LineItems {
			if !known[out.LineItems[i].LineItemID] {
				out.LineItems[i].LineItemID = ""
			}
		}
		msgs = append(msgs, bankTransactionLines(&out, unitdp)...)
		chart := h.store.Accounts.For(tenant).List()
		for i := range out.LineItems {
			if out.LineItems[i].LineItemID == "" {
				out.LineItems[i].LineItemID = h.store.NewID()
			}
			out.LineItems[i].AccountID = ""
			for _, a := range chart {
				if a.Code == out.LineItems[i].AccountCode {
					out.LineItems[i].AccountID = a.AccountID
				}
			}
		}
	}
	if out.Status != "DELETED" {
		msgs = append(msgs, h.validateBankTransaction(tenant, out)...)
	}
	if len(msgs) > 0 {
		return old, msgs
	}
	if out.BankAccount != nil {
		if a, ok := h.findAccount(tenant, &store.PaymentAccount{AccountID: out.BankAccount.AccountID, Code: out.BankAccount.Code}, ""); ok {
			out.BankAccount = &store.AccountRef{AccountID: a.AccountID, Code: a.Code}
			if a.CurrencyCode != "" {
				out.CurrencyCode = a.CurrencyCode
			}
		}
	}
	out.StatusAttributeString, out.UpdatedDateUTC = "OK", h.store.Clock.Now().UTC()
	return out, nil
}
