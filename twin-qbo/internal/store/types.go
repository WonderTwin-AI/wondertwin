// Package store provides QBO-shaped entity types and the in-memory store.
// All types use QBO's PascalCase JSON field names and string integer IDs.
package store

// Ref is a QBO reference to another entity (foreign key).
type Ref struct {
	Value string `json:"value"`
	Name  string `json:"name,omitempty"`
}

// MetaData holds entity timestamps.
type MetaData struct {
	CreateTime      string `json:"CreateTime"`
	LastUpdatedTime string `json:"LastUpdatedTime"`
}

// Customer represents a QBO customer.
type Customer struct {
	Id               string   `json:"Id"`
	SyncToken        string   `json:"SyncToken"`
	DisplayName      string   `json:"DisplayName"`
	GivenName        string   `json:"GivenName,omitempty"`
	FamilyName       string   `json:"FamilyName,omitempty"`
	CompanyName      string   `json:"CompanyName,omitempty"`
	PrimaryEmailAddr *Email   `json:"PrimaryEmailAddr,omitempty"`
	Active           bool     `json:"Active"`
	Balance          float64  `json:"Balance"`
	ParentRef        *Ref     `json:"ParentRef,omitempty"`
	MetaData         MetaData `json:"MetaData"`
	Domain           string   `json:"domain"`
	Sparse           bool     `json:"sparse,omitempty"`
}

// Email wraps an email address.
type Email struct {
	Address string `json:"Address"`
}

// Vendor represents a QBO vendor.
type Vendor struct {
	Id               string   `json:"Id"`
	SyncToken        string   `json:"SyncToken"`
	DisplayName      string   `json:"DisplayName"`
	GivenName        string   `json:"GivenName,omitempty"`
	FamilyName       string   `json:"FamilyName,omitempty"`
	CompanyName      string   `json:"CompanyName,omitempty"`
	PrimaryEmailAddr *Email   `json:"PrimaryEmailAddr,omitempty"`
	Active           bool     `json:"Active"`
	Balance          float64  `json:"Balance"`
	MetaData         MetaData `json:"MetaData"`
	Domain           string   `json:"domain"`
}

// Account represents a QBO chart-of-accounts entry.
type Account struct {
	Id              string   `json:"Id"`
	SyncToken       string   `json:"SyncToken"`
	Name            string   `json:"Name"`
	AccountType     string   `json:"AccountType"`
	AccountSubType  string   `json:"AccountSubType,omitempty"`
	Classification  string   `json:"Classification"`
	AcctNum         string   `json:"AcctNum,omitempty"`
	CurrentBalance  float64  `json:"CurrentBalance"`
	Active          bool     `json:"Active"`
	CurrencyRef     *Ref     `json:"CurrencyRef,omitempty"`
	Description     string   `json:"Description,omitempty"`
	MetaData        MetaData `json:"MetaData"`
	Domain          string   `json:"domain"`
}

// Item represents a QBO product/service item.
type Item struct {
	Id               string   `json:"Id"`
	SyncToken        string   `json:"SyncToken"`
	Name             string   `json:"Name"`
	Type             string   `json:"Type"` // Inventory, NonInventory, Service, Bundle
	IncomeAccountRef *Ref     `json:"IncomeAccountRef,omitempty"`
	ExpenseAccountRef *Ref    `json:"ExpenseAccountRef,omitempty"`
	UnitPrice        float64  `json:"UnitPrice,omitempty"`
	Active           bool     `json:"Active"`
	MetaData         MetaData `json:"MetaData"`
	Domain           string   `json:"domain"`
}

// Line is a polymorphic line item on a transaction.
type Line struct {
	Id                          string                       `json:"Id,omitempty"`
	LineNum                     int                          `json:"LineNum,omitempty"`
	Amount                      float64                      `json:"Amount"`
	DetailType                  string                       `json:"DetailType"`
	SalesItemLineDetail         *SalesItemLineDetail         `json:"SalesItemLineDetail,omitempty"`
	AccountBasedExpenseLineDetail *AccountBasedExpenseLineDetail `json:"AccountBasedExpenseLineDetail,omitempty"`
	JournalEntryLineDetail      *JournalEntryLineDetail      `json:"JournalEntryLineDetail,omitempty"`
	LinkedTxn                   []LinkedTxn                  `json:"LinkedTxn,omitempty"`
}

