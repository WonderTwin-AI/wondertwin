package store

import (
	"fmt"
	"time"
)

// Account is a chart-of-accounts entry. UpdatedDateUTC is written as
// /Date(ms+0000)/ by the API layer, which shadows it.
type Account struct {
	Code                    string    `json:"Code"`
	Name                    string    `json:"Name"`
	AccountID               string    `json:"AccountID"`
	Type                    string    `json:"Type"`
	BankAccountNumber       string    `json:"BankAccountNumber,omitempty"`
	Status                  string    `json:"Status"`
	Description             string    `json:"Description,omitempty"`
	BankAccountType         string    `json:"BankAccountType,omitempty"`
	CurrencyCode            string    `json:"CurrencyCode,omitempty"`
	TaxType                 string    `json:"TaxType"`
	EnablePaymentsToAccount bool      `json:"EnablePaymentsToAccount"`
	ShowInExpenseClaims     bool      `json:"ShowInExpenseClaims"`
	Class                   string    `json:"Class"`
	SystemAccount           string    `json:"SystemAccount,omitempty"`
	ReportingCode           string    `json:"ReportingCode,omitempty"`
	ReportingCodeName       string    `json:"ReportingCodeName,omitempty"`
	HasAttachments          bool      `json:"HasAttachments"`
	UpdatedDateUTC          time.Time `json:"UpdatedDateUTC"`
	AddToWatchlist          bool      `json:"AddToWatchlist"`
}

// seedAccountID is a fixed ID per chart position, so IDs survive a reset.
func seedAccountID(i int) string { return fmt.Sprintf("a0000000-0000-4000-8000-%012d", i+1) }

func account(code, name, typ, class, tax, system string) Account {
	return Account{Code: code, Name: name, Type: typ, Class: class, TaxType: tax, SystemAccount: system, Status: "ACTIVE"}
}

// defaultChart is the demo company's chart: the accounts finance-ops posts to,
// the system accounts Xero requires, and retained earnings.
var defaultChart = []Account{
	func() Account {
		a := account("090", "Business Bank Account", "BANK", "ASSET", "NONE", "")
		a.BankAccountNumber, a.BankAccountType, a.CurrencyCode, a.EnablePaymentsToAccount = "0123456789", "BANK", "USD", true
		return a
	}(),
	account("200", "Sales - Monthly Subscriptions", "REVENUE", "REVENUE", "OUTPUT", ""),
	account("201", "Sales - Annual Subscriptions", "REVENUE", "REVENUE", "OUTPUT", ""),
	account("404", "Bank Fees", "EXPENSE", "EXPENSE", "NONE", ""),
	account("610", "Accounts Receivable", "CURRENT", "ASSET", "NONE", "DEBTORS"),
	account("800", "Accounts Payable", "CURRLIAB", "LIABILITY", "NONE", "CREDITORS"),
	account("820", "Sales Tax", "CURRLIAB", "LIABILITY", "NONE", "GST"),
	account("960", "Retained Earnings", "EQUITY", "EQUITY", "NONE", "RETAINEDEARNINGS"),
}

// seedChart restores the default tenant's chart, which carries the accounts
// localstack/finance-ops posts to (090, 200, 201, 404).
func (s *MemoryStore) seedChart(now time.Time) {
	accounts := s.Accounts.For(DefaultTenantID)
	for i, a := range defaultChart {
		a.AccountID, a.UpdatedDateUTC = seedAccountID(i), now
		accounts.Set(a.AccountID, a)
	}
}
