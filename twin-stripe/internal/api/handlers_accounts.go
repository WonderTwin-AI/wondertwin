package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// CreateAccount handles POST /v1/accounts.
// Stripe SDK: account.New(params)
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", err.Error())
		return
	}

	id := h.store.Accounts.NextID()
	now := store.Now()

	acctType := r.FormValue("type")
	if acctType == "" {
		acctType = "custom"
	}

	country := r.FormValue("country")
	if country == "" {
		country = "US"
	}

	currency := r.FormValue("default_currency")
	if currency == "" {
		currency = "usd"
	}

	acct := store.Account{
		ID:               id,
		Object:           "account",
		Type:             acctType,
		BusinessType:     r.FormValue("business_type"),
		Email:            r.FormValue("email"),
		Country:          country,
		DefaultCurrency:  currency,
		ChargesEnabled:   false,
		PayoutsEnabled:   false,
		DetailsSubmitted: false,
		Capabilities:     make(map[string]string),
		Requirements: &store.Requirements{
			CurrentlyDue:  []string{"business_type", "external_account", "tos_acceptance.date", "tos_acceptance.ip"},
			EventuallyDue: []string{"business_type", "external_account", "tos_acceptance.date", "tos_acceptance.ip"},
			PastDue:       []string{},
		},
		Metadata: extractMetadata(r),
		Created:  now,
	}

	// Parse capabilities from form
	for key, values := range r.Form {
		if strings.HasPrefix(key, "capabilities[") && strings.HasSuffix(key, "][requested]") {
			capName := strings.TrimPrefix(key, "capabilities[")
			capName = strings.TrimSuffix(capName, "][requested]")
			if len(values) > 0 && values[0] == "true" {
				acct.Capabilities[capName] = "inactive"
			}
		}
	}

	// Parse business_profile
	if r.FormValue("business_profile[url]") != "" || r.FormValue("business_profile[mcc]") != "" {
		acct.BusinessProfile = map[string]any{
			"url": r.FormValue("business_profile[url]"),
			"mcc": r.FormValue("business_profile[mcc]"),
		}
	}

	// Parse tos_acceptance
	if r.FormValue("tos_acceptance[date]") != "" || r.FormValue("tos_acceptance[ip]") != "" {
		acct.TOSAcceptance = map[string]any{
			"date": r.FormValue("tos_acceptance[date]"),
			"ip":   r.FormValue("tos_acceptance[ip]"),
		}
	}

	acct.ExternalAccounts = &store.ExternalAccounts{
		Object:  "list",
		Data:    []store.ExternalAccount{},
		HasMore: false,
		URL:     "/v1/accounts/" + id + "/external_accounts",
	}

	h.store.Accounts.Set(id, acct)
	h.store.GetOrCreateBalance(id)

	// Emit account.updated event
	h.emitEvent("account.updated", accountToMap(acct))

	twincore.JSON(w, http.StatusOK, acct)
}

// GetAccount handles GET /v1/accounts/{id}.
// Stripe SDK: account.GetByID(id, nil)
func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	acct, ok := h.store.Accounts.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such account: '"+id+"'")
		return
	}

	// Attach external accounts
	acct.ExternalAccounts = h.getExternalAccountsForAccount(id)

	twincore.JSON(w, http.StatusOK, acct)
}