// SalesItemLineDetail is the detail for sales lines.
type SalesItemLineDetail struct {
	ItemRef    *Ref    `json:"ItemRef,omitempty"`
	Qty        float64 `json:"Qty,omitempty"`
	UnitPrice  float64 `json:"UnitPrice,omitempty"`
	TaxCodeRef *Ref    `json:"TaxCodeRef,omitempty"`
}

// AccountBasedExpenseLineDetail is the detail for expense lines.
type AccountBasedExpenseLineDetail struct {
	AccountRef  *Ref   `json:"AccountRef"`
	TaxCodeRef  *Ref   `json:"TaxCodeRef,omitempty"`
	CustomerRef *Ref   `json:"CustomerRef,omitempty"`
}

// JournalEntryLineDetail is the detail for journal entry lines.
type JournalEntryLineDetail struct {
	PostingType string `json:"PostingType"` // Debit or Credit
	AccountRef  *Ref   `json:"AccountRef"`
}

// TxnTaxDetail holds tax information on a transaction.
type TxnTaxDetail struct {
	TxnTaxCodeRef *Ref      `json:"TxnTaxCodeRef,omitempty"`
	TotalTax      float64   `json:"TotalTax"`
	TaxLine       []TaxLine `json:"TaxLine,omitempty"`
}

// TaxLine is a single tax line on a transaction.
type TaxLine struct {
	Amount         float64        `json:"Amount"`
	DetailType     string         `json:"DetailType"` // TaxLineDetail
	TaxLineDetail  *TaxLineDetail `json:"TaxLineDetail,omitempty"`
}

// TaxLineDetail holds tax computation details.
type TaxLineDetail struct {
	TaxRateRef       *Ref    `json:"TaxRateRef,omitempty"`
	PercentBased     bool    `json:"PercentBased"`
	TaxPercent       float64 `json:"TaxPercent"`
	NetAmountTaxable float64 `json:"NetAmountTaxable"`
}

// LinkedTxn links a payment to an invoice or bill.
type LinkedTxn struct {
	TxnId   string `json:"TxnId"`
	TxnType string `json:"TxnType"`
}

// Invoice represents a QBO invoice (ACCREC).
type Invoice struct {
	Id            string         `json:"Id"`
	SyncToken     string         `json:"SyncToken"`
	DocNumber     string         `json:"DocNumber,omitempty"`
	TxnDate       string         `json:"TxnDate,omitempty"`
	DueDate       string         `json:"DueDate,omitempty"`
	CustomerRef   *Ref           `json:"CustomerRef"`
	Line          []Line         `json:"Line"`
	TxnTaxDetail  *TxnTaxDetail  `json:"TxnTaxDetail,omitempty"`
	TotalAmt      float64        `json:"TotalAmt"`
	Balance       float64        `json:"Balance"`
	EmailStatus   string         `json:"EmailStatus,omitempty"`
	CurrencyRef   *Ref           `json:"CurrencyRef,omitempty"`
	SalesTermRef  *Ref           `json:"SalesTermRef,omitempty"`
	MetaData      MetaData       `json:"MetaData"`
	Domain        string         `json:"domain"`
	Sparse        bool           `json:"sparse,omitempty"`
}

// Bill represents a QBO bill (ACCPAY).
type Bill struct {
	Id           string         `json:"Id"`
	SyncToken    string         `json:"SyncToken"`
	DocNumber    string         `json:"DocNumber,omitempty"`
	TxnDate      string         `json:"TxnDate,omitempty"`
	DueDate      string         `json:"DueDate,omitempty"`
	VendorRef    *Ref           `json:"VendorRef"`
	Line         []Line         `json:"Line"`
	TxnTaxDetail *TxnTaxDetail  `json:"TxnTaxDetail,omitempty"`
	TotalAmt     float64        `json:"TotalAmt"`
	Balance      float64        `json:"Balance"`
	CurrencyRef  *Ref           `json:"CurrencyRef,omitempty"`
	APAccountRef *Ref           `json:"APAccountRef,omitempty"`
	MetaData     MetaData       `json:"MetaData"`
	Domain       string         `json:"domain"`
}

