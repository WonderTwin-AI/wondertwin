package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// CreateExternalAccount handles POST /v1/accounts/{account_id}/external_accounts.
func (h *Handler) CreateExternalAccount(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "account_id")

	acct, ok := h.store.Accounts.Get(accountID)
	if !ok {
		stripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such account: '"+accountID+"'")
		return
	}

	if err := parseFormOrJSON(r); err != nil {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", err.Error())
		return
	}

	// Stripe requires the account to attach (quirk crud-012); bank_account is
	// the spec's other name for it.
	if !hasExternalAccount(r) && !hasBankAccountParams(r) {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing",
			"Missing required param: external_account.")
		return
	}

	ea, ok := h.externalAccountFromRequest(w, r, accountID, true)
	if !ok {
		return
	}
	ea.Metadata = parseMetadata(r)
	h.store.ExternalAccts.Set(ea.ID, ea)
	dropExternalAccountRequirement(acct.Requirements)
	h.store.Accounts.Set(accountID, acct)
	twincore.JSON(w, http.StatusOK, ea)
}

// dropExternalAccountRequirement removes external_account from the
// account's outstanding requirements once a bank account is attached.
func dropExternalAccountRequirement(req *store.Requirements) {
	if req == nil {
		return
	}
	without := func(fields []string) []string {
		out := []string{}
		for _, f := range fields {
			if f != "external_account" {
				out = append(out, f)
			}
		}
		return out
	}
	req.CurrentlyDue = without(req.CurrentlyDue)
	req.EventuallyDue = without(req.EventuallyDue)
	req.PastDue = without(req.PastDue)
}

// hasExternalAccount reports whether the request carries external_account,
// as a token or as a dictionary.
func hasExternalAccount(r *http.Request) bool {
	if r.FormValue("external_account") != "" {
		return true
	}
	for k := range r.Form {
		if strings.HasPrefix(k, "external_account[") {
			return true
		}
	}
	return false
}

// externalAccountFromRequest builds the bank account external_account
// describes: a token resolved by resolveBankToken, or the dictionary form.
// flatFallback also reads each field from bank_account[...] and then from
// the top level, which only the external accounts route does; on an
// account route those names are the account's own. On refusal it has
// written Stripe's error and returns false.
func (h *Handler) externalAccountFromRequest(w http.ResponseWriter, r *http.Request, accountID string, flatFallback bool) (store.ExternalAccount, bool) {
	// A token arrives as the plain string form of external_account; the
	// dictionary form arrives as external_account[...] fields.
	if token := r.FormValue("external_account"); token != "" {
		bank, ok := h.resolveBankToken(w, token)
		if !ok {
			return store.ExternalAccount{}, false
		}
		return store.ExternalAccount{
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
		}, true
	}

	field := func(name, fallback string) string {
		if v := r.FormValue("external_account[" + name + "]"); v != "" {
			return v
		}
		if flatFallback {
			if v := r.FormValue("bank_account[" + name + "]"); v != "" {
				return v
			}
			if v := r.FormValue(name); v != "" {
				return v
			}
		}
		return fallback
	}
	routingNumber := field("routing_number", "")
	accountNumber := field("account_number", "")
	last4 := "0000"
	if len(accountNumber) >= 4 {
		last4 = accountNumber[len(accountNumber)-4:]
	}
	return store.ExternalAccount{
		ID:                 h.store.StripeID(h.store.ExternalAccts.NextID()),
		Object:             "bank_account",
		AccountID:          accountID,
		AccountHolderName:  field("account_holder_name", ""),
		BankName:           testBankName,
		Country:            field("country", "US"),
		Currency:           field("currency", "usd"),
		Last4:              last4,
		RoutingNumber:      routingNumber,
		Status:             "new",
		DefaultForCurrency: true,
		Fingerprint:        bankFingerprint(routingNumber, accountNumber),
	}, true
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
