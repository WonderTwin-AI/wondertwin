package store

import "time"

// ManualJournal is a journal posted straight to the ledger, as Xero returns it.
// Its dates are written as /Date(ms+0000)/ by the API layer, which shadows them.
// Xero's journal lines carry no ID of their own.
type ManualJournal struct {
	ManualJournalID        string            `json:"ManualJournalID"`
	Narration              string            `json:"Narration"`
	JournalLines           []JournalLine     `json:"JournalLines"`
	Date                   *time.Time        `json:"Date,omitempty"`
	LineAmountTypes        string            `json:"LineAmountTypes"`
	Status                 string            `json:"Status"`
	URL                    string            `json:"Url,omitempty"`
	ShowOnCashBasisReports bool              `json:"ShowOnCashBasisReports"`
	HasAttachments         bool              `json:"HasAttachments"`
	UpdatedDateUTC         time.Time         `json:"UpdatedDateUTC"`
	StatusAttributeString  string            `json:"StatusAttributeString,omitempty"`
	Warnings               []ValidationError `json:"Warnings,omitempty"`
	ValidationErrors       []ValidationError `json:"ValidationErrors,omitempty"`
}

// JournalLine is one line of a manual journal: a debit is a positive LineAmount
// and a credit a negative one, as the spec says.
type JournalLine struct {
	LineAmount  float64            `json:"LineAmount"`
	AccountCode string             `json:"AccountCode,omitempty"`
	AccountID   string             `json:"AccountID,omitempty"`
	Description string             `json:"Description,omitempty"`
	TaxType     string             `json:"TaxType,omitempty"`
	TaxAmount   *float64           `json:"TaxAmount,omitempty"`
	Tracking    []LineItemTracking `json:"Tracking"`
	IsBlank     bool               `json:"IsBlank"`
}
