package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// bankTransactionTypes are the types a client may create. RECEIVE-TRANSFER and
// SPEND-TRANSFER are returned by GET only: Xero documents that transfers cannot
// be created through PUT or POST.
var bankTransactionTypes = map[string]bool{
	"RECEIVE": true, "SPEND": true,
	"RECEIVE-OVERPAYMENT": true, "SPEND-OVERPAYMENT": true,
	"RECEIVE-PREPAYMENT": true, "SPEND-PREPAYMENT": true,
}

// CreateBankTransactions serves PUT /api.xro/2.0/BankTransactions: it creates
// the bank transactions in {"BankTransactions": [...]} or the one sent bare
// (crud-001), and answers the plural envelope with status 200. If any is
// invalid nothing is created and the answer is a 400 ValidationException that
// echoes every submitted one, unless summarizeErrors=false, which creates the
// valid ones and answers each invalid one in place (crud-006). Xero sends no
// webhook event for a bank transaction.
func (h *Handler) CreateBankTransactions(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	var in struct{ BankTransactions []json.RawMessage }
	if err == nil {
		err = json.Unmarshal(raw, &in)
	}
	if err == nil && in.BankTransactions == nil {
		in.BankTransactions = []json.RawMessage{raw}
	}
	sent := make([]bankTransaction, len(in.BankTransactions))
	for i, item := range in.BankTransactions {
		if err == nil {
			err = decodeBody(item, &sent[i])
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
	var elements []validationElement
	made := make([]store.BankTransaction, len(sent))
	invalid := false
	for i := range sent {
		bt, msgs := h.buildBankTransaction(tenant, inBankTransaction(sent[i]), unitdp)
		invalid = invalid || len(msgs) > 0
		elements = append(elements, validationElement{Object: in.BankTransactions[i], Messages: msgs})
		made[i] = bt
	}
	if invalid && !strings.EqualFold(r.URL.Query().Get("summarizeErrors"), "false") {
		writeValidationException(w, elements...)
		return
	}
	saved := h.store.BankTransactions.For(tenant)
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

// buildBankTransaction validates a bank transaction and fills in what Xero
// computes or defaults: the ID, line IDs and accounts, totals, Status
// AUTHORISED (bank transactions have no draft), the currency of the bank
// account, and today's date when none is sent (time-004, a guess from invoices).
// Anything the client sent for those is ignored.
func (h *Handler) buildBankTransaction(tenant string, in store.BankTransaction, unitdp int) (store.BankTransaction, []string) {
	now := h.store.Clock.Now().UTC()
	var msgs []string
	if !bankTransactionTypes[in.Type] {
		msgs = append(msgs, fmt.Sprintf("Bank transaction Type '%s' cannot be created. Use RECEIVE, SPEND, or an overpayment or prepayment type.", in.Type))
	}
	msgs = append(msgs, bankTransactionLines(&in, unitdp)...)
	msgs = append(msgs, h.validateBankTransaction(tenant, in)...)
	chart := h.store.Accounts.For(tenant).List()
	for i := range in.LineItems {
		in.LineItems[i].LineItemID, in.LineItems[i].AccountID = h.store.NewID(), ""
		for _, a := range chart {
			if a.Code == in.LineItems[i].AccountCode {
				in.LineItems[i].AccountID = a.AccountID
			}
		}
	}
	if len(msgs) > 0 {
		return in, msgs
	}
	bank, ok := store.Account{}, false
	if in.BankAccount != nil {
		bank, ok = h.findAccount(tenant, &store.PaymentAccount{AccountID: in.BankAccount.AccountID, Code: in.BankAccount.Code}, "")
	}
	if ok {
		in.BankAccount = &store.AccountRef{AccountID: bank.AccountID, Code: bank.Code}
	}
	if in.Date == nil {
		d := now.Truncate(24 * time.Hour)
		in.Date = &d
	}
	switch {
	case bank.CurrencyCode != "":
		in.CurrencyCode = bank.CurrencyCode
	case in.CurrencyCode == "":
		in.CurrencyCode = "USD"
		if org, found := h.store.Organisations.Get(tenant); found && org.BaseCurrency != "" {
			in.CurrencyCode = org.BaseCurrency
		}
	}
	in.BankTransactionID, in.Status, in.StatusAttributeString = h.store.NewID(), "AUTHORISED", "OK"
	in.HasAttachments, in.ValidationErrors, in.Warnings = false, nil, nil
	in.PrepaymentID, in.OverpaymentID = "", ""
	in.UpdatedDateUTC = now
	return in, nil
}
