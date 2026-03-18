// Package api implements the QBO-compatible HTTP API handlers for the twin.
package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger/accounting"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

// Handler holds all API handler state.
type Handler struct {
	store      *store.MemoryStore
	engine     *accounting.Engine
	dispatcher *webhook.Dispatcher
	mw         *twincore.Middleware
}

// NewHandler creates a new API handler.
func NewHandler(s *store.MemoryStore, engine *accounting.Engine, d *webhook.Dispatcher, mw *twincore.Middleware) *Handler {
	return &Handler{store: s, engine: engine, dispatcher: d, mw: mw}
}

// Routes mounts the QBO v3 API routes.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/v3/company/{realmId}", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Use(realmIDMiddleware)
		r.Use(h.mw.FaultInjection)

		// Query endpoint (SQL-like)
		r.Get("/query", h.Query)

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

		// Purchases
		r.Post("/purchase", h.CreateOrUpdatePurchase)
		r.Get("/purchase/{id}", h.GetPurchase)

		// Reports
		r.Get("/reports/ProfitAndLoss", h.ProfitAndLoss)
		r.Get("/reports/BalanceSheet", h.BalanceSheet)
		r.Get("/reports/TrialBalance", h.TrialBalance)

		// Company Info
		r.Get("/companyinfo/{id}", h.GetCompanyInfo)
		r.Post("/companyinfo", h.UpdateCompanyInfo)
	})
}
