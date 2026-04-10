package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-increase/internal/store"
)

// GetInboundACHTransfer handles GET /inbound_ach_transfers/{inbound_ach_transfer_id}.
func (h *Handler) GetInboundACHTransfer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "inbound_ach_transfer_id")
	transfer, ok := h.store.InboundACHTransfers.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No inbound ACH transfer found for %s", id))
		return
	}
	twincore.JSON(w, http.StatusOK, transfer)
}

// ListInboundACHTransfers handles GET /inbound_ach_transfers.
func (h *Handler) ListInboundACHTransfers(w http.ResponseWriter, r *http.Request) {
	cursor, limit := paginationParams(r)
	accountID := r.URL.Query().Get("account_id")

	var page []store.InboundACHTransfer
	if accountID != "" {
		filtered := h.store.InboundACHTransfers.Filter(func(id string, t store.InboundACHTransfer) bool {
			return t.AccountID == accountID
		})
		page = filtered
	} else {
		page = h.store.InboundACHTransfers.Paginate(cursor, limit).Data
	}

	data := make([]any, len(page))
	for i, t := range page {
		data[i] = t
	}

	resp := listResponse(data, "")
	twincore.JSON(w, http.StatusOK, resp)
}

// DeclineInboundACHTransfer handles POST /inbound_ach_transfers/{inbound_ach_transfer_id}/decline.
func (h *Handler) DeclineInboundACHTransfer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "inbound_ach_transfer_id")
	transfer, ok := h.store.InboundACHTransfers.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No inbound ACH transfer found for %s", id))
		return
	}

	if transfer.Status != store.InboundACHTransferStatusPending {
		increaseError(w, http.StatusBadRequest, "invalid_operation", "Inbound ACH transfer is not pending")
		return
	}

	now := h.store.Clock.Now().Format("2006-01-02T15:04:05.000Z")
	transfer.Status = store.InboundACHTransferStatusDeclined
	transfer.DeclinedAt = now

	h.store.InboundACHTransfers.Set(id, transfer)
	h.emitEvent("inbound_ach_transfer.declined", id, "inbound_ach_transfer")

	twincore.JSON(w, http.StatusOK, transfer)
}

// NotificationOfChangeInboundACHTransfer handles POST /inbound_ach_transfers/{inbound_ach_transfer_id}/notification_of_change.
func (h *Handler) NotificationOfChangeInboundACHTransfer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "inbound_ach_transfer_id")
	transfer, ok := h.store.InboundACHTransfers.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No inbound ACH transfer found for %s", id))
		return
	}

	// NOC can be sent after acceptance
	h.emitEvent("inbound_ach_transfer.notification_of_change", id, "inbound_ach_transfer")

	twincore.JSON(w, http.StatusOK, transfer)
}

// ReturnInboundACHTransfer handles POST /inbound_ach_transfers/{inbound_ach_transfer_id}/transfer_return.
func (h *Handler) ReturnInboundACHTransfer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "inbound_ach_transfer_id")
	transfer, ok := h.store.InboundACHTransfers.Get(id)
	if !ok {
		increaseError(w, http.StatusNotFound, "object_not_found", fmt.Sprintf("No inbound ACH transfer found for %s", id))
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
	transfer.Status = store.InboundACHTransferStatusReturned
	transfer.ReturnedAt = now
	transfer.ReturnReasonCode = req.ReturnReasonCode

	h.store.InboundACHTransfers.Set(id, transfer)

	// Reverse the credit if it was already accepted
	if transfer.TransactionID != "" {
		h.store.DebitAccount(transfer.AccountID, transfer.Amount)

		// Create reversal transaction
		txnID := h.store.Transactions.NextID()
		txn := store.Transaction{
			ID:          txnID,
			Type:        "transaction",
			AccountID:   transfer.AccountID,
			Amount:      -transfer.Amount,
			Currency:    "USD",
			Description: fmt.Sprintf("ACH return %s", id),
			RouteType:   "ach",
			RouteID:     id,
			CreatedAt:   now,
		}
		h.store.Transactions.Set(txnID, txn)
	}

	h.emitEvent("inbound_ach_transfer.returned", id, "inbound_ach_transfer")

	twincore.JSON(w, http.StatusOK, transfer)
}
