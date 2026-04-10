package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-increase/internal/store"
)

// CreateAccount handles POST /accounts.
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             string `json:"name"`
		EntityID         string `json:"entity_id,omitempty"`
		ProgramID        string `json:"program_id,omitempty"`
		IdempotencyKey   string `json:"idempotency_key,omitempty"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		increaseError(w, http.StatusBadRequest, "invalid_parameters", "Invalid JSON body")
		return
	}

	if req.Name == "" {
		increaseError(w, http.StatusBadRequest, "invalid_parameters", "name is required")
		return
	}

	id := h.store.Accounts.NextID()
	now := h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z")

	// Default program if not specified
	programID := req.ProgramID
	if programID == "" {
		programID = "program_default"
	}

	acct := store.Account{
		ID:               id,
		Type:             "account",
		Name:             req.Name,
		EntityID:         req.EntityID,
		ProgramID:        programID,
		Status:           store.AccountStatusOpen,
		Bank:             "grasshopper_bank",
		Currency:         "USD",
		CurrentBalance:   0,
		AvailableBalance: 0,
		CreatedAt:        now,
		IdempotencyKey:   req.IdempotencyKey,
	}

	h.store.Accounts.Set(id, acct)
	h.emitEvent("account.created", id, "account")

	twincore.JSON(w, http.StatusOK, acct)
}

// GetAccount handles GET /accounts/{account_id}.
func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "account_id")
	acct, ok := h.store.Accounts.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No account found for %s", id))
		return
	}
	twincore.JSON(w, http.StatusOK, acct)
}

// UpdateAccount handles PATCH /accounts/{account_id}.
func (h *Handler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "account_id")
	acct, ok := h.store.Accounts.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No account found for %s", id))
		return
	}

	var req struct {
		Name string `json:"name,omitempty"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		increaseError(w, http.StatusBadRequest, "invalid_parameters", "Invalid JSON body")
		return
	}

	if req.Name != "" {
		acct.Name = req.Name
	}

	h.store.Accounts.Set(id, acct)
	h.emitEvent("account.updated", id, "account")

	twincore.JSON(w, http.StatusOK, acct)
}

// ListAccounts handles GET /accounts.
func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	cursor, limit := paginationParams(r)

	page := h.store.Accounts.Paginate(cursor, limit)

	data := make([]any, len(page.Data))
	for i, acct := range page.Data {
		data[i] = acct
	}

	resp := listResponse(data, page.Cursor)
	twincore.JSON(w, http.StatusOK, resp)
}

// CloseAccount handles POST /accounts/{account_id}/close.
func (h *Handler) CloseAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "account_id")
	acct, ok := h.store.Accounts.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No account found for %s", id))
		return
	}

	if acct.Status == store.AccountStatusClosed {
		increaseError(w, http.StatusBadRequest, "invalid_operation", "Account is already closed")
		return
	}

	// Check if balance is zero
	if acct.CurrentBalance != 0 {
		increaseError(w, http.StatusBadRequest, "invalid_operation", "Cannot close account with non-zero balance")
		return
	}

	acct.Status = store.AccountStatusClosed
	acct.ClosedAt = h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z")
	h.store.Accounts.Set(id, acct)
	h.emitEvent("account.closed", id, "account")

	twincore.JSON(w, http.StatusOK, acct)
}