// UpdateAccount handles POST /v1/accounts/{id}.
// Stripe SDK: account.Update(id, params)
func (h *Handler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	acct, ok := h.store.Accounts.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such account: '"+id+"'")
		return
	}

	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", err.Error())
		return
	}

	// Update fields from form
	if v := r.FormValue("email"); v != "" {
		acct.Email = v
	}
	if v := r.FormValue("business_type"); v != "" {
		acct.BusinessType = v
	}

	// Parse business_profile updates
	for key, values := range r.Form {
		if strings.HasPrefix(key, "business_profile[") && len(values) > 0 {
			if acct.BusinessProfile == nil {
				acct.BusinessProfile = make(map[string]any)
			}
			field := strings.TrimPrefix(key, "business_profile[")
			field = strings.TrimSuffix(field, "]")
			acct.BusinessProfile[field] = values[0]
		}
	}

	// Parse individual updates (KYB fields)
	for key, values := range r.Form {
		if strings.HasPrefix(key, "individual[") && len(values) > 0 {
			if acct.Individual == nil {
				acct.Individual = make(map[string]any)
			}
			field := strings.TrimPrefix(key, "individual[")
			field = strings.TrimSuffix(field, "]")
			acct.Individual[field] = values[0]
		}
	}

	// Parse company updates
	for key, values := range r.Form {
		if strings.HasPrefix(key, "company[") && len(values) > 0 {
			if acct.Company == nil {
				acct.Company = make(map[string]any)
			}
			field := strings.TrimPrefix(key, "company[")
			field = strings.TrimSuffix(field, "]")
			acct.Company[field] = values[0]
		}
	}

	// Parse tos_acceptance
	if r.FormValue("tos_acceptance[date]") != "" || r.FormValue("tos_acceptance[ip]") != "" {
		acct.TOSAcceptance = map[string]any{
			"date": r.FormValue("tos_acceptance[date]"),
			"ip":   r.FormValue("tos_acceptance[ip]"),
		}
	}

	// Parse capability requests
	for key, values := range r.Form {
		if strings.HasPrefix(key, "capabilities[") && strings.HasSuffix(key, "][requested]") {
			capName := strings.TrimPrefix(key, "capabilities[")
			capName = strings.TrimSuffix(capName, "][requested]")
			if len(values) > 0 && values[0] == "true" {
				if acct.Capabilities == nil {
					acct.Capabilities = make(map[string]string)
				}
				// If not already active, set to inactive (pending verification)
				if _, exists := acct.Capabilities[capName]; !exists {
					acct.Capabilities[capName] = "inactive"
				}
			}
		}
	}

	// Update metadata
	for key, values := range r.Form {
		if strings.HasPrefix(key, "metadata[") && len(values) > 0 {
			if acct.Metadata == nil {
				acct.Metadata = make(map[string]string)
			}
			field := strings.TrimPrefix(key, "metadata[")
			field = strings.TrimSuffix(field, "]")
			if values[0] == "" {
				delete(acct.Metadata, field)
			} else {
				acct.Metadata[field] = values[0]
			}
		}
	}

	// Simulate KYB verification progress:
	// If business_type, tos_acceptance, and some individual/company info is provided,
	// mark requirements as satisfied and enable capabilities.
	if acct.BusinessType != "" && acct.TOSAcceptance != nil {
		hasDetails := (acct.Individual != nil && len(acct.Individual) > 0) ||
			(acct.Company != nil && len(acct.Company) > 0)
		if hasDetails {
			acct.DetailsSubmitted = true
			acct.Requirements.CurrentlyDue = []string{}
			acct.Requirements.EventuallyDue = []string{}
			// Enable capabilities
			for cap := range acct.Capabilities {
				acct.Capabilities[cap] = "active"
			}
			acct.ChargesEnabled = true
			acct.PayoutsEnabled = true
		}
	}

	acct.Updated = store.Now()
	h.store.Accounts.Set(id, acct)

	// Emit account.updated event
	h.emitEvent("account.updated", accountToMap(acct))

	acct.ExternalAccounts = h.getExternalAccountsForAccount(id)
	twincore.JSON(w, http.StatusOK, acct)
}

// DeleteAccount handles DELETE /v1/accounts/{id}.
func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if !h.store.Accounts.Delete(id) {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such account: '"+id+"'")
		return
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"id":      id,
		"object":  "account",
		"deleted": true,
	})
}

// ListAccounts handles GET /v1/accounts.
func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	page := h.store.Accounts.Paginate(cursor, limit)

	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/accounts",
		"data":     page.Data,
		"has_more": page.HasMore,
	})
}

// getExternalAccountsForAccount returns the external accounts list for an account.
func (h *Handler) getExternalAccountsForAccount(accountID string) *store.ExternalAccounts {
	extAccts := h.store.ExternalAccts.Filter(func(id string, ea store.ExternalAccount) bool {
		return ea.AccountID == accountID
	})
	return &store.ExternalAccounts{
		Object:  "list",
		Data:    extAccts,
		HasMore: false,
		URL:     "/v1/accounts/" + accountID + "/external_accounts",
	}
}

// extractMetadata extracts metadata[key]=value from form data.
func extractMetadata(r *http.Request) map[string]string {
	meta := make(map[string]string)
	for key, values := range r.Form {
		if strings.HasPrefix(key, "metadata[") && strings.HasSuffix(key, "]") && len(values) > 0 {
			field := strings.TrimPrefix(key, "metadata[")
			field = strings.TrimSuffix(field, "]")
			meta[field] = values[0]
		}
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

// accountToMap converts an Account to a map for event payloads.
func accountToMap(acct store.Account) map[string]any {
	// Quick and dirty: marshal to JSON and back to map
	data, _ := json.Marshal(acct)
	var m map[string]any
	json.Unmarshal(data, &m)
	return m
}