// Payment represents a QBO customer payment.
type Payment struct {
	Id                  string   `json:"Id"`
	SyncToken           string   `json:"SyncToken"`
	TxnDate             string   `json:"TxnDate,omitempty"`
	CustomerRef         *Ref     `json:"CustomerRef"`
	TotalAmt            float64  `json:"TotalAmt"`
	UnappliedAmt        float64  `json:"UnappliedAmt"`
	Line                []Line   `json:"Line"`
	DepositToAccountRef *Ref     `json:"DepositToAccountRef,omitempty"`
	PaymentMethodRef    *Ref     `json:"PaymentMethodRef,omitempty"`
	CurrencyRef         *Ref     `json:"CurrencyRef,omitempty"`
	MetaData            MetaData `json:"MetaData"`
	Domain              string   `json:"domain"`
}

// BillPayment represents a QBO vendor bill payment.
type BillPayment struct {
	Id          string   `json:"Id"`
	SyncToken   string   `json:"SyncToken"`
	TxnDate     string   `json:"TxnDate,omitempty"`
	VendorRef   *Ref     `json:"VendorRef"`
	TotalAmt    float64  `json:"TotalAmt"`
	PayType     string   `json:"PayType"` // Check, CreditCard
	Line        []Line   `json:"Line"`
	APAccountRef *Ref    `json:"APAccountRef,omitempty"`
	CheckPayment *CheckPayment `json:"CheckPayment,omitempty"`
	MetaData    MetaData `json:"MetaData"`
	Domain      string   `json:"domain"`
}

// CheckPayment holds the bank account for check-type bill payments.
type CheckPayment struct {
	BankAccountRef *Ref `json:"BankAccountRef"`
}

// CreditMemo represents a QBO credit memo.
type CreditMemo struct {
	Id          string   `json:"Id"`
	SyncToken   string   `json:"SyncToken"`
	DocNumber   string   `json:"DocNumber,omitempty"`
	TxnDate     string   `json:"TxnDate,omitempty"`
	CustomerRef *Ref     `json:"CustomerRef"`
	Line        []Line   `json:"Line"`
	TotalAmt    float64  `json:"TotalAmt"`
	RemainingCredit float64 `json:"RemainingCredit"`
	MetaData    MetaData `json:"MetaData"`
	Domain      string   `json:"domain"`
}

// VendorCredit represents a QBO vendor credit.
type VendorCredit struct {
	Id        string   `json:"Id"`
	SyncToken string   `json:"SyncToken"`
	TxnDate   string   `json:"TxnDate,omitempty"`
	VendorRef *Ref     `json:"VendorRef"`
	Line      []Line   `json:"Line"`
	TotalAmt  float64  `json:"TotalAmt"`
	Balance   float64  `json:"Balance"`
	MetaData  MetaData `json:"MetaData"`
	Domain    string   `json:"domain"`
}

// SalesReceipt represents a QBO sales receipt (immediate payment).
type SalesReceipt struct {
	Id                  string   `json:"Id"`
	SyncToken           string   `json:"SyncToken"`
	DocNumber           string   `json:"DocNumber,omitempty"`
	TxnDate             string   `json:"TxnDate,omitempty"`
	CustomerRef         *Ref     `json:"CustomerRef"`
	Line                []Line   `json:"Line"`
	TotalAmt            float64  `json:"TotalAmt"`
	DepositToAccountRef *Ref     `json:"DepositToAccountRef,omitempty"`
	MetaData            MetaData `json:"MetaData"`
	Domain              string   `json:"domain"`
}

// Deposit represents a QBO bank deposit.
type Deposit struct {
	Id                  string   `json:"Id"`
	SyncToken           string   `json:"SyncToken"`
	TxnDate             string   `json:"TxnDate,omitempty"`
	DepositToAccountRef *Ref     `json:"DepositToAccountRef"`
	Line                []Line   `json:"Line"`
	TotalAmt            float64  `json:"TotalAmt"`
	MetaData            MetaData `json:"MetaData"`
	Domain              string   `json:"domain"`
}

