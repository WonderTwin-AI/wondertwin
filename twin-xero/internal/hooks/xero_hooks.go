// Package hooks implements AccountingHooks for the Xero twin.
// It fires Xero-style webhook events on state transitions.
package hooks

import (
	"context"

	"github.com/wondertwin-ai/wondertwin/twinkit/ledger/accounting"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
)

// XeroHooks implements accounting.AccountingHooks for the Xero twin.
type XeroHooks struct {
	accounting.NoOpAccountingHooks
	Dispatcher *webhook.Dispatcher
}

func (h *XeroHooks) OnDocumentStateTransition(_ context.Context, doc *accounting.Document, from, to accounting.DocumentStatus) error {
	if h.Dispatcher == nil {
		return nil
	}

	var eventCategory string
	switch doc.Type {
	case accounting.DocTypeInvoice:
		eventCategory = "INVOICE"
	case accounting.DocTypeBill:
		eventCategory = "INVOICE" // Xero treats bills as invoices (Type=ACCPAY)
	case accounting.DocTypeCreditNote:
		eventCategory = "CREDITNOTE"
	default:
		return nil
	}

	eventType := eventCategory + ".UPDATE"
	if from == accounting.StatusDraft && (to == accounting.StatusSubmitted || to == accounting.StatusDraft) {
		eventType = eventCategory + ".CREATE"
	}

	h.Dispatcher.Enqueue(eventType, map[string]any{
		"resourceId":  doc.ID,
		"resourceUrl": "", // filled by handler if needed
		"eventType":   eventType,
	})
	return nil
}

func (h *XeroHooks) OnPaymentApplied(_ context.Context, pmt *accounting.Payment, _ *accounting.Document) error {
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
