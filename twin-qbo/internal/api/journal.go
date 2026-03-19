package api

import (
	"github.com/wondertwin-ai/wondertwin/twinkit/state/journal"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

// Well-known account IDs. These are set when accounts are created via the API.
// If an account with the matching AccountType exists, its ID is used.
// Otherwise these fallback IDs are used (the engine will accept them
// even without a registered account — journal entries are ID-based).
const (
	fallbackAR          = "accounts_receivable"
	fallbackAP          = "accounts_payable"
	fallbackUndeposited = "undeposited_funds"
)

// resolveAR returns the Accounts Receivable account ID.
func (h *Handler) resolveAR() string {
	items := h.store.Accounts.Filter(func(_ string, a store.Account) bool {
		return a.AccountType == "Accounts Receivable" && a.Active
	})
	if len(items) > 0 {
		return items[0].Id
	}
	return fallbackAR
}

// resolveAP returns the Accounts Payable account ID.
func (h *Handler) resolveAP() string {
	items := h.store.Accounts.Filter(func(_ string, a store.Account) bool {
		return a.AccountType == "Accounts Payable" && a.Active
	})
	if len(items) > 0 {
		return items[0].Id
	}
	return fallbackAP
}

// resolveUndeposited returns the Undeposited Funds account ID.
func (h *Handler) resolveUndeposited() string {
	items := h.store.Accounts.Filter(func(_ string, a store.Account) bool {
		return a.AccountType == "Other Current Asset" && a.Name == "Undeposited Funds"
	})
	if len(items) > 0 {
		return items[0].Id
	}
	return fallbackUndeposited
}

// journalInvoiceCreated creates journal entries for a new invoice.
// Debit: Accounts Receivable, Credit: Income account(s) from line items.
func (h *Handler) journalInvoiceCreated(inv *store.Invoice) {
	if inv.TotalAmt <= 0 {
		return
	}
	amount := int64(inv.TotalAmt * 100)
	if inv.ExchangeRate > 0 {
		amount = int64(inv.HomeTotalAmt * 100)
	}
	currency := "USD"
	if inv.CurrencyRef != nil {
		currency = inv.CurrencyRef.Value
	}

	var entries []journal.Entry
	entries = append(entries, journal.Entry{
		AccountID: h.resolveAR(), Type: journal.Debit, Amount: amount,
		Currency: currency, SourceType: "invoice", SourceID: inv.Id,
	})

	// Credit income accounts from line items.
	credited := h.creditLineItems(inv.Line, currency, "invoice", inv.Id)
	if len(credited) > 0 {
		entries = append(entries, credited...)
	} else {
		// No line-level accounts — credit a single income entry.
		entries = append(entries, journal.Entry{
			AccountID: "income", Type: journal.Credit, Amount: amount,
			Currency: currency, SourceType: "invoice", SourceID: inv.Id,
		})
	}

	h.engine.Journal().Append(journal.Transaction{Entries: entries})
	h.updateAccountBalances(entries)
}

// journalBillCreated creates journal entries for a new bill.
// Debit: Expense account(s), Credit: Accounts Payable.
func (h *Handler) journalBillCreated(bill *store.Bill) {
	if bill.TotalAmt <= 0 {
		return
	}
	amount := int64(bill.TotalAmt * 100)
	if bill.ExchangeRate > 0 {
		amount = int64(bill.HomeTotalAmt * 100)
	}
	currency := "USD"
	if bill.CurrencyRef != nil {
		currency = bill.CurrencyRef.Value
	}

	var entries []journal.Entry
	// Debit expense accounts from line items.
	debited := h.debitLineItems(bill.Line, currency, "bill", bill.Id)
	if len(debited) > 0 {
		entries = append(entries, debited...)
	} else {
		entries = append(entries, journal.Entry{
			AccountID: "expense", Type: journal.Debit, Amount: amount,
			Currency: currency, SourceType: "bill", SourceID: bill.Id,
		})
	}

	entries = append(entries, journal.Entry{
		AccountID: h.resolveAP(), Type: journal.Credit, Amount: amount,
		Currency: currency, SourceType: "bill", SourceID: bill.Id,
	})

	h.engine.Journal().Append(journal.Transaction{Entries: entries})
	h.updateAccountBalances(entries)
}

// journalPaymentCreated creates journal entries for a customer payment.
// Debit: Bank/Undeposited Funds, Credit: Accounts Receivable.
func (h *Handler) journalPaymentCreated(pmt *store.Payment) {
	if pmt.TotalAmt <= 0 {
		return
	}
	amount := int64(pmt.TotalAmt * 100)
	currency := "USD"

	depositTo := h.resolveUndeposited()
	if pmt.DepositToAccountRef != nil {
		depositTo = pmt.DepositToAccountRef.Value
	}

	entries := []journal.Entry{
		{AccountID: depositTo, Type: journal.Debit, Amount: amount,
			Currency: currency, SourceType: "payment", SourceID: pmt.Id},
		{AccountID: h.resolveAR(), Type: journal.Credit, Amount: amount,
			Currency: currency, SourceType: "payment", SourceID: pmt.Id},
	}
	h.engine.Journal().Append(journal.Transaction{Entries: entries})
	h.updateAccountBalances(entries)
}

// journalBillPaymentCreated creates journal entries for a vendor bill payment.
// Debit: Accounts Payable, Credit: Bank account.
func (h *Handler) journalBillPaymentCreated(bp *store.BillPayment) {
	if bp.TotalAmt <= 0 {
		return
	}
	amount := int64(bp.TotalAmt * 100)
	currency := "USD"

	bankAcct := "bank"
	if bp.CheckPayment != nil && bp.CheckPayment.BankAccountRef != nil {
		bankAcct = bp.CheckPayment.BankAccountRef.Value
	}

	entries := []journal.Entry{
		{AccountID: h.resolveAP(), Type: journal.Debit, Amount: amount,
			Currency: currency, SourceType: "billpayment", SourceID: bp.Id},
		{AccountID: bankAcct, Type: journal.Credit, Amount: amount,
			Currency: currency, SourceType: "billpayment", SourceID: bp.Id},
	}
	h.engine.Journal().Append(journal.Transaction{Entries: entries})
	h.updateAccountBalances(entries)
}

// journalSalesReceiptCreated creates entries for an immediate-payment sale.
// Debit: Undeposited Funds/Bank, Credit: Income.
func (h *Handler) journalSalesReceiptCreated(sr *store.SalesReceipt) {
	if sr.TotalAmt <= 0 {
		return
	}
	amount := int64(sr.TotalAmt * 100)
	currency := "USD"
	depositTo := h.resolveUndeposited()
	if sr.DepositToAccountRef != nil {
		depositTo = sr.DepositToAccountRef.Value
	}

	var entries []journal.Entry
	entries = append(entries, journal.Entry{
		AccountID: depositTo, Type: journal.Debit, Amount: amount,
		Currency: currency, SourceType: "salesreceipt", SourceID: sr.Id,
	})
	credited := h.creditLineItems(sr.Line, currency, "salesreceipt", sr.Id)
	if len(credited) > 0 {
		entries = append(entries, credited...)
	} else {
		entries = append(entries, journal.Entry{
			AccountID: "income", Type: journal.Credit, Amount: amount,
			Currency: currency, SourceType: "salesreceipt", SourceID: sr.Id,
		})
	}
	h.engine.Journal().Append(journal.Transaction{Entries: entries})
	h.updateAccountBalances(entries)
}

// journalDepositCreated creates entries for a bank deposit.
// Debit: Bank, Credit: Undeposited Funds.
func (h *Handler) journalDepositCreated(dep *store.Deposit) {
	if dep.TotalAmt <= 0 {
		return
	}
	amount := int64(dep.TotalAmt * 100)
	currency := "USD"
	bankAcct := "bank"
	if dep.DepositToAccountRef != nil {
		bankAcct = dep.DepositToAccountRef.Value
	}

	entries := []journal.Entry{
		{AccountID: bankAcct, Type: journal.Debit, Amount: amount,
			Currency: currency, SourceType: "deposit", SourceID: dep.Id},
		{AccountID: h.resolveUndeposited(), Type: journal.Credit, Amount: amount,
			Currency: currency, SourceType: "deposit", SourceID: dep.Id},
	}
	h.engine.Journal().Append(journal.Transaction{Entries: entries})
	h.updateAccountBalances(entries)
}

// journalTransferCreated creates entries for an inter-account transfer.
// Debit: ToAccount, Credit: FromAccount.
func (h *Handler) journalTransferCreated(xfer *store.Transfer) {
	if xfer.Amount <= 0 {
		return
	}
	amount := int64(xfer.Amount * 100)
	currency := "USD"

	from := "bank_from"
	to := "bank_to"
	if xfer.FromAccountRef != nil {
		from = xfer.FromAccountRef.Value
	}
	if xfer.ToAccountRef != nil {
		to = xfer.ToAccountRef.Value
	}

	entries := []journal.Entry{
		{AccountID: to, Type: journal.Debit, Amount: amount,
			Currency: currency, SourceType: "transfer", SourceID: xfer.Id},
		{AccountID: from, Type: journal.Credit, Amount: amount,
			Currency: currency, SourceType: "transfer", SourceID: xfer.Id},
	}
	h.engine.Journal().Append(journal.Transaction{Entries: entries})
	h.updateAccountBalances(entries)
}

// journalJournalEntryCreated creates entries from a manual journal entry.
func (h *Handler) journalJournalEntryCreated(je *store.JournalEntry) {
	var entries []journal.Entry
	currency := "USD"
	for _, line := range je.Line {
		if line.JournalEntryLineDetail == nil {
			continue
		}
		acctID := "unknown"
		if line.JournalEntryLineDetail.AccountRef != nil {
			acctID = line.JournalEntryLineDetail.AccountRef.Value
		}
		amount := int64(line.Amount * 100)
		if amount <= 0 {
			continue
		}
		entryType := journal.Debit
		if line.JournalEntryLineDetail.PostingType == "Credit" {
			entryType = journal.Credit
		}
		entries = append(entries, journal.Entry{
			AccountID: acctID, Type: entryType, Amount: amount,
			Currency: currency, SourceType: "journalentry", SourceID: je.Id,
		})
	}
	if len(entries) >= 2 {
		h.engine.Journal().Append(journal.Transaction{Entries: entries})
		h.updateAccountBalances(entries)
	}
}

// journalPurchaseCreated creates entries for an expense purchase.
// Debit: Expense account(s), Credit: Payment account (bank/credit card).
func (h *Handler) journalPurchaseCreated(pur *store.Purchase) {
	if pur.TotalAmt <= 0 {
		return
	}
	amount := int64(pur.TotalAmt * 100)
	currency := "USD"
	payAcct := "bank"
	if pur.AccountRef != nil {
		payAcct = pur.AccountRef.Value
	}

	var entries []journal.Entry
	debited := h.debitLineItems(pur.Line, currency, "purchase", pur.Id)
	if len(debited) > 0 {
		entries = append(entries, debited...)
	} else {
		entries = append(entries, journal.Entry{
			AccountID: "expense", Type: journal.Debit, Amount: amount,
			Currency: currency, SourceType: "purchase", SourceID: pur.Id,
		})
	}
	entries = append(entries, journal.Entry{
		AccountID: payAcct, Type: journal.Credit, Amount: amount,
		Currency: currency, SourceType: "purchase", SourceID: pur.Id,
	})
	h.engine.Journal().Append(journal.Transaction{Entries: entries})
	h.updateAccountBalances(entries)
}

// journalInvoiceVoided reverses the AR/Income entries created when the invoice was posted.
// Must be called BEFORE zeroing amounts on the invoice.
func (h *Handler) journalInvoiceVoided(inv *store.Invoice) {
	if inv.TotalAmt <= 0 {
		return
	}
	amount := int64(inv.TotalAmt * 100)
	currency := "USD"
	if inv.CurrencyRef != nil {
		currency = inv.CurrencyRef.Value
	}

	var entries []journal.Entry
	// Reverse: Credit AR (was Debit).
	entries = append(entries, journal.Entry{
		AccountID: h.resolveAR(), Type: journal.Credit, Amount: amount,
		Currency: currency, SourceType: "invoice", SourceID: inv.Id,
	})
	// Reverse: Debit income accounts (were Credit).
	debited := h.debitCreditLineItems(inv.Line, currency, "invoice", inv.Id)
	if len(debited) > 0 {
		entries = append(entries, debited...)
	} else {
		entries = append(entries, journal.Entry{
			AccountID: "income", Type: journal.Debit, Amount: amount,
			Currency: currency, SourceType: "invoice", SourceID: inv.Id,
		})
	}
	h.engine.Journal().Append(journal.Transaction{Entries: entries})
	h.updateAccountBalances(entries)
}

// journalBillVoided reverses the Expense/AP entries created when the bill was posted.
// Must be called BEFORE zeroing amounts on the bill.
func (h *Handler) journalBillVoided(bill *store.Bill) {
	if bill.TotalAmt <= 0 {
		return
	}
	amount := int64(bill.TotalAmt * 100)
	currency := "USD"

	var entries []journal.Entry
	// Reverse: Credit expense accounts (were Debit).
	credited := h.creditDebitLineItems(bill.Line, currency, "bill", bill.Id)
	if len(credited) > 0 {
		entries = append(entries, credited...)
	} else {
		entries = append(entries, journal.Entry{
			AccountID: "expense", Type: journal.Credit, Amount: amount,
			Currency: currency, SourceType: "bill", SourceID: bill.Id,
		})
	}
	// Reverse: Debit AP (was Credit).
	entries = append(entries, journal.Entry{
		AccountID: h.resolveAP(), Type: journal.Debit, Amount: amount,
		Currency: currency, SourceType: "bill", SourceID: bill.Id,
	})
	h.engine.Journal().Append(journal.Transaction{Entries: entries})
	h.updateAccountBalances(entries)
}

// journalPaymentVoided reverses the Bank/AR entries created when the payment was posted.
func (h *Handler) journalPaymentVoided(pmt *store.Payment) {
	if pmt.TotalAmt <= 0 {
		return
	}
	amount := int64(pmt.TotalAmt * 100)
	currency := "USD"

	depositTo := h.resolveUndeposited()
	if pmt.DepositToAccountRef != nil {
		depositTo = pmt.DepositToAccountRef.Value
	}

	// Reverse: Credit Bank/Undeposited (was Debit), Debit AR (was Credit).
	entries := []journal.Entry{
		{AccountID: depositTo, Type: journal.Credit, Amount: amount,
			Currency: currency, SourceType: "payment", SourceID: pmt.Id},
		{AccountID: h.resolveAR(), Type: journal.Debit, Amount: amount,
			Currency: currency, SourceType: "payment", SourceID: pmt.Id},
	}
	h.engine.Journal().Append(journal.Transaction{Entries: entries})
	h.updateAccountBalances(entries)
}

// journalBillPaymentVoided reverses the AP/Bank entries created when the bill payment was posted.
func (h *Handler) journalBillPaymentVoided(bp *store.BillPayment) {
	if bp.TotalAmt <= 0 {
		return
	}
	amount := int64(bp.TotalAmt * 100)
	currency := "USD"

	bankAcct := "bank"
	if bp.CheckPayment != nil && bp.CheckPayment.BankAccountRef != nil {
		bankAcct = bp.CheckPayment.BankAccountRef.Value
	}

	// Reverse: Credit AP (was Debit), Debit Bank (was Credit).
	entries := []journal.Entry{
		{AccountID: h.resolveAP(), Type: journal.Credit, Amount: amount,
			Currency: currency, SourceType: "billpayment", SourceID: bp.Id},
		{AccountID: bankAcct, Type: journal.Debit, Amount: amount,
			Currency: currency, SourceType: "billpayment", SourceID: bp.Id},
	}
	h.engine.Journal().Append(journal.Transaction{Entries: entries})
	h.updateAccountBalances(entries)
}

// journalSalesReceiptVoided reverses the Undeposited/Income entries created when the sales receipt was posted.
// Must be called BEFORE zeroing amounts on the sales receipt.
func (h *Handler) journalSalesReceiptVoided(sr *store.SalesReceipt) {
	if sr.TotalAmt <= 0 {
		return
	}
	amount := int64(sr.TotalAmt * 100)
	currency := "USD"
	depositTo := h.resolveUndeposited()
	if sr.DepositToAccountRef != nil {
		depositTo = sr.DepositToAccountRef.Value
	}

	var entries []journal.Entry
	// Reverse: Credit Undeposited/Bank (was Debit).
	entries = append(entries, journal.Entry{
		AccountID: depositTo, Type: journal.Credit, Amount: amount,
		Currency: currency, SourceType: "salesreceipt", SourceID: sr.Id,
	})
	// Reverse: Debit income accounts (were Credit).
	debited := h.debitCreditLineItems(sr.Line, currency, "salesreceipt", sr.Id)
	if len(debited) > 0 {
		entries = append(entries, debited...)
	} else {
		entries = append(entries, journal.Entry{
			AccountID: "income", Type: journal.Debit, Amount: amount,
			Currency: currency, SourceType: "salesreceipt", SourceID: sr.Id,
		})
	}
	h.engine.Journal().Append(journal.Transaction{Entries: entries})
	h.updateAccountBalances(entries)
}

// updateAccountBalances adjusts Account.CurrentBalance for each entry.
// Debits increase asset/expense accounts, decrease liability/equity/revenue.
// Credits decrease asset/expense accounts, increase liability/equity/revenue.
func (h *Handler) updateAccountBalances(entries []journal.Entry) {
	for _, e := range entries {
		acct, ok := h.store.Accounts.Get(e.AccountID)
		if !ok {
			continue
		}
		amt := float64(e.Amount) / 100.0
		switch acct.Classification {
		case "Asset", "Expense":
			if e.Type == journal.Debit {
				acct.CurrentBalance += amt
			} else {
				acct.CurrentBalance -= amt
			}
		case "Liability", "Equity", "Revenue":
			if e.Type == journal.Credit {
				acct.CurrentBalance += amt
			} else {
				acct.CurrentBalance -= amt
			}
		default:
			if e.Type == journal.Debit {
				acct.CurrentBalance += amt
			} else {
				acct.CurrentBalance -= amt
			}
		}
		h.store.Accounts.Set(acct.Id, acct)
	}
}

// creditLineItems creates Credit entries from sales line items that have account references.
func (h *Handler) creditLineItems(lines []store.Line, currency, sourceType, sourceID string) []journal.Entry {
	var entries []journal.Entry
	for _, line := range lines {
		if line.Amount <= 0 {
			continue
		}
		acctID := ""
		if line.SalesItemLineDetail != nil && line.SalesItemLineDetail.ItemRef != nil {
			// Resolve item's income account.
			if item, ok := h.store.Items.Get(line.SalesItemLineDetail.ItemRef.Value); ok && item.IncomeAccountRef != nil {
				acctID = item.IncomeAccountRef.Value
			}
		}
		if acctID == "" {
			continue
		}
		entries = append(entries, journal.Entry{
			AccountID: acctID, Type: journal.Credit, Amount: int64(line.Amount * 100),
			Currency: currency, SourceType: sourceType, SourceID: sourceID,
		})
	}
	return entries
}

// debitCreditLineItems creates Debit entries from sales line items (reversing credits).
func (h *Handler) debitCreditLineItems(lines []store.Line, currency, sourceType, sourceID string) []journal.Entry {
	var entries []journal.Entry
	for _, line := range lines {
		if line.Amount <= 0 {
			continue
		}
		acctID := ""
		if line.SalesItemLineDetail != nil && line.SalesItemLineDetail.ItemRef != nil {
			if item, ok := h.store.Items.Get(line.SalesItemLineDetail.ItemRef.Value); ok && item.IncomeAccountRef != nil {
				acctID = item.IncomeAccountRef.Value
			}
		}
		if acctID == "" {
			continue
		}
		entries = append(entries, journal.Entry{
			AccountID: acctID, Type: journal.Debit, Amount: int64(line.Amount * 100),
			Currency: currency, SourceType: sourceType, SourceID: sourceID,
		})
	}
	return entries
}

// creditDebitLineItems creates Credit entries from expense line items (reversing debits).
func (h *Handler) creditDebitLineItems(lines []store.Line, currency, sourceType, sourceID string) []journal.Entry {
	var entries []journal.Entry
	for _, line := range lines {
		if line.Amount <= 0 {
			continue
		}
		acctID := ""
		if line.AccountBasedExpenseLineDetail != nil && line.AccountBasedExpenseLineDetail.AccountRef != nil {
			acctID = line.AccountBasedExpenseLineDetail.AccountRef.Value
		}
		if acctID == "" {
			continue
		}
		entries = append(entries, journal.Entry{
			AccountID: acctID, Type: journal.Credit, Amount: int64(line.Amount * 100),
			Currency: currency, SourceType: sourceType, SourceID: sourceID,
		})
	}
	return entries
}

// journalCreditMemoCreated creates entries for a new credit memo.
// Debit: Income, Credit: Accounts Receivable.
func (h *Handler) journalCreditMemoCreated(cm *store.CreditMemo) {
	if cm.TotalAmt <= 0 {
		return
	}
	amtCents := int64(cm.TotalAmt * 100)
	currency := "USD"
	entries := []journal.Entry{
		{AccountID: "income", Type: journal.Debit, Amount: amtCents,
			Currency: currency, SourceType: "creditmemo", SourceID: cm.Id},
		{AccountID: h.resolveAR(), Type: journal.Credit, Amount: amtCents,
			Currency: currency, SourceType: "creditmemo", SourceID: cm.Id},
	}
	h.engine.Journal().Append(journal.Transaction{Entries: entries})
	h.updateAccountBalances(entries)
}

// journalCreditMemoApplied creates reversing entries when a credit memo is applied to an invoice.
// Debit: Income (reduces revenue), Credit: Accounts Receivable (reduces AR).
func (h *Handler) journalCreditMemoApplied(creditMemoID string, amount float64) {
	if amount <= 0 {
		return
	}
	amtCents := int64(amount * 100)
	currency := "USD"
	entries := []journal.Entry{
		{AccountID: h.resolveAR(), Type: journal.Credit, Amount: amtCents,
			Currency: currency, SourceType: "creditmemo", SourceID: creditMemoID},
		{AccountID: "income", Type: journal.Debit, Amount: amtCents,
			Currency: currency, SourceType: "creditmemo", SourceID: creditMemoID},
	}
	h.engine.Journal().Append(journal.Transaction{Entries: entries})
	h.updateAccountBalances(entries)
}

// debitLineItems creates Debit entries from expense line items.
func (h *Handler) debitLineItems(lines []store.Line, currency, sourceType, sourceID string) []journal.Entry {
	var entries []journal.Entry
	for _, line := range lines {
		if line.Amount <= 0 {
			continue
		}
		acctID := ""
		if line.AccountBasedExpenseLineDetail != nil && line.AccountBasedExpenseLineDetail.AccountRef != nil {
			acctID = line.AccountBasedExpenseLineDetail.AccountRef.Value
		}
		if acctID == "" {
			continue
		}
		entries = append(entries, journal.Entry{
			AccountID: acctID, Type: journal.Debit, Amount: int64(line.Amount * 100),
			Currency: currency, SourceType: sourceType, SourceID: sourceID,
		})
	}
	return entries
}
