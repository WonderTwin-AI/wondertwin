package store

// LineItem is an invoice line. The amounts are pointers because the client may
// leave any of them out: Quantity, UnitAmount and LineAmount derive from each
// other, and a missing TaxAmount is computed.
type LineItem struct {
	LineItemID         string             `json:"LineItemID"`
	Description        string             `json:"Description,omitempty"`
	Quantity           *float64           `json:"Quantity,omitempty"`
	UnitAmount         *float64           `json:"UnitAmount,omitempty"`
	ItemCode           string             `json:"ItemCode,omitempty"`
	AccountCode        string             `json:"AccountCode,omitempty"`
	AccountID          string             `json:"AccountID,omitempty"`
	TaxType            string             `json:"TaxType,omitempty"`
	TaxAmount          *float64           `json:"TaxAmount,omitempty"`
	Item               *LineItemItem      `json:"Item,omitempty"`
	LineAmount         *float64           `json:"LineAmount,omitempty"`
	Tracking           []LineItemTracking `json:"Tracking"`
	DiscountRate       *float64           `json:"DiscountRate,omitempty"`
	DiscountAmount     *float64           `json:"DiscountAmount,omitempty"`
	RepeatingInvoiceID string             `json:"RepeatingInvoiceID,omitempty"`
	Taxability         string             `json:"Taxability,omitempty"`
	SalesTaxCodeID     *float64           `json:"SalesTaxCodeId,omitempty"`
	TaxBreakdown       []TaxBreakdownPart `json:"TaxBreakdown,omitempty"`
}

// LineItemItem is the inventory item a line points at.
type LineItemItem struct {
	Code   string `json:"Code,omitempty"`
	Name   string `json:"Name,omitempty"`
	ItemID string `json:"ItemID,omitempty"`
}

// LineItemTracking is one tracking category and option on a line.
type LineItemTracking struct {
	TrackingCategoryID string `json:"TrackingCategoryID,omitempty"`
	TrackingOptionID   string `json:"TrackingOptionID,omitempty"`
	Name               string `json:"Name,omitempty"`
	Option             string `json:"Option,omitempty"`
}

// TaxBreakdownPart is one component of a line's tax.
type TaxBreakdownPart struct {
	TaxComponentID     string   `json:"TaxComponentId,omitempty"`
	Type               string   `json:"Type,omitempty"`
	Name               string   `json:"Name,omitempty"`
	TaxPercentage      *float64 `json:"TaxPercentage,omitempty"`
	TaxAmount          *float64 `json:"TaxAmount,omitempty"`
	TaxableAmount      *float64 `json:"TaxableAmount,omitempty"`
	NonTaxableAmount   *float64 `json:"NonTaxableAmount,omitempty"`
	ExemptAmount       *float64 `json:"ExemptAmount,omitempty"`
	StateAssignedNo    string   `json:"StateAssignedNo,omitempty"`
	JurisdictionRegion string   `json:"JurisdictionRegion,omitempty"`
}
