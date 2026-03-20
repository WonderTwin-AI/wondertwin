// Package hooks implements AccountingHooks for the Xero twin.
// It fires Xero-style webhook events on state transitions.
package hooks

import (
	"context"

	"github.com/wondertwin-ai/wondertwin/twinkit/ledger"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
)

// XeroHooks implements ledger.AccountingHooks for the Xero twin.
type XeroHooks struct {
	ledger.NoOpAccountingHooks
	Dispatcher *webhook.Dispatcher
}

func (h *XeroHooks) OnDocumentStateTransition(_ context.Context, doc *ledger.Document, from, to ledger.DocumentStatus) error {
	if h.Dispatcher == nil {
		return nil
	}

	var eventCategory string
	switch doc.Type {
	case ledger.DocTypeInvoice:
		eventCategory = "INVOICE"
	case ledger.DocTypeBill:
		eventCategory = "INVOICE" // Xero treats bills as invoices (Type=ACCPAY)
	case ledger.DocTypeCreditNote:
		eventCategory = "CREDITNOTE"
	default:
		return nil
	}

	eventType := eventCategory + ".UPDATE"
	if from == ledger.StatusDraft && (to == ledger.StatusSubmitted || to == ledger.StatusDraft) {
		eventType = eventCategory + ".CREATE"
	}

	h.Dispatcher.Enqueue(eventType, map[string]any{
		"resourceId":  doc.ID,
		"resourceUrl": "", // filled by handler if needed
		"eventType":   eventType,
	})
	return nil
}

func (h *XeroHooks) OnPaymentApplied(_ context.Context, pmt *ledger.Payment, _ *ledger.Document) error {
	if h.Dispatcher == nil {
		return nil
	}
	h.Dispatcher.Enqueue("PAYMENT.CREATE", map[string]any{
		"resourceId":  pmt.ID,
		"resourceUrl": "",
		"eventType":   "PAYMENT.CREATE",
	})
	return nil
}
