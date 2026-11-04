package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// CreateExternalAccount handles POST /v1/accounts/{account_id}/external_accounts.
func (h *Handler) CreateExternalAccount(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "account_id")

	if _, ok := h.store.Accounts.Get(accountID); !ok {
		stripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such account: '"+accountID+"'")
		return
	}

	if err := parseFormOrJSON(r); err != nil {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", err.Error())
		return
	}

	// A token arrives as the plain string form of external_account; the
	// dictionary form arrives as external_account[...] fields.
	if token := r.FormValue("external_account"); token != "" {
		bank, ok := h.resolveBankToken(w, token)
		if !ok {
			return
		}
		ea := store.ExternalAccount{
			ID:                 h.store.StripeID(h.store.ExternalAccts.NextID()),
			Object:             "bank_account",
			AccountID:          accountID,
			AccountHolderName:  bank.AccountHolderName,
			BankName:           bank.BankName,
			Country:            bank.Country,
			Currency:           bank.Currency,
			Last4:              bank.Last4,
			RoutingNumber:      bank.RoutingNumber,
			Status:             "new",
			DefaultForCurrency: true,
			Fingerprint:        bank.Fingerprint,
			Metadata:           parseMetadata(r),
		}
		h.store.ExternalAccts.Set(ea.ID, ea)
		twincore.JSON(w, http.StatusOK, ea)
		return
	}

	id := h.store.StripeID(h.store.ExternalAccts.NextID())

	routingNumber := r.FormValue("external_account[routing_number]")
	if routingNumber == "" {
		routingNumber = r.FormValue("routing_number")
	}
	accountNumber := r.FormValue("external_account[account_number]")
	if accountNumber == "" {
		accountNumber = r.FormValue("account_number")
	}
	country := r.FormValue("external_account[country]")
	if country == "" {
		country = r.FormValue("country")
		if country == "" {
			country = "US"
		}
	}
	currency := r.FormValue("external_account[currency]")
	if currency == "" {
		currency = r.FormValue("currency")
		if currency == "" {
			currency = "usd"
		}
	}

	// Generate last4 from account number
	last4 := "0000"
	if len(accountNumber) >= 4 {
		last4 = accountNumber[len(accountNumber)-4:]
	}

	// Generate fingerprint
	fingerprint := bankFingerprint(routingNumber, accountNumber)

	ea := store.ExternalAccount{
		ID:                 id,
		Object:             "bank_account",
		AccountID:          accountID,
		BankName:           "STRIPE TEST BANK",
		Country:            country,
		Currency:           currency,
		Last4:              last4,
		RoutingNumber:      routingNumber,
		Status:             "new",
		DefaultForCurrency: true,
		Fingerprint:        fingerprint,
		Metadata:           parseMetadata(r),
	}

	h.store.ExternalAccts.Set(id, ea)

	twincore.JSON(w, http.StatusOK, ea)
}

// GetExternalAccount handles GET /v1/accounts/{account_id}/external_accounts/{id}.
func (h *Handler) GetExternalAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ea, ok := h.store.ExternalAccts.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such external account: '"+id+"'")
		return
	}

	twincore.JSON(w, http.StatusOK, ea)
}

// UpdateExternalAccount handles POST /v1/accounts/{account_id}/external_accounts/{id}.
func (h *Handler) UpdateExternalAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ea, ok := h.store.ExternalAccts.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such external account: '"+id+"'")
		return
	}

	if err := parseFormOrJSON(r); err != nil {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", err.Error())
		return
	}

	meta, ok := updateMetadata(w, r, ea.Metadata)
	if !ok {
		return
	}
	ea.Metadata = meta

	if v := r.FormValue("default_for_currency"); v == "true" {
		ea.DefaultForCurrency = true
	}

	h.store.ExternalAccts.Set(id, ea)

	twincore.JSON(w, http.StatusOK, ea)
}

// DeleteExternalAccount handles DELETE /v1/accounts/{account_id}/external_accounts/{id}.
func (h *Handler) DeleteExternalAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if !h.store.ExternalAccts.Delete(id) {
		stripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such external account: '"+id+"'")
		return
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"id":      id,
		"object":  "bank_account",
		"deleted": true,
	})
}
