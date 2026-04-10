package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-increase/internal/store"
)

// CreateExternalAccount handles POST /external_accounts.
func (h *Handler) CreateExternalAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RoutingNumber  string `json:"routing_number"`
		AccountNumber  string `json:"account_number"`
		AccountHolder  string `json:"account_holder_name,omitempty"`
		Description    string `json:"description,omitempty"`
		Funding        string `json:"funding,omitempty"`
		IdempotencyKey string `json:"idempotency_key,omitempty"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		increaseError(w, http.StatusBadRequest, "invalid_parameters", "Invalid JSON body")
		return
	}

	if req.RoutingNumber == "" || req.AccountNumber == "" {
		increaseError(w, http.StatusBadRequest, "invalid_parameters", "routing_number and account_number are required")
		return
	}

	id := h.store.ExternalAccounts.NextID()
	now := h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z")

	funding := req.Funding
	if funding == "" {
		funding = "checking"
	}

	extAcct := store.ExternalAccount{
		ID:                 id,
		Type:               "external_account",
		RoutingNumber:      req.RoutingNumber,
		AccountNumber:      req.AccountNumber,
		AccountHolder:      req.AccountHolder,
		Description:        req.Description,
		Status:             store.ExternalAccountStatusActive,
		Funding:            funding,
		VerificationStatus: "unverified",
		CreatedAt:          now,
		IdempotencyKey:     req.IdempotencyKey,
	}

	h.store.ExternalAccounts.Set(id, extAcct)
	h.emitEvent("external_account.created", id, "external_account")

	twincore.JSON(w, http.StatusOK, extAcct)
}

// GetExternalAccount handles GET /external_accounts/{external_account_id}.
func (h *Handler) GetExternalAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "external_account_id")
	extAcct, ok := h.store.ExternalAccounts.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No external account found for %s", id))
		return
	}
	twincore.JSON(w, http.StatusOK, extAcct)
}

// UpdateExternalAccount handles PATCH /external_accounts/{external_account_id}.
func (h *Handler) UpdateExternalAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "external_account_id")
	extAcct, ok := h.store.ExternalAccounts.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No external account found for %s", id))
		return
	}

	var req struct {
		Description string `json:"description,omitempty"`
		Status      string `json:"status,omitempty"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		increaseError(w, http.StatusBadRequest, "invalid_parameters", "Invalid JSON body")
		return
	}

	if req.Description != "" {
		extAcct.Description = req.Description
	}
	if req.Status != "" {
		extAcct.Status = req.Status
	}

	h.store.ExternalAccounts.Set(id, extAcct)
	h.emitEvent("external_account.updated", id, "external_account")

	twincore.JSON(w, http.StatusOK, extAcct)
}

// ListExternalAccounts handles GET /external_accounts.
func (h *Handler) ListExternalAccounts(w http.ResponseWriter, r *http.Request) {
	cursor, limit := paginationParams(r)

	page := h.store.ExternalAccounts.Paginate(cursor, limit)

	data := make([]any, len(page.Data))
	for i, ea := range page.Data {
		data[i] = ea
	}

	resp := listResponse(data, page.Cursor)
	twincore.JSON(w, http.StatusOK, resp)
}
