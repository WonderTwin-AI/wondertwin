// Package api implements the QBO-compatible HTTP API handlers for the twin.
package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

// Handler holds all API handler state.
type Handler struct {
	store      *store.MemoryStore
	engine     *ledger.Engine
	dispatcher *webhook.Dispatcher
	mw         *twincore.Middleware
	emitter    *telemetry.Emitter
}

// NewHandler creates a new API handler.
func NewHandler(s *store.MemoryStore, engine *ledger.Engine, d *webhook.Dispatcher, mw *twincore.Middleware, em *telemetry.Emitter) *Handler {
	return &Handler{store: s, engine: engine, dispatcher: d, mw: mw, emitter: em}
}

// Routes mounts the QBO v3 API routes.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/v3/company/{realmId}", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Use(realmIDMiddleware)
		r.Use(minorVersionMiddleware)
		r.Use(h.mw.FaultInjection)
		r.Use(telemetry.Middleware(h.emitter))

		// Query endpoint (SQL-like)
		r.Get("/query", h.Query)

		// Batch endpoint (up to 30 operations)
		r.Post("/batch", h.Batch)

		// ChangeDataCapture
		r.Get("/cdc", h.CDC)

		// Customers
		r.Post("/customer", h.CreateOrUpdateCustomer)
		r.Get("/customer/{id}", h.GetCustomer)

		// Vendors
		r.Post("/vendor", h.CreateOrUpdateVendor)
		r.Get("/vendor/{id}", h.GetVendor)

		// Accounts
		r.Post("/account", h.CreateOrUpdateAccount)
		r.Get("/account/{id}", h.GetAccount)

		// Items
		r.Post("/item", h.CreateOrUpdateItem)
		r.Get("/item/{id}", h.GetItem)

		// Invoices
		r.Post("/invoice", h.CreateOrUpdateInvoice)
		r.Get("/invoice/{id}", h.GetInvoice)
		r.Post("/invoice/{id}/send", h.SendInvoice)
		r.Get("/invoice/{id}/pdf", h.InvoicePDF)

		// Bills
		r.Post("/bill", h.CreateOrUpdateBill)
		r.Get("/bill/{id}", h.GetBill)

		// Payments
		r.Post("/payment", h.CreateOrUpdatePayment)
		r.Get("/payment/{id}", h.GetPayment)

		// Bill Payments
		r.Post("/billpayment", h.CreateOrUpdateBillPayment)
		r.Get("/billpayment/{id}", h.GetBillPayment)

		// Credit Memos
		r.Post("/creditmemo", h.CreateOrUpdateCreditMemo)
		r.Get("/creditmemo/{id}", h.GetCreditMemo)

		// Vendor Credits
		r.Post("/vendorcredit", h.CreateOrUpdateVendorCredit)
		r.Get("/vendorcredit/{id}", h.GetVendorCredit)

		// Sales Receipts
		r.Post("/salesreceipt", h.CreateOrUpdateSalesReceipt)
		r.Get("/salesreceipt/{id}", h.GetSalesReceipt)
		r.Post("/salesreceipt/{id}/send", h.SendSalesReceipt)

		// Deposits
		r.Post("/deposit", h.CreateOrUpdateDeposit)
		r.Get("/deposit/{id}", h.GetDeposit)

		// Transfers
		r.Post("/transfer", h.CreateOrUpdateTransfer)
		r.Get("/transfer/{id}", h.GetTransfer)

		// Journal Entries
		r.Post("/journalentry", h.CreateOrUpdateJournalEntry)
		r.Get("/journalentry/{id}", h.GetJournalEntry)

		// Estimates
		r.Post("/estimate", h.CreateOrUpdateEstimate)
		r.Get("/estimate/{id}", h.GetEstimate)
		r.Post("/estimate/{id}/send", h.SendEstimate)
		r.Post("/estimate/{id}/convert", h.ConvertEstimate)

		// Purchases
		r.Post("/purchase", h.CreateOrUpdatePurchase)
		r.Get("/purchase/{id}", h.GetPurchase)

		// Employees
		r.Post("/employee", h.CreateOrUpdateEmployee)
		r.Get("/employee/{id}", h.GetEmployee)

		// Classes
		r.Post("/class", h.CreateOrUpdateClass)
		r.Get("/class/{id}", h.GetClass)

		// Departments
		r.Post("/department", h.CreateOrUpdateDepartment)
		r.Get("/department/{id}", h.GetDepartment)

		// Terms
		r.Post("/term", h.CreateOrUpdateTerm)
		r.Get("/term/{id}", h.GetTerm)

		// Payment Methods
		r.Post("/paymentmethod", h.CreateOrUpdatePaymentMethod)
		r.Get("/paymentmethod/{id}", h.GetPaymentMethod)

		// Tax Codes (read-only)
		r.Get("/taxcode/{id}", h.GetTaxCode)

		// Tax Rates (read-only)
		r.Get("/taxrate/{id}", h.GetTaxRate)

		// Preferences
		r.Get("/preferences", h.GetPreferences)
		r.Post("/preferences", h.UpdatePreferences)

		// Refund Receipts
		r.Post("/refundreceipt", h.CreateOrUpdateRefundReceipt)
		r.Get("/refundreceipt/{id}", h.GetRefundReceipt)

		// Purchase Orders
		r.Post("/purchaseorder", h.CreateOrUpdatePurchaseOrder)
		r.Get("/purchaseorder/{id}", h.GetPurchaseOrder)

		// Time Activities
		r.Post("/timeactivity", h.CreateOrUpdateTimeActivity)
		r.Get("/timeactivity/{id}", h.GetTimeActivity)

		// Recurring Transactions
		r.Post("/recurringtransaction", h.CreateOrUpdateRecurringTransaction)
		r.Get("/recurringtransaction/{id}", h.GetRecurringTransaction)

		// Reports
		r.Get("/reports/ProfitAndLoss", h.ProfitAndLoss)
		r.Get("/reports/BalanceSheet", h.BalanceSheet)
		r.Get("/reports/TrialBalance", h.TrialBalance)
		r.Get("/reports/AgedReceivables", h.AgedReceivables)
		r.Get("/reports/AgedPayables", h.AgedPayables)
		r.Get("/reports/CustomerBalance", h.CustomerBalance)
		r.Get("/reports/VendorBalance", h.VendorBalance)
		r.Get("/reports/GeneralLedger", h.GeneralLedger)

		// Company Info
		r.Get("/companyinfo/{id}", h.GetCompanyInfo)
		r.Post("/companyinfo", h.UpdateCompanyInfo)
	})
}
