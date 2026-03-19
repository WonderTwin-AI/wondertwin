package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/state/journal"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func (h *Handler) CreateBankTransaction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BankTransactions []store.BankTransaction `json:"BankTransactions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}
	if len(req.BankTransactions) == 0 {
		xeroError(w, http.StatusBadRequest, "ValidationException", "At least one bank transaction is required")
		return
	}

	var result []store.BankTransaction
	for _, bt := range req.BankTransactions {
		if bt.BankTransactionID == "" {
			bt.BankTransactionID = newUUID()
		}
		if bt.Status == "" {
			bt.Status = "AUTHORISED"
		}

		var subTotal float64
		for i := range bt.LineItems {
			li := &bt.LineItems[i]
			li.LineAmount = li.Quantity * li.UnitAmount
			subTotal += li.LineAmount
		}
		bt.SubTotal = subTotal
		bt.Total = subTotal
		bt.UpdatedDateUTC = store.XeroDateNow()

		// Validate contact if provided.
		if bt.Contact.ContactID != "" {
			if _, ok := h.store.Contacts.Get(bt.Contact.ContactID); !ok {
				xeroError(w, http.StatusBadRequest, "ValidationException", "Contact not found: "+bt.Contact.ContactID)
				return
			}
		}

		// Post journal entries for the bank transaction.
		if bt.Total > 0 {
			bankAcctID := bt.BankAccount.AccountID
			if bankAcctID == "" {
				bankAcctID = bt.BankAccount.Code
			}

			var entries []journal.Entry
			amount := int64(bt.Total * 100)
			currency := bt.CurrencyCode
			if currency == "" {
				currency = "USD"
			}

			if bt.Type == "RECEIVE" {
				// Debit bank account, credit line item accounts.
				entries = append(entries, journal.Entry{
					AccountID: bankAcctID, Type: journal.Debit, Amount: amount,
					Currency: currency, SourceType: "banktransaction", SourceID: bt.BankTransactionID,
				})
				for _, li := range bt.LineItems {
					if li.AccountCode != "" && li.LineAmount > 0 {
						acctID := h.resolveAccountCode(li.AccountCode)
						entries = append(entries, journal.Entry{
							AccountID: acctID, Type: journal.Credit, Amount: int64(li.LineAmount * 100),
							Currency: currency, SourceType: "banktransaction", SourceID: bt.BankTransactionID,
						})
					}
				}
			} else if bt.Type == "SPEND" {
				// Debit line item accounts, credit bank account.
				for _, li := range bt.LineItems {
					if li.AccountCode != "" && li.LineAmount > 0 {
						acctID := h.resolveAccountCode(li.AccountCode)
						entries = append(entries, journal.Entry{
							AccountID: acctID, Type: journal.Debit, Amount: int64(li.LineAmount * 100),
							Currency: currency, SourceType: "banktransaction", SourceID: bt.BankTransactionID,
						})
					}
				}
				entries = append(entries, journal.Entry{
					AccountID: bankAcctID, Type: journal.Credit, Amount: amount,
					Currency: currency, SourceType: "banktransaction", SourceID: bt.BankTransactionID,
				})
			}

			// If no line-level accounts, use a single fallback entry.
			if len(entries) == 1 {
				fallbackAcct := "income"
				fallbackType := journal.Credit
				if bt.Type == "SPEND" {
					fallbackAcct = "expense"
					fallbackType = journal.Debit
					entries = []journal.Entry{
						{AccountID: fallbackAcct, Type: fallbackType, Amount: amount,
							Currency: currency, SourceType: "banktransaction", SourceID: bt.BankTransactionID},
						{AccountID: bankAcctID, Type: journal.Credit, Amount: amount,
							Currency: currency, SourceType: "banktransaction", SourceID: bt.BankTransactionID},
					}
				} else {
					entries = append(entries, journal.Entry{
						AccountID: fallbackAcct, Type: fallbackType, Amount: amount,
						Currency: currency, SourceType: "banktransaction", SourceID: bt.BankTransactionID,
					})
				}
			}

			if len(entries) >= 2 {
				h.engine.Journal().Append(journal.Transaction{Entries: entries})
			}
		}

		h.store.BankTxns.Set(bt.BankTransactionID, bt)
		result = append(result, bt)

		if h.dispatcher != nil {
			h.dispatcher.Enqueue("BANKTRANSACTION.CREATE", map[string]any{
				"resourceId": bt.BankTransactionID,
				"eventType":  "BANKTRANSACTION.CREATE",
			})
		}
	}

	xeroJSON(w, http.StatusOK, map[string]any{"BankTransactions": result})
}

func (h *Handler) ListBankTransactions(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	all := h.store.BankTxns.List()
	xeroJSON(w, http.StatusOK, map[string]any{"BankTransactions": paginate(all, page, pageSize)})
}

func (h *Handler) GetBankTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "BankTransactionID")
	bt, ok := h.store.BankTxns.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "BankTransaction not found")
		return
	}
	xeroJSON(w, http.StatusOK, map[string]any{"BankTransactions": []store.BankTransaction{bt}})
}

func (h *Handler) DeleteBankTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "BankTransactionID")
	bt, ok := h.store.BankTxns.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "BankTransaction not found")
		return
	}
	bt.Status = "DELETED"
	bt.UpdatedDateUTC = store.XeroDateNow()
	h.store.BankTxns.Set(id, bt)

	// Reverse journal entries.
	if bt.Total > 0 {
		bankAcctID := bt.BankAccount.AccountID
		if bankAcctID == "" {
			bankAcctID = bt.BankAccount.Code
		}
		amount := int64(bt.Total * 100)
		currency := bt.CurrencyCode
		if currency == "" {
			currency = "USD"
		}

		var entries []journal.Entry
		if bt.Type == "RECEIVE" {
			// Reverse: credit bank, debit line accounts.
			entries = append(entries, journal.Entry{
				AccountID: bankAcctID, Type: journal.Credit, Amount: amount,
				Currency: currency, SourceType: "banktransaction_reversal", SourceID: id,
			})
			for _, li := range bt.LineItems {
				if li.AccountCode != "" && li.LineAmount > 0 {
					acctID := h.resolveAccountCode(li.AccountCode)
					entries = append(entries, journal.Entry{
						AccountID: acctID, Type: journal.Debit, Amount: int64(li.LineAmount * 100),
						Currency: currency, SourceType: "banktransaction_reversal", SourceID: id,
					})
				}
			}
		} else if bt.Type == "SPEND" {
			for _, li := range bt.LineItems {
				if li.AccountCode != "" && li.LineAmount > 0 {
					acctID := h.resolveAccountCode(li.AccountCode)
					entries = append(entries, journal.Entry{
						AccountID: acctID, Type: journal.Credit, Amount: int64(li.LineAmount * 100),
						Currency: currency, SourceType: "banktransaction_reversal", SourceID: id,
					})
				}
			}
			entries = append(entries, journal.Entry{
				AccountID: bankAcctID, Type: journal.Debit, Amount: amount,
				Currency: currency, SourceType: "banktransaction_reversal", SourceID: id,
			})
		}
		if len(entries) >= 2 {
			h.engine.Journal().Append(journal.Transaction{Entries: entries})
		}
	}

	xeroJSON(w, http.StatusOK, map[string]any{"BankTransactions": []store.BankTransaction{bt}})
}
