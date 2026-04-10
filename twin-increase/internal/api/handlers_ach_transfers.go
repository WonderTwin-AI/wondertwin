package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-increase/internal/store"
)

// CreateACHTransfer handles POST /ach_transfers.
func (h *Handler) CreateACHTransfer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID             string `json:"account_id"`
		Amount                int64  `json:"amount"`
		ExternalAccountID     string `json:"external_account_id,omitempty"`
		RoutingNumber         string `json:"routing_number,omitempty"`
		AccountNumber         string `json:"account_number,omitempty"`
		StatementDescriptor   string `json:"statement_descriptor,omitempty"`
		RequireApproval       bool   `json:"require_approval,omitempty"`
		CompanyName           string `json:"company_name,omitempty"`
		CompanyEntryDescription string `json:"company_entry_description,omitempty"`
		StandardEntryClassCode string `json:"standard_entry_class_code,omitempty"`
		Funding               string `json:"funding,omitempty"`
		IdempotencyKey        string `json:"idempotency_key,omitempty"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		increaseError(w, http.StatusBadRequest, "invalid_parameters", "Invalid JSON body")
		return
	}

	if req.AccountID == "" || req.Amount == 0 {
		increaseError(w, http.StatusBadRequest, "invalid_parameters", "account_id and amount are required")
		return
	}

	// Verify account exists
	acct, ok := h.store.Accounts.Get(req.AccountID)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No account found for %s", req.AccountID))
		return
	}

	// Check if funds available (for debits)
	if req.Amount < 0 && acct.AvailableBalance < -req.Amount {
		increaseError(w, http.StatusBadRequest, "insufficient_funds", "Insufficient funds in account")
		return
	}

	// Resolve external account details
	routingNum := req.RoutingNumber
	acctNum := req.AccountNumber
	if req.ExternalAccountID != "" {
		extAcct, ok := h.store.ExternalAccounts.Get(req.ExternalAccountID)
		if !ok {
			increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No external account found for %s", req.ExternalAccountID))
			return
		}
		routingNum = extAcct.RoutingNumber
		acctNum = extAcct.AccountNumber
	}

	id := h.store.ACHTransfers.NextID()
	now := h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z")

	funding := req.Funding
	if funding == "" {
		funding = "checking"
	}

	status := store.ACHTransferStatusPendingSubmission
	if req.RequireApproval {
		status = store.ACHTransferStatusPendingApproval
	}

	transfer := store.ACHTransfer{
		ID:                      id,
		Type:                    "ach_transfer",
		AccountID:               req.AccountID,
		ExternalAccountID:       req.ExternalAccountID,
		RoutingNumber:           routingNum,
		AccountNumber:           acctNum,
		Amount:                  req.Amount,
		Currency:                "USD",
		StatementDescriptor:     req.StatementDescriptor,
		CompanyName:             req.CompanyName,
		CompanyEntryDescription: req.CompanyEntryDescription,
		StandardEntryClassCode:  req.StandardEntryClassCode,
		Funding:                 funding,
		Status:                  status,
		CreatedAt:               now,
		UpdatedAt:               now,
		IdempotencyKey:          req.IdempotencyKey,
	}

	h.store.ACHTransfers.Set(id, transfer)

	// Hold funds immediately for debits
	if req.Amount < 0 {
		h.store.HoldFunds(req.AccountID, -req.Amount)

		// Create pending transaction
		pendingID := h.store.PendingTransactions.NextID()
		pending := store.PendingTransaction{
			ID:          pendingID,
			Type:        "pending_transaction",
			AccountID:   req.AccountID,
			Amount:      req.Amount,
			Currency:    "USD",
			Description: fmt.Sprintf("ACH transfer %s", id),
			RouteType:   "ach",
			RouteID:     id,
			Status:      "pending",
			CreatedAt:   now,
		}
		h.store.PendingTransactions.Set(pendingID, pending)
	}

	h.emitEvent("ach_transfer.created", id, "ach_transfer")

	twincore.JSON(w, http.StatusOK, transfer)
}

// GetACHTransfer handles GET /ach_transfers/{ach_transfer_id}.
func (h *Handler) GetACHTransfer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ach_transfer_id")
	transfer, ok := h.store.ACHTransfers.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No ACH transfer found for %s", id))
		return
	}
	twincore.JSON(w, http.StatusOK, transfer)
}

// ListACHTransfers handles GET /ach_transfers.
func (h *Handler) ListACHTransfers(w http.ResponseWriter, r *http.Request) {
	cursor, limit := paginationParams(r)
	accountID := r.URL.Query().Get("account_id")

	var page []store.ACHTransfer
	if accountID != "" {
		filtered := h.store.ACHTransfers.Filter(func(id string, t store.ACHTransfer) bool {
			return t.AccountID == accountID
		})
		page = filtered
	} else {
		page = h.store.ACHTransfers.Paginate(cursor, limit).Data
	}

	data := make([]any, len(page))
	for i, t := range page {
		data[i] = t
	}

	resp := listResponse(data, "")
	twincore.JSON(w, http.StatusOK, resp)
}

// ApproveACHTransfer handles POST /ach_transfers/{ach_transfer_id}/approve.
func (h *Handler) ApproveACHTransfer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ach_transfer_id")
	transfer, ok := h.store.ACHTransfers.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No ACH transfer found for %s", id))
		return
	}

	if transfer.Status != store.ACHTransferStatusPendingApproval {
		increaseError(w, http.StatusBadRequest, "invalid_operation", "ACH transfer is not pending approval")
		return
	}

	now := h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z")
	transfer.Status = store.ACHTransferStatusPendingSubmission
	transfer.Approval = &store.Approval{
		ApprovedAt: now,
		ApprovedBy: "api",
	}
	transfer.UpdatedAt = now

	h.store.ACHTransfers.Set(id, transfer)
	h.emitEvent("ach_transfer.approved", id, "ach_transfer")

	twincore.JSON(w, http.StatusOK, transfer)
}

// CancelACHTransfer handles POST /ach_transfers/{ach_transfer_id}/cancel.
func (h *Handler) CancelACHTransfer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ach_transfer_id")
	transfer, ok := h.store.ACHTransfers.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No ACH transfer found for %s", id))
		return
	}

	if transfer.Status != store.ACHTransferStatusPendingApproval && transfer.Status != store.ACHTransferStatusPendingSubmission {
		increaseError(w, http.StatusBadRequest, "invalid_operation", "ACH transfer cannot be canceled in current state")
		return
	}

	now := h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z")
	transfer.Status = store.ACHTransferStatusCanceled
	transfer.UpdatedAt = now

	h.store.ACHTransfers.Set(id, transfer)

	// Release held funds for debits
	if transfer.Amount < 0 {
		h.store.ReleaseFunds(transfer.AccountID, -transfer.Amount)

		// Complete pending transaction
		pendings := h.store.PendingTransactions.Filter(func(pid string, pt store.PendingTransaction) bool {
			return pt.RouteID == id && pt.Status == "pending"
		})
		for _, pending := range pendings {
			pending.Status = "complete"
			pending.CompletedAt = now
			h.store.PendingTransactions.Set(pending.ID, pending)
		}
	}

	h.emitEvent("ach_transfer.canceled", id, "ach_transfer")

	twincore.JSON(w, http.StatusOK, transfer)
}