// Transfer represents a QBO inter-account transfer.
type Transfer struct {
	Id             string   `json:"Id"`
	SyncToken      string   `json:"SyncToken"`
	TxnDate        string   `json:"TxnDate,omitempty"`
	FromAccountRef *Ref     `json:"FromAccountRef"`
	ToAccountRef   *Ref     `json:"ToAccountRef"`
	Amount         float64  `json:"Amount"`
	MetaData       MetaData `json:"MetaData"`
	Domain         string   `json:"domain"`
}

// JournalEntry represents a QBO manual journal entry.
type JournalEntry struct {
	Id        string   `json:"Id"`
	SyncToken string   `json:"SyncToken"`
	TxnDate   string   `json:"TxnDate,omitempty"`
	DocNumber string   `json:"DocNumber,omitempty"`
	Line      []Line   `json:"Line"`
	TotalAmt  float64  `json:"TotalAmt"`
	MetaData  MetaData `json:"MetaData"`
	Domain    string   `json:"domain"`
}

// Estimate represents a QBO estimate/quote.
type Estimate struct {
	Id          string   `json:"Id"`
	SyncToken   string   `json:"SyncToken"`
	DocNumber   string   `json:"DocNumber,omitempty"`
	TxnDate     string   `json:"TxnDate,omitempty"`
	TxnStatus   string   `json:"TxnStatus"` // Pending, Accepted, Closed, Rejected
	CustomerRef *Ref     `json:"CustomerRef"`
	Line        []Line   `json:"Line"`
	TotalAmt    float64  `json:"TotalAmt"`
	MetaData    MetaData `json:"MetaData"`
	Domain      string   `json:"domain"`
}

// Purchase represents a QBO expense (check/cash/credit card purchase).
type Purchase struct {
	Id          string   `json:"Id"`
	SyncToken   string   `json:"SyncToken"`
	TxnDate     string   `json:"TxnDate,omitempty"`
	PaymentType string   `json:"PaymentType"` // Cash, Check, CreditCard
	AccountRef  *Ref     `json:"AccountRef"`
	EntityRef   *Ref     `json:"EntityRef,omitempty"`
	Line        []Line   `json:"Line"`
	TotalAmt    float64  `json:"TotalAmt"`
	MetaData    MetaData `json:"MetaData"`
	Domain      string   `json:"domain"`
}

// Employee represents a QBO employee.
type Employee struct {
	Id          string   `json:"Id"`
	SyncToken   string   `json:"SyncToken"`
	DisplayName string   `json:"DisplayName"`
	GivenName   string   `json:"GivenName,omitempty"`
	FamilyName  string   `json:"FamilyName,omitempty"`
	Active      bool     `json:"Active"`
	MetaData    MetaData `json:"MetaData"`
	Domain      string   `json:"domain"`
}

// Class represents a QBO classification for tracking.
type Class struct {
	Id        string   `json:"Id"`
	SyncToken string   `json:"SyncToken"`
	Name      string   `json:"Name"`
	Active    bool     `json:"Active"`
	ParentRef *Ref     `json:"ParentRef,omitempty"`
	MetaData  MetaData `json:"MetaData"`
	Domain    string   `json:"domain"`
}

// Department represents a QBO department/location.
type Department struct {
	Id        string   `json:"Id"`
	SyncToken string   `json:"SyncToken"`
	Name      string   `json:"Name"`
	Active    bool     `json:"Active"`
	ParentRef *Ref     `json:"ParentRef,omitempty"`
	MetaData  MetaData `json:"MetaData"`
	Domain    string   `json:"domain"`
}

// Term represents a QBO payment term (e.g., Net 30).
type Term struct {
	Id        string   `json:"Id"`
	SyncToken string   `json:"SyncToken"`
	Name      string   `json:"Name"`
	DueDays   int      `json:"DueDays,omitempty"`
	Active    bool     `json:"Active"`
	MetaData  MetaData `json:"MetaData"`
	Domain    string   `json:"domain"`
}

