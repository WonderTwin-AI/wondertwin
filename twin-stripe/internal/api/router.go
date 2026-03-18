// Package api implements the Stripe-compatible HTTP API handlers for the twin.
package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// Handler holds all API handler state.
type Handler struct {
	store      *store.MemoryStore
	dispatcher *webhook.Dispatcher
	mw         *twincore.Middleware
}

// NewHandler creates a new API handler.
func NewHandler(s *store.MemoryStore, d *webhook.Dispatcher, mw *twincore.Middleware) *Handler {
	return &Handler{store: s, dispatcher: d, mw: mw}
}

// Routes mounts the Stripe v1 API routes.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/v1", func(r chi.Router) {
		// Auth middleware for all v1 routes
		r.Use(h.authMiddleware)
		// Idempotency key caching for POST requests
		r.Use(h.idempotencyMiddleware)
		// Fault injection for API routes (not admin)
		r.Use(h.mw.FaultInjection)

		// Customers
		r.Post("/customers", h.CreateCustomer)
		r.Get("/customers/{id}", h.GetCustomer)
		r.Post("/customers/{id}", h.UpdateCustomer)
		r.Delete("/customers/{id}", h.DeleteCustomer)
		r.Get("/customers", h.ListCustomers)

		// Products
		r.Post("/products", h.CreateProduct)
		r.Get("/products/{id}", h.GetProduct)
		r.Post("/products/{id}", h.UpdateProduct)
		r.Delete("/products/{id}", h.DeleteProduct)
		r.Get("/products", h.ListProducts)

		// Prices
		r.Post("/prices", h.CreatePrice)
		r.Get("/prices/{id}", h.GetPrice)
		r.Post("/prices/{id}", h.UpdatePrice)
		r.Get("/prices", h.ListPrices)

		// Payment Intents
		r.Post("/payment_intents", h.CreatePaymentIntent)
		r.Get("/payment_intents/{id}", h.GetPaymentIntent)
		r.Post("/payment_intents/{id}/confirm", h.ConfirmPaymentIntent)
		r.Post("/payment_intents/{id}/cancel", h.CancelPaymentIntent)
		r.Get("/payment_intents", h.ListPaymentIntents)

		// Payment Methods
		r.Post("/payment_methods", h.CreatePaymentMethod)
		r.Get("/payment_methods/{id}", h.GetPaymentMethod)
		r.Post("/payment_methods/{id}/attach", h.AttachPaymentMethod)
		r.Post("/payment_methods/{id}/detach", h.DetachPaymentMethod)
		r.Get("/payment_methods", h.ListPaymentMethods)

		// Charges
		r.Get("/charges/{id}", h.GetCharge)
		r.Get("/charges", h.ListCharges)

		// Refunds
		r.Post("/refunds", h.CreateRefund)
		r.Get("/refunds/{id}", h.GetRefund)
		r.Get("/refunds", h.ListRefunds)

		// Subscriptions
		r.Post("/subscriptions", h.CreateSubscription)
		r.Get("/subscriptions/{id}", h.GetSubscription)
		r.Post("/subscriptions/{id}", h.UpdateSubscription)
		r.Delete("/subscriptions/{id}", h.CancelSubscription)
		r.Get("/subscriptions", h.ListSubscriptions)

		// Invoices
		r.Post("/invoices", h.CreateInvoice)
		r.Get("/invoices/{id}", h.GetInvoice)
		r.Post("/invoices/{id}/finalize", h.FinalizeInvoice)
		r.Post("/invoices/{id}/pay", h.PayInvoice)
		r.Post("/invoices/{id}/void", h.VoidInvoice)
		r.Get("/invoices", h.ListInvoices)

		// Invoice Items
		r.Post("/invoiceitems", h.CreateInvoiceItem)
		r.Get("/invoiceitems/{id}", h.GetInvoiceItem)
		r.Delete("/invoiceitems/{id}", h.DeleteInvoiceItem)

		// Coupons
		r.Post("/coupons", h.CreateCoupon)
		r.Get("/coupons/{id}", h.GetCoupon)
		r.Delete("/coupons/{id}", h.DeleteCoupon)
		r.Get("/coupons", h.ListCoupons)

		// Setup Intents
		r.Post("/setup_intents", h.CreateSetupIntent)
		r.Get("/setup_intents/{id}", h.GetSetupIntent)
		r.Post("/setup_intents/{id}/confirm", h.ConfirmSetupIntent)
		r.Post("/setup_intents/{id}/cancel", h.CancelSetupIntent)
		r.Get("/setup_intents", h.ListSetupIntents)

		// Tax Rates
		r.Post("/tax_rates", h.CreateTaxRate)
		r.Get("/tax_rates/{id}", h.GetTaxRate)
		r.Get("/tax_rates", h.ListTaxRates)

		// Disputes
		r.Get("/disputes/{id}", h.GetDispute)
		r.Post("/disputes/{id}/close", h.CloseDispute)
		r.Get("/disputes", h.ListDisputes)

		// Accounts
		r.Post("/accounts", h.CreateAccount)
		r.Get("/accounts/{id}", h.GetAccount)
		r.Post("/accounts/{id}", h.UpdateAccount)
		r.Delete("/accounts/{id}", h.DeleteAccount)
		r.Get("/accounts", h.ListAccounts)

		// External Accounts
		r.Post("/accounts/{account_id}/external_accounts", h.CreateExternalAccount)
		r.Get("/accounts/{account_id}/external_accounts/{id}", h.GetExternalAccount)
		r.Post("/accounts/{account_id}/external_accounts/{id}", h.UpdateExternalAccount)
		r.Delete("/accounts/{account_id}/external_accounts/{id}", h.DeleteExternalAccount)

		// Transfers
		r.Post("/transfers", h.CreateTransfer)
		r.Get("/transfers/{id}", h.GetTransfer)
		r.Get("/transfers", h.ListTransfers)

		// Balance
		r.Get("/balance", h.GetBalance)

		// Payouts
		r.Post("/payouts", h.CreatePayout)
		r.Get("/payouts/{id}", h.GetPayout)
		r.Get("/payouts", h.ListPayouts)

		// Balance Transactions
		r.Get("/balance_transactions", h.ListBalanceTransactions)
		r.Get("/balance_transactions/{id}", h.GetBalanceTransaction)

		// Events
		r.Get("/events", h.ListEvents)
		r.Get("/events/{id}", h.GetEvent)

		// Checkout Sessions
		r.Post("/checkout/sessions", h.CreateCheckoutSession)
		r.Get("/checkout/sessions/{id}", h.GetCheckoutSession)
		r.Get("/checkout/sessions", h.ListCheckoutSessions)
		r.Post("/checkout/sessions/{id}/expire", h.ExpireCheckoutSession)

		// Payment Links
		r.Post("/payment_links", h.CreatePaymentLink)
		r.Get("/payment_links/{id}", h.GetPaymentLink)
		r.Post("/payment_links/{id}", h.UpdatePaymentLink)
		r.Get("/payment_links", h.ListPaymentLinks)

		// Tokens
		r.Post("/tokens", h.CreateToken)
		r.Get("/tokens/{id}", h.GetToken)

		// Sources
		r.Post("/sources", h.CreateSource)
		r.Get("/sources/{id}", h.GetSource)
		r.Post("/sources/{id}", h.UpdateSource)

		// Mandates
		r.Get("/mandates/{id}", h.GetMandate)

		// Confirmation Tokens
		r.Get("/confirmation_tokens/{id}", h.GetConfirmationToken)

		// Credit Notes
		r.Post("/credit_notes", h.CreateCreditNote)
		r.Get("/credit_notes/preview", h.PreviewCreditNote)
		r.Get("/credit_notes/{id}", h.GetCreditNote)
		r.Get("/credit_notes", h.ListCreditNotes)
		r.Post("/credit_notes/{id}/void", h.VoidCreditNote)

		// Promotion Codes
		r.Post("/promotion_codes", h.CreatePromotionCode)
		r.Get("/promotion_codes/{id}", h.GetPromotionCode)
		r.Post("/promotion_codes/{id}", h.UpdatePromotionCode)
		r.Get("/promotion_codes", h.ListPromotionCodes)

		// Subscription Items
		r.Post("/subscription_items", h.CreateSubscriptionItem)
		r.Get("/subscription_items/{id}", h.GetSubscriptionItem)
		r.Post("/subscription_items/{id}", h.UpdateSubscriptionItem)
		r.Delete("/subscription_items/{id}", h.DeleteSubscriptionItem)
		r.Get("/subscription_items", h.ListSubscriptionItems)

		// Quotes
		r.Post("/quotes", h.CreateQuote)
		r.Get("/quotes/{id}", h.GetQuote)
		r.Post("/quotes/{id}", h.UpdateQuote)
		r.Post("/quotes/{id}/finalize", h.FinalizeQuote)
		r.Post("/quotes/{id}/accept", h.AcceptQuote)
		r.Post("/quotes/{id}/cancel", h.CancelQuote)
		r.Get("/quotes", h.ListQuotes)

		// Billing Portal Sessions
		r.Post("/billing_portal/sessions", h.CreateBillingPortalSession)

		// Reviews
		r.Get("/reviews/{id}", h.GetReview)
		r.Get("/reviews", h.ListReviews)
		r.Post("/reviews/{id}/approve", h.ApproveReview)

		// Tax IDs (nested under customers)
		r.Post("/customers/{customer_id}/tax_ids", h.CreateTaxID)
		r.Get("/customers/{customer_id}/tax_ids/{id}", h.GetCustomerTaxID)
		r.Delete("/customers/{customer_id}/tax_ids/{id}", h.DeleteTaxID)
		r.Get("/customers/{customer_id}/tax_ids", h.ListCustomerTaxIDs)

		// Tax IDs (top-level)
		r.Get("/tax_ids/{id}", h.GetTaxID)
		r.Get("/tax_ids", h.ListTaxIDs)

		// Webhook Endpoints
		r.Post("/webhook_endpoints", h.CreateWebhookEndpoint)
		r.Get("/webhook_endpoints/{id}", h.GetWebhookEndpoint)
		r.Post("/webhook_endpoints/{id}", h.UpdateWebhookEndpoint)
		r.Delete("/webhook_endpoints/{id}", h.DeleteWebhookEndpoint)
		r.Get("/webhook_endpoints", h.ListWebhookEndpoints)

		// Files
		r.Post("/files", h.CreateFile)
		r.Get("/files/{id}", h.GetFile)
		r.Get("/files", h.ListFiles)

		// File Links
		r.Post("/file_links", h.CreateFileLink)
		r.Get("/file_links/{id}", h.GetFileLink)
		r.Post("/file_links/{id}", h.UpdateFileLink)
		r.Get("/file_links", h.ListFileLinks)

		// Shipping Rates
		r.Post("/shipping_rates", h.CreateShippingRate)
		r.Get("/shipping_rates/{id}", h.GetShippingRate)
		r.Post("/shipping_rates/{id}", h.UpdateShippingRate)
		r.Get("/shipping_rates", h.ListShippingRates)

		// Application Fees
		r.Get("/application_fees/{id}", h.GetApplicationFee)
		r.Get("/application_fees", h.ListApplicationFees)

		// Application Fee Refunds (nested under application fees)
		r.Post("/application_fees/{fee_id}/refunds", h.CreateApplicationFeeRefund)
		r.Get("/application_fees/{fee_id}/refunds/{id}", h.GetApplicationFeeRefund)
		r.Get("/application_fees/{fee_id}/refunds", h.ListApplicationFeeRefunds)

		// Transfer Reversals (nested under transfers)
		r.Post("/transfers/{transfer_id}/reversals", h.CreateTransferReversal)
		r.Get("/transfers/{transfer_id}/reversals/{id}", h.GetTransferReversal)
		r.Get("/transfers/{transfer_id}/reversals", h.ListTransferReversals)

		// Account Links
		r.Post("/account_links", h.CreateAccountLink)

		// Persons (nested under accounts)
		r.Post("/accounts/{account_id}/persons", h.CreatePerson)
		r.Get("/accounts/{account_id}/persons/{id}", h.GetPerson)
		r.Post("/accounts/{account_id}/persons/{id}", h.UpdatePerson)
		r.Delete("/accounts/{account_id}/persons/{id}", h.DeletePerson)
		r.Get("/accounts/{account_id}/persons", h.ListPersons)

		// TopUps
		r.Post("/topups", h.CreateTopUp)
		r.Get("/topups/{id}", h.GetTopUp)
		r.Get("/topups", h.ListTopUps)
	})

	// Stripe-specific admin endpoints (outside /v1, no auth)
	r.Post("/admin/payouts/{id}/fail", h.AdminFailPayout)
	r.Post("/admin/accounts/{id}/fund", h.AdminFundAccount)
}

// authMiddleware validates Stripe-style Bearer token authentication.
func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			twincore.StripeError(w, http.StatusUnauthorized,
				"invalid_request_error", "api_key_required",
				"You did not provide an API key.")
			return
		}

		// Accept "Bearer sk_test_*" or "Bearer sk_sim_*" or just "Bearer <anything>"
		key := strings.TrimPrefix(auth, "Bearer ")
		if key == auth {
			// No "Bearer " prefix, also check for raw key
			key = auth
		}
		if key == "" {
			twincore.StripeError(w, http.StatusUnauthorized,
				"invalid_request_error", "api_key_required",
				"You did not provide an API key.")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// stripeAccountFromRequest extracts the Stripe-Account header for connected account context.
func stripeAccountFromRequest(r *http.Request) string {
	return r.Header.Get("Stripe-Account")
}

// parseFormOrJSON extracts form values from either form-encoded or JSON body.
// Stripe's API uses form-encoded requests.
func parseFormOrJSON(r *http.Request) error {
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/x-www-form-urlencoded") || strings.Contains(ct, "multipart/form-data") {
		return r.ParseForm()
	}
	// Also parse form for requests without explicit content type (Stripe SDK default)
	return r.ParseForm()
}
