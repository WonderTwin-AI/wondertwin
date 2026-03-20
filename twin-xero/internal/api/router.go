// Package api implements the Xero-compatible HTTP API handlers for the twin.
package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger"
	"github.com/wondertwin-ai/wondertwin/twinkit/quirks"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// Handler holds all API handler state.
type Handler struct {
	store      *store.MemoryStore
	engine     *ledger.Engine
	dispatcher *webhook.Dispatcher
	mw         *twincore.Middleware
	emitter    *telemetry.Emitter
	quirks     *quirks.Engine
}

// NewHandler creates a new API handler.
func NewHandler(s *store.MemoryStore, engine *ledger.Engine, d *webhook.Dispatcher, mw *twincore.Middleware, em *telemetry.Emitter, qe *quirks.Engine) *Handler {
	return &Handler{store: s, engine: engine, dispatcher: d, mw: mw, emitter: em, quirks: qe}
}

// Routes mounts the Xero API routes.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/api.xro/2.0", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Use(tenantMiddleware)
		r.Use(h.mw.FaultInjection)
		r.Use(quirks.Middleware(h.quirks))
		r.Use(telemetry.Middleware(h.emitter))

		// Contacts — CRUD only (no engine)
		r.Post("/Contacts", h.CreateContact)
		r.Get("/Contacts", h.ListContacts)
		r.Get("/Contacts/{ContactID}", h.GetContact)
		r.Put("/Contacts/{ContactID}", h.UpdateContact)
		r.Delete("/Contacts/{ContactID}", h.DeleteContact)

		// Accounts — CRUD + engine.CreateAccount
		r.Post("/Accounts", h.CreateAccount)
		r.Get("/Accounts", h.ListAccounts)
		r.Get("/Accounts/{AccountID}", h.GetAccount)
		r.Put("/Accounts/{AccountID}", h.UpdateAccount)
		r.Delete("/Accounts/{AccountID}", h.DeleteAccount)

		// Invoices — engine state transitions
		r.Post("/Invoices", h.CreateOrUpdateInvoice)
		r.Get("/Invoices", h.ListInvoices)
		r.Get("/Invoices/{InvoiceID}", h.GetInvoice)
		r.Put("/Invoices/{InvoiceID}", h.UpdateInvoice)
		r.Delete("/Invoices/{InvoiceID}", h.DeleteInvoice)

		// Credit Notes
		r.Post("/CreditNotes", h.CreateOrUpdateCreditNote)
		r.Get("/CreditNotes", h.ListCreditNotes)
		r.Get("/CreditNotes/{CreditNoteID}", h.GetCreditNote)
		r.Put("/CreditNotes/{CreditNoteID}/Allocations", h.AllocateCreditNote)

		// Payments — PUT per Xero convention
		r.Put("/Payments", h.CreatePayment)
		r.Get("/Payments", h.ListPayments)
		r.Get("/Payments/{PaymentID}", h.GetPayment)
		r.Delete("/Payments/{PaymentID}", h.DeletePayment)

		// Bank Transactions — PUT per Xero convention
		r.Put("/BankTransactions", h.CreateBankTransaction)
		r.Get("/BankTransactions", h.ListBankTransactions)
		r.Get("/BankTransactions/{BankTransactionID}", h.GetBankTransaction)
		r.Delete("/BankTransactions/{BankTransactionID}", h.DeleteBankTransaction)

		// Manual Journals — PUT per Xero convention
		r.Put("/ManualJournals", h.CreateManualJournal)
		r.Get("/ManualJournals", h.ListManualJournals)
		r.Get("/ManualJournals/{ManualJournalID}", h.GetManualJournal)

		// Items — CRUD only
		r.Post("/Items", h.CreateItem)
		r.Get("/Items", h.ListItems)
		r.Get("/Items/{ItemID}", h.GetItem)
		r.Put("/Items/{ItemID}", h.UpdateItem)
		r.Delete("/Items/{ItemID}", h.DeleteItem)

		// Tax Rates
		r.Put("/TaxRates", h.CreateTaxRate)
		r.Get("/TaxRates", h.ListTaxRates)

		// Tracking Categories
		r.Post("/TrackingCategories", h.CreateTrackingCategory)
		r.Get("/TrackingCategories", h.ListTrackingCategories)
		r.Get("/TrackingCategories/{TrackingCategoryID}", h.GetTrackingCategory)
		r.Put("/TrackingCategories/{TrackingCategoryID}", h.UpdateTrackingCategory)
		r.Delete("/TrackingCategories/{TrackingCategoryID}", h.DeleteTrackingCategory)

		// Prepayments
		r.Put("/Prepayments", h.CreatePrepayment)
		r.Get("/Prepayments", h.ListPrepayments)
		r.Get("/Prepayments/{PrepaymentID}", h.GetPrepayment)

		// Overpayments
		r.Put("/Overpayments", h.CreateOverpayment)
		r.Get("/Overpayments", h.ListOverpayments)
		r.Get("/Overpayments/{OverpaymentID}", h.GetOverpayment)

		// Reports
		r.Get("/Reports/TrialBalance", h.TrialBalance)
		r.Get("/Reports/ProfitAndLoss", h.ProfitAndLoss)
		r.Get("/Reports/BalanceSheet", h.BalanceSheet)
	})
}
