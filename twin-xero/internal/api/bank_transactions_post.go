package api

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// UpdateOrCreateBankTransactions serves POST /api.xro/2.0/BankTransactions
// (crud-022): a bank transaction that carries the BankTransactionID of a stored
// one is updated by the rules of POST /BankTransactions/{BankTransactionID}, and
// one with no BankTransactionID is created by the rules of PUT
// /BankTransactions. Whether Xero also matches on another field, such as
// Reference, is not settled, so this does not. One invalid item fails the
// request with the 400 ValidationException, or with summarizeErrors=false the
// others are written and it is answered in place. A BankTransactionID that
// matches nothing is a 404 whose body is a guess, and nothing is written.
func (h *Handler) UpdateOrCreateBankTransactions(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	var in struct{ BankTransactions []json.RawMessage }
	if err == nil {
		err = json.Unmarshal(raw, &in)
	}
	if err == nil && in.BankTransactions == nil {
		in.BankTransactions = []json.RawMessage{raw}
	}
	sent := make([]bankTransaction, len(in.BankTransactions))
	keys := make([]map[string]json.RawMessage, len(in.BankTransactions))
	for i, item := range in.BankTransactions {
		if err == nil {
			err = decodeBody(item, &sent[i])
		}
		if err == nil {
			err = json.Unmarshal(item, &keys[i])
		}
	}
	if err != nil {
		h.writeBadBody(w, r, err)
		return
	}
	unitdp := 2
	if r.URL.Query().Get("unitdp") == "4" {
		unitdp = 4
	}

	tenant := tenantID(r)
	saved := h.store.BankTransactions.For(tenant)
	stored := saved.List()
	find := func(id string) int {
		return slices.IndexFunc(stored, func(e store.BankTransaction) bool { return strings.EqualFold(e.BankTransactionID, id) })
	}
	for _, s := range sent {
		if id := s.BankTransactionID; id != "" && find(id) < 0 {
			h.writeProblem(w, r, http.StatusNotFound, "Not Found", "The resource you have specified cannot be found")
			return
		}
	}

	made := make([]store.BankTransaction, len(sent))
	var elements []validationElement
	invalid := false
	for i, s := range sent {
		var msgs []string
		if id := s.BankTransactionID; id != "" {
			made[i], msgs = h.applyBankTransactionUpdate(tenant, stored[find(id)], inBankTransaction(s), keys[i], unitdp)
		} else {
			made[i], msgs = h.buildBankTransaction(tenant, inBankTransaction(s), unitdp)
		}
		invalid = invalid || len(msgs) > 0
		elements = append(elements, validationElement{Object: in.BankTransactions[i], Messages: msgs})
	}
	if invalid && !strings.EqualFold(r.URL.Query().Get("summarizeErrors"), "false") {
		writeValidationException(w, elements...)
		return
	}
	out := make([]any, len(made))
	for i, bt := range made {
		if msgs := elements[i].Messages; len(msgs) > 0 {
			out[i] = erroredInvoice(in.BankTransactions[i], msgs)
			continue
		}
		saved.Set(bt.BankTransactionID, bt)
		out[i] = h.wireBankTransaction(tenant, bt)
	}
	h.writeEnvelope(w, http.StatusOK, "BankTransactions", out)
}
