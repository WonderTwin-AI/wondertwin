package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-increase/internal/store"
)

// SimulateACHTransferSubmit handles POST /simulations/ach_transfers/{ach_transfer_id}/submit.
func (h *Handler) SimulateACHTransferSubmit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ach_transfer_id")
	transfer, ok := h.store.ACHTransfers.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No ACH transfer found for %s", id))
		return
	}

	if transfer.Status != store.ACHTransferStatusPendingSubmission {
		increaseError(w, http.StatusBadRequest, "invalid_operation", "ACH transfer is not pending submission")
		return
	}

	now := h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z")
	transfer.Status = store.ACHTransferStatusSubmitted
	transfer.Submission = &store.Submission{SubmittedAt: now}
	transfer.UpdatedAt = now

	h.store.ACHTransfers.Set(id, transfer)
	h.emitEvent("ach_transfer.submitted", id, "ach_transfer")

	twincore.JSON(w, http.StatusOK, transfer)
}

// SimulateACHTransferAcknowledge handles POST /simulations/ach_transfers/{ach_transfer_id}/acknowledge.
func (h *Handler) SimulateACHTransferAcknowledge(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ach_transfer_id")
	transfer, ok := h.store.ACHTransfers.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No ACH transfer found for %s", id))
		return
	}

	if transfer.Status != store.ACHTransferStatusSubmitted {
		increaseError(w, http.StatusBadRequest, "invalid_operation", "ACH transfer is not submitted")
		return
	}

	now := h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z")
	transfer.Status = store.ACHTransferStatusAcknowledged
	transfer.Acknowledgement = &store.Acknowledgement{AcknowledgedAt: now}
	transfer.UpdatedAt = now

	h.store.ACHTransfers.Set(id, transfer)
	h.emitEvent("ach_transfer.acknowledged", id, "ach_transfer")

	twincore.JSON(w, http.StatusOK, transfer)
}

// SimulateACHTransferSettle handles POST /simulations/ach_transfers/{ach_transfer_id}/settle.
func (h *Handler) SimulateACHTransferSettle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ach_transfer_id")
	transfer, ok := h.store.ACHTransfers.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No ACH transfer found for %s", id))
		return
	}

	if transfer.Status != store.ACHTransferStatusAcknowledged {
		increaseError(w, http.StatusBadRequest, "invalid_operation", "ACH transfer is not acknowledged")
		return
	}

	now := h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z")
	transfer.Status = store.ACHTransferStatusSettled
	transfer.UpdatedAt = now

	// Settle the held funds for debits
	if transfer.Amount < 0 {
		h.store.SettleHold(transfer.AccountID, -transfer.Amount)

		// Complete pending transaction
		pendings := h.store.PendingTransactions.Filter(func(pid string, pt store.PendingTransaction) bool {
			return pt.RouteID == id && pt.Status == "pending"
		})
		for _, pending := range pendings {
			pending.Status = "complete"
			pending.CompletedAt = now
			h.store.PendingTransactions.Set(pending.ID, pending)
		}
	} else {
		// Credit the account for incoming transfers
		h.store.CreditAccount(transfer.AccountID, transfer.Amount)
	}

	// Create settled transaction
	txnID := h.store.Transactions.NextID()
	txn := store.Transaction{
		ID:          txnID,
		Type:        "transaction",
		AccountID:   transfer.AccountID,
		Amount:      transfer.Amount,
		Currency:    "USD",
		Description: fmt.Sprintf("ACH transfer %s", id),
		RouteType:   "ach",
		RouteID:     id,
		CreatedAt:   now,
	}
	h.store.Transactions.Set(txnID, txn)
	transfer.TransactionID = txnID

	h.store.ACHTransfers.Set(id, transfer)
	h.emitEvent("ach_transfer.settled", id, "ach_transfer")

	twincore.JSON(w, http.StatusOK, transfer)
}

// SimulateACHTransferReturn handles POST /simulations/ach_transfers/{ach_transfer_id}/return.
func (h *Handler) SimulateACHTransferReturn(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ach_transfer_id")
	transfer, ok := h.store.ACHTransfers.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No ACH transfer found for %s", id))
		return
	}

	var req struct {
		ReturnReasonCode string `json:"return_reason_code,omitempty"`
	}
	parseJSONBody(r, &req)

	if req.ReturnReasonCode == "" {
		req.ReturnReasonCode = "R01" // Insufficient funds
	}

	now := h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z")
	transfer.Status = store.ACHTransferStatusReturned
	transfer.Return = &store.ACHReturn{
		ReturnReasonCode: req.ReturnReasonCode,
		ReturnedAt:       now,
	}
	transfer.UpdatedAt = now

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

	h.store.ACHTransfers.Set(id, transfer)
	h.emitEvent("ach_transfer.returned", id, "ach_transfer")

	twincore.JSON(w, http.StatusOK, transfer)
}

// SimulateInboundACHTransfer handles POST /simulations/inbound_ach_transfers.
func (h *Handler) SimulateInboundACHTransfer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID               string `json:"account_id"`
		Amount                  int64  `json:"amount"`
		OriginatorCompanyName   string `json:"originator_company_name,omitempty"`
		OriginatorCompanyEntry  string `json:"originator_company_entry_description,omitempty"`
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
	_, ok := h.store.Accounts.Get(req.AccountID)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No account found for %s", req.AccountID))
		return
	}

	id := h.store.InboundACHTransfers.NextID()
	now := h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z")

	inbound := store.InboundACHTransfer{
		ID:                                 id,
		Type:                               "inbound_ach_transfer",
		AccountID:                          req.AccountID,
		Amount:                             req.Amount,
		Currency:                           "USD",
		OriginatorCompanyName:              req.OriginatorCompanyName,
		OriginatorCompanyEntryDescription:  req.OriginatorCompanyEntry,
		Status:                             store.InboundACHTransferStatusPending,
		CreatedAt:                          now,
	}

	h.store.InboundACHTransfers.Set(id, inbound)
	h.emitEvent("inbound_ach_transfer.created", id, "inbound_ach_transfer")

	// Auto-accept and credit account
	inbound.Status = store.InboundACHTransferStatusAccepted
	h.store.CreditAccount(req.AccountID, req.Amount)

	// Create transaction
	txnID := h.store.Transactions.NextID()
	txn := store.Transaction{
		ID:          txnID,
		Type:        "transaction",
		AccountID:   req.AccountID,
		Amount:      req.Amount,
		Currency:    "USD",
		Description: fmt.Sprintf("Inbound ACH transfer %s", id),
		RouteType:   "ach",
		RouteID:     id,
		CreatedAt:   now,
	}
	h.store.Transactions.Set(txnID, txn)
	inbound.TransactionID = txnID

	h.store.InboundACHTransfers.Set(id, inbound)
	h.emitEvent("inbound_ach_transfer.accepted", id, "inbound_ach_transfer")

	twincore.JSON(w, http.StatusOK, inbound)
}
