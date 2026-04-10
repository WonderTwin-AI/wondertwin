package api

import (
	"fmt"
	"math/rand"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-increase/internal/store"
)

// CreateAccountNumber handles POST /account_numbers.
func (h *Handler) CreateAccountNumber(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID      string `json:"account_id"`
		Name           string `json:"name"`
		IdempotencyKey string `json:"idempotency_key,omitempty"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		increaseError(w, http.StatusBadRequest, "invalid_parameters", "Invalid JSON body")
		return
	}

	if req.AccountID == "" {
		increaseError(w, http.StatusBadRequest, "invalid_parameters", "account_id is required")
		return
	}

	// Verify account exists
	_, ok := h.store.Accounts.Get(req.AccountID)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No account found for %s", req.AccountID))
		return
	}

	id := h.store.AccountNumbers.NextID()
	now := h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z")

	// Generate realistic routing/account numbers
	routingNumber := "021000021" // Simulated routing number
	accountNumber := fmt.Sprintf("%010d", rand.Int63n(10000000000))

	acctNum := store.AccountNumber{
		ID:             id,
		Type:           "account_number",
		AccountID:      req.AccountID,
		AccountNumber:  accountNumber,
		RoutingNumber:  routingNumber,
		Name:           req.Name,
		Status:         store.AccountNumberStatusActive,
		CreatedAt:      now,
		IdempotencyKey: req.IdempotencyKey,
	}

	h.store.AccountNumbers.Set(id, acctNum)
	h.emitEvent("account_number.created", id, "account_number")

	twincore.JSON(w, http.StatusOK, acctNum)
}

// GetAccountNumber handles GET /account_numbers/{account_number_id}.
func (h *Handler) GetAccountNumber(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "account_number_id")
	acctNum, ok := h.store.AccountNumbers.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No account number found for %s", id))
		return
	}
	twincore.JSON(w, http.StatusOK, acctNum)
}

// UpdateAccountNumber handles PATCH /account_numbers/{account_number_id}.
func (h *Handler) UpdateAccountNumber(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "account_number_id")
	acctNum, ok := h.store.AccountNumbers.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No account number found for %s", id))
		return
	}

	var req struct {
		Name   string `json:"name,omitempty"`
		Status string `json:"status,omitempty"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		increaseError(w, http.StatusBadRequest, "invalid_parameters", "Invalid JSON body")
		return
	}

	if req.Name != "" {
		acctNum.Name = req.Name
	}
	if req.Status != "" {
		acctNum.Status = req.Status
	}

	h.store.AccountNumbers.Set(id, acctNum)
	h.emitEvent("account_number.updated", id, "account_number")

	twincore.JSON(w, http.StatusOK, acctNum)
}

// ListAccountNumbers handles GET /account_numbers.
func (h *Handler) ListAccountNumbers(w http.ResponseWriter, r *http.Request) {
	cursor, limit := paginationParams(r)
	accountID := r.URL.Query().Get("account_id")

	var page []store.AccountNumber
	if accountID != "" {
		filtered := h.store.AccountNumbers.Filter(func(id string, an store.AccountNumber) bool {
			return an.AccountID == accountID
		})
		page = filtered
	} else {
		page = h.store.AccountNumbers.Paginate(cursor, limit).Data
	}

	data := make([]any, len(page))
	for i, an := range page {
		data[i] = an
	}

	resp := listResponse(data, "")
	twincore.JSON(w, http.StatusOK, resp)
}
