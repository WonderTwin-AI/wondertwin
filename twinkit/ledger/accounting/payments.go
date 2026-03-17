package accounting

import (
	"context"
	"fmt"

	"github.com/wondertwin-ai/wondertwin/twinkit/store/journal"
)

// applyPayment handles the payment application logic including journal entry generation.
// It validates the payment, creates balanced journal entries, updates the document,
// and transitions to PAID when fully paid.
func (e *Engine) applyPayment(ctx context.Context, pmt *Payment, doc *Document) error {
	if doc.Status != StatusAuthorised && doc.Status != StatusPaid {
		// Allow partial payments on already-PAID docs? No — once PAID, no more payments.
		if doc.Status == StatusPaid {
			return fmt.Errorf("document %s is already fully paid", doc.ID)
		}
		return fmt.Errorf("document %s has status %s, must be AUTHORISED to accept payment", doc.ID, doc.Status)
	}

	if pmt.Amount <= 0 {
		return fmt.Errorf("payment amount must be positive")
	}

	if pmt.Amount > doc.AmountDue {
		return fmt.Errorf("payment amount %d exceeds amount due %d", pmt.Amount, doc.AmountDue)
	}

	if pmt.Currency != doc.Currency {
		return fmt.Errorf("payment currency %s does not match document currency %s", pmt.Currency, doc.Currency)
	}

	// Generate journal entries based on document type.
	var entries []journal.Entry
	switch doc.Type {
	case DocTypeInvoice:
		// Invoice payment: Debit Bank, Credit Accounts Receivable
		entries = []journal.Entry{
			{AccountID: pmt.BankAccountID, Type: journal.Debit, Amount: pmt.Amount, Currency: pmt.Currency, SourceType: "payment", SourceID: pmt.ID},
			{AccountID: e.receivableAccountID(), Type: journal.Credit, Amount: pmt.Amount, Currency: pmt.Currency, SourceType: "payment", SourceID: pmt.ID},
		}
	case DocTypeBill:
		// Bill payment: Debit Accounts Payable, Credit Bank
		entries = []journal.Entry{
			{AccountID: e.payableAccountID(), Type: journal.Debit, Amount: pmt.Amount, Currency: pmt.Currency, SourceType: "payment", SourceID: pmt.ID},
			{AccountID: pmt.BankAccountID, Type: journal.Credit, Amount: pmt.Amount, Currency: pmt.Currency, SourceType: "payment", SourceID: pmt.ID},
		}
	case DocTypeCreditNote:
		// Credit note payment: Debit Accounts Receivable, Credit Bank (refund)
		entries = []journal.Entry{
			{AccountID: e.receivableAccountID(), Type: journal.Debit, Amount: pmt.Amount, Currency: pmt.Currency, SourceType: "payment", SourceID: pmt.ID},
			{AccountID: pmt.BankAccountID, Type: journal.Credit, Amount: pmt.Amount, Currency: pmt.Currency, SourceType: "payment", SourceID: pmt.ID},
		}
	default:
		return fmt.Errorf("unsupported document type %s for payment", doc.Type)
	}

	if err := e.journal.Append(journal.Transaction{Entries: entries}); err != nil {
		return fmt.Errorf("journal append for payment: %w", err)
	}

	// Update document amounts.
	doc.AmountPaid += pmt.Amount
	doc.AmountDue = doc.Total - doc.AmountPaid
	doc.UpdatedAt = e.clock.Now()

	// Notify hooks about journal entries.
	if e.hooks != nil {
		infos := make([]JournalEntryInfo, len(entries))
		for i, ent := range entries {
			infos[i] = JournalEntryInfo{
				AccountID:  ent.AccountID,
				Type:       string(ent.Type),
				Amount:     ent.Amount,
				SourceType: ent.SourceType,
				SourceID:   ent.SourceID,
			}
		}
		if err := e.hooks.OnJournalEntryCreated(ctx, doc, infos); err != nil {
			return fmt.Errorf("hook OnJournalEntryCreated: %w", err)
		}
	}

	// Auto-transition to PAID when fully paid.
	if doc.AmountDue == 0 {
		from := doc.Status
		doc.Status = StatusPaid
		if e.hooks != nil {
			if err := e.hooks.OnDocumentStateTransition(ctx, doc, from, StatusPaid); err != nil {
				return fmt.Errorf("hook OnDocumentStateTransition: %w", err)
			}
		}
	}

	// Notify hooks about payment.
	if e.hooks != nil {
		if err := e.hooks.OnPaymentApplied(ctx, pmt, doc); err != nil {
			return fmt.Errorf("hook OnPaymentApplied: %w", err)
		}
	}

	return nil
}
