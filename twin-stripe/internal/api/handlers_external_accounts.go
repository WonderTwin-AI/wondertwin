package api

import (
	"crypto/sha256"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// CreateExternalAccount handles POST /v1/accounts/{account_id}/external_accounts.
func (h *Handler) CreateExternalAccount(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "account_id")

	if _, ok := h.store.Accounts.Get(accountID); !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such account: '"+accountID+"'")
		return
	}

	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", err.Error())
		return
	}

	id := h.store.ExternalAccts.NextID()

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
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(routingNumber+accountNumber)))[:16]

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
		Metadata:           extractMetadata(r),
	}

	h.store.ExternalAccts.Set(id, ea)

	twincore.JSON(w, http.StatusOK, ea)
}

// GetExternalAccount handles GET /v1/accounts/{account_id}/external_accounts/{id}.
func (h *Handler) GetExternalAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ea, ok := h.store.ExternalAccts.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound,
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
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such external account: '"+id+"'")
		return
	}

	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", err.Error())
		return
	}

	if v := r.FormValue("default_for_currency"); v == "true" {
		ea.DefaultForCurrency = true
	}

	// Update metadata
	for key, values := range r.Form {
		if len(values) > 0 && len(key) > 9 && key[:9] == "metadata[" {
			if ea.Metadata == nil {
				ea.Metadata = make(map[string]string)
			}
			field := key[9 : len(key)-1]
			ea.Metadata[field] = values[0]
		}
	}

	h.store.ExternalAccts.Set(id, ea)

	twincore.JSON(w, http.StatusOK, ea)
}

// DeleteExternalAccount handles DELETE /v1/accounts/{account_id}/external_accounts/{id}.
func (h *Handler) DeleteExternalAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if !h.store.ExternalAccts.Delete(id) {
		twincore.StripeError(w, http.StatusNotFound,
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
