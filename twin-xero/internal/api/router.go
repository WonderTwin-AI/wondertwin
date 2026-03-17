// Package api implements the Xero-compatible HTTP API handlers for the twin.
package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger/accounting"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
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

// Routes mounts the Xero API routes.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/api.xro/2.0", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Use(tenantMiddleware)
		r.Use(h.mw.FaultInjection)

		// Contacts — CRUD only (no engine)
		r.Post("/Contacts", h.CreateContact)
		r.Get("/Contacts", h.ListContacts)
		r.Get("/Contacts/{ContactID}", h.GetContact)

		// Accounts — CRUD + engine.CreateAccount
		r.Post("/Accounts", h.CreateAccount)
		r.Get("/Accounts", h.ListAccounts)
		r.Get("/Accounts/{AccountID}", h.GetAccount)

		// Invoices — engine state transitions
		r.Post("/Invoices", h.CreateOrUpdateInvoice)
		r.Get("/Invoices", h.ListInvoices)
		r.Get("/Invoices/{InvoiceID}", h.GetInvoice)

		// Credit Notes
		r.Post("/CreditNotes", h.CreateOrUpdateCreditNote)
		r.Get("/CreditNotes", h.ListCreditNotes)
		r.Get("/CreditNotes/{CreditNoteID}", h.GetCreditNote)

		// Payments — PUT per Xero convention
		r.Put("/Payments", h.CreatePayment)
		r.Get("/Payments", h.ListPayments)
		r.Get("/Payments/{PaymentID}", h.GetPayment)

		// Bank Transactions — PUT per Xero convention
		r.Put("/BankTransactions", h.CreateBankTransaction)
		r.Get("/BankTransactions", h.ListBankTransactions)
		r.Get("/BankTransactions/{BankTransactionID}", h.GetBankTransaction)

		// Manual Journals — PUT per Xero convention
		r.Put("/ManualJournals", h.CreateManualJournal)
		r.Get("/ManualJournals", h.ListManualJournals)
		r.Get("/ManualJournals/{ManualJournalID}", h.GetManualJournal)

		// Items — CRUD only
		r.Post("/Items", h.CreateItem)
		r.Get("/Items", h.ListItems)
		r.Get("/Items/{ItemID}", h.GetItem)

		// Reports
		r.Get("/Reports/TrialBalance", h.TrialBalance)
		r.Get("/Reports/ProfitAndLoss", h.ProfitAndLoss)
		r.Get("/Reports/BalanceSheet", h.BalanceSheet)
	})
}
