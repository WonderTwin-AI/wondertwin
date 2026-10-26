package store

// The lists an invoice or a credit note carries of the records that touch it.
// Their fields are declared in name order, so the wire answer keeps the key order
// the map values they replace had. A date here is the /Date(ms+0000)/ string the
// API layer wrote, since the store knows no wire formats.

// PaymentSummary is a payment as an invoice or a credit note lists it.
type PaymentSummary struct {
	Amount              float64 `json:"Amount"`
	Date                string  `json:"Date"`
	HasAccount          bool    `json:"HasAccount"`
	HasValidationErrors bool    `json:"HasValidationErrors"`
	PaymentID           string  `json:"PaymentID"`
	Reference           string  `json:"Reference"`
}

// CreditNoteSummary is a credit note as the invoice it was applied to lists it.
type CreditNoteSummary struct {
	AppliedAmount    float64 `json:"AppliedAmount"`
	CreditNoteID     string  `json:"CreditNoteID"`
	CreditNoteNumber string  `json:"CreditNoteNumber"`
	Date             string  `json:"Date"`
	HasErrors        bool    `json:"HasErrors"`
	ID               string  `json:"ID"`
}

// PrepaymentSummary is a prepayment as an invoice lists it. Nothing writes one
// yet, so its fields beyond the identity are a guess.
type PrepaymentSummary struct {
	AppliedAmount float64 `json:"AppliedAmount"`
	Date          string  `json:"Date"`
	HasErrors     bool    `json:"HasErrors"`
	ID            string  `json:"ID"`
	PrepaymentID  string  `json:"PrepaymentID"`
}

// OverpaymentSummary is an overpayment as an invoice lists it. Nothing writes
// one yet, so its fields beyond the identity are a guess.
type OverpaymentSummary struct {
	AppliedAmount float64 `json:"AppliedAmount"`
	Date          string  `json:"Date"`
	HasErrors     bool    `json:"HasErrors"`
	ID            string  `json:"ID"`
	OverpaymentID string  `json:"OverpaymentID"`
}
