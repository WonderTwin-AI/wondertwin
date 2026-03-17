package accounting

import "fmt"

// validTransitions defines the document state machine.
var validTransitions = map[DocumentStatus][]DocumentStatus{
	StatusDraft:      {StatusSubmitted, StatusDeleted},
	StatusSubmitted:  {StatusAuthorised, StatusDeleted},
	StatusAuthorised: {StatusVoided}, // PAID only via ApplyPayment
	StatusPaid:       {},             // terminal
	StatusVoided:     {},             // terminal
	StatusDeleted:    {},             // terminal
}

// ValidateTransition checks if a state transition is allowed.
func ValidateTransition(from, to DocumentStatus) error {
	allowed, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("unknown status %q", from)
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("transition from %s to %s is not allowed", from, to)
}

// ComputeLineItemTotals calculates line item amounts and document totals.
// LineItem.Amount = Quantity * UnitAmount / 100 (quantity is scaled by 100).
func ComputeLineItemTotals(doc *Document) {
	var subTotal int64
	for i := range doc.LineItems {
		li := &doc.LineItems[i]
		li.Amount = li.Quantity * li.UnitAmount / 100
		subTotal += li.Amount
	}
	doc.SubTotal = subTotal
	doc.Total = subTotal // no tax in v0.1
	doc.AmountDue = doc.Total - doc.AmountPaid
}