// PaymentMethod represents a QBO payment method (Cash, Check, etc.).
type PaymentMethod struct {
	Id        string   `json:"Id"`
	SyncToken string   `json:"SyncToken"`
	Name      string   `json:"Name"`
	Type      string   `json:"Type,omitempty"` // CREDIT_CARD or NON_CREDIT_CARD
	Active    bool     `json:"Active"`
	MetaData  MetaData `json:"MetaData"`
	Domain    string   `json:"domain"`
}

// TaxCode represents a QBO tax code (read-only in US).
type TaxCode struct {
	Id          string `json:"Id"`
	SyncToken   string `json:"SyncToken"`
	Name        string `json:"Name"`
	Description string `json:"Description,omitempty"`
	Active      bool   `json:"Active"`
	Taxable     bool   `json:"Taxable"`
	Domain      string `json:"domain"`
}

// TaxRate represents a QBO tax rate (read-only in US).
type TaxRate struct {
	Id          string  `json:"Id"`
	SyncToken   string  `json:"SyncToken"`
	Name        string  `json:"Name"`
	Description string  `json:"Description,omitempty"`
	RateValue   float64 `json:"RateValue"`
	Active      bool    `json:"Active"`
	Domain      string  `json:"domain"`
}

// Preferences holds company-wide QBO settings.
type Preferences struct {
	Id                    string   `json:"Id"`
	SyncToken             string   `json:"SyncToken"`
	AccountingInfoPrefs   map[string]any `json:"AccountingInfoPrefs,omitempty"`
	SalesFormsPrefs       map[string]any `json:"SalesFormsPrefs,omitempty"`
	VendorAndPurchasesPrefs map[string]any `json:"VendorAndPurchasesPrefs,omitempty"`
	MetaData              MetaData `json:"MetaData"`
	Domain                string   `json:"domain"`
}

// RefundReceipt represents a QBO refund.
type RefundReceipt struct {
	Id                  string   `json:"Id"`
	SyncToken           string   `json:"SyncToken"`
	DocNumber           string   `json:"DocNumber,omitempty"`
	TxnDate             string   `json:"TxnDate,omitempty"`
	CustomerRef         *Ref     `json:"CustomerRef"`
	Line                []Line   `json:"Line"`
	TotalAmt            float64  `json:"TotalAmt"`
	DepositToAccountRef *Ref     `json:"DepositToAccountRef,omitempty"`
	MetaData            MetaData `json:"MetaData"`
	Domain              string   `json:"domain"`
}

// PurchaseOrder represents a QBO purchase order.
type PurchaseOrder struct {
	Id        string   `json:"Id"`
	SyncToken string   `json:"SyncToken"`
	DocNumber string   `json:"DocNumber,omitempty"`
	TxnDate   string   `json:"TxnDate,omitempty"`
	VendorRef *Ref     `json:"VendorRef"`
	Line      []Line   `json:"Line"`
	TotalAmt  float64  `json:"TotalAmt"`
	POStatus  string   `json:"POStatus"` // Open, Closed
	MetaData  MetaData `json:"MetaData"`
	Domain    string   `json:"domain"`
}

// TimeActivity represents a QBO time tracking entry.
type TimeActivity struct {
	Id          string   `json:"Id"`
	SyncToken   string   `json:"SyncToken"`
	TxnDate     string   `json:"TxnDate,omitempty"`
	EmployeeRef *Ref     `json:"EmployeeRef,omitempty"`
	CustomerRef *Ref     `json:"CustomerRef,omitempty"`
	ItemRef     *Ref     `json:"ItemRef,omitempty"`
	Hours       int      `json:"Hours,omitempty"`
	Minutes     int      `json:"Minutes,omitempty"`
	Description string   `json:"Description,omitempty"`
	MetaData    MetaData `json:"MetaData"`
	Domain      string   `json:"domain"`
}

// CompanyInfo holds the company settings (single record, Id="1").
type CompanyInfo struct {
	Id                string   `json:"Id"`
	SyncToken         string   `json:"SyncToken"`
	CompanyName       string   `json:"CompanyName"`
	LegalName         string   `json:"LegalName,omitempty"`
	Country           string   `json:"Country,omitempty"`
	FiscalYearStartMonth string `json:"FiscalYearStartMonth,omitempty"`
	MetaData          MetaData `json:"MetaData"`
	Domain            string   `json:"domain"`
}
