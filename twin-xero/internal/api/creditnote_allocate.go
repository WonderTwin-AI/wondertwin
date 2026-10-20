package api

import (
	"fmt"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// creditType is the invoice type a credit note type can be applied to.
var creditType = map[string]string{"ACCRECCREDIT": "ACCREC", "ACCPAYCREDIT": "ACCPAY"}

// planAllocations checks a new credit note's inline allocations against the
// invoices they name and returns the reasons any is refused. due tracks each
// invoice's AmountDue as earlier allocations in the same request reduce it, so
// nothing is applied until every credit note in the request has been checked.
// It matters only under StrictAuth.
// The rules are guesses. The research has one example (allocate part of an
// AUTHORISED credit note to an outstanding invoice), and localstack/finance-ops
// allocates the whole refund to an invoice that is normally already PAID, so by
// default an AUTHORISED or PAID invoice is accepted and the amount is bounded
// only by the credit remaining. Under StrictAuth the invoice must be AUTHORISED
// and the amount within its AmountDue. Either way the invoice exists with the
// matching type and the same contact, the amount is above zero, and a date is
// given.
func (h *Handler) planAllocations(tenant string, cn store.CreditNote, due map[string]float64) []string {
	if len(cn.Allocations) == 0 {
		return nil
	}
	if cn.Status != "AUTHORISED" {
		return []string{"Allocations can only be applied to an authorised credit note."}
	}
	var msgs []string
	strict := h.store.StrictAuth.Load()
	remaining := cn.Total
	invoices := h.store.Invoices.For(tenant)
	for i, a := range cn.Allocations {
		inv, ok := store.Invoice{}, false
		if a.Invoice != nil {
			inv, ok = invoices.Get(a.Invoice.InvoiceID)
		}
		if _, seen := due[inv.InvoiceID]; ok && !seen {
			due[inv.InvoiceID] = inv.AmountDue
		}
		n := i + 1
		switch {
		case !ok:
			msgs = append(msgs, fmt.Sprintf("Allocation %d: the invoice could not be found.", n))
			continue
		case inv.Type != creditType[cn.Type] || inv.Contact == nil || cn.Contact == nil || inv.Contact.ContactID != cn.Contact.ContactID:
			msgs = append(msgs, fmt.Sprintf("Allocation %d: the invoice must be a %s for the same contact.", n, creditType[cn.Type]))
		case inv.Status != "AUTHORISED" && (strict || inv.Status != "PAID"):
			msgs = append(msgs, fmt.Sprintf("Allocation %d: credit can only be applied to an authorised invoice.", n))
		}
		if a.Date == nil {
			msgs = append(msgs, fmt.Sprintf("Allocation %d: a Date is required.", n))
		}
		switch {
		case a.Amount <= 0:
			msgs = append(msgs, fmt.Sprintf("Allocation %d: the Amount must be above zero.", n))
		case a.Amount > remaining+0.005:
			msgs = append(msgs, fmt.Sprintf("Allocation %d: the Amount is more than the credit remaining.", n))
		case strict && a.Amount > due[inv.InvoiceID]+0.005:
			msgs = append(msgs, fmt.Sprintf("Allocation %d: the Amount is more than the invoice's amount due.", n))
		default:
			remaining, due[inv.InvoiceID] = remaining-a.Amount, due[inv.InvoiceID]-a.Amount
		}
	}
	return msgs
}

// applyAllocations records a checked credit note's allocations on the invoices
// they name: each takes the amount off AmountDue (never below zero, so a PAID
// invoice stays at zero) and onto AmountCredited, an invoice that reaches zero
// becomes PAID, and the credit note's AppliedAmount
// and RemainingCredit follow. A credit note with nothing left becomes PAID too;
// the research marks the credit note state machine as an inference.
func (h *Handler) applyAllocations(tenant string, cn *store.CreditNote) {
	if len(cn.Allocations) == 0 {
		return
	}
	invoices := h.store.Invoices.For(tenant)
	var applied float64
	for i := range cn.Allocations {
		a := &cn.Allocations[i]
		inv, _ := invoices.Get(a.Invoice.InvoiceID)
		a.AllocationID, a.IsDeleted, a.StatusAttributeString = h.store.NewID(), false, "OK"
		a.Invoice = &store.InvoiceRef{InvoiceID: inv.InvoiceID, InvoiceNumber: inv.InvoiceNumber}
		wasPaid := inv.Status == "PAID"
		inv.AmountCredited, inv.AmountDue = round(inv.AmountCredited+a.Amount, 2), max(round(inv.AmountDue-a.Amount, 2), 0)
		if inv.AmountDue == 0 && !wasPaid {
			inv.Status, inv.FullyPaidOnDate = "PAID", a.Date
		}
		inv.CreditNotes = append(inv.CreditNotes, map[string]any{
			"CreditNoteID": cn.CreditNoteID, "ID": cn.CreditNoteID, "CreditNoteNumber": cn.CreditNoteNumber,
			"AppliedAmount": a.Amount, "Date": Msdate(*a.Date), "HasErrors": false,
		})
		inv.UpdatedDateUTC, inv.UpdatedDateUTCString = cn.UpdatedDateUTC, cn.UpdatedDateUTCString
		invoices.Set(inv.InvoiceID, inv)
		applied += a.Amount
		if round(cn.Total-applied, 2) == 0 {
			cn.FullyPaidOnDate = a.Date
		}
	}
	cn.AppliedAmount, cn.RemainingCredit = round(applied, 2), round(cn.Total-applied, 2)
	if cn.RemainingCredit == 0 {
		cn.Status = "PAID"
	}
}
