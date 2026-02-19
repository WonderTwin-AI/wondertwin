package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// CreateTransfer handles POST /v1/transfers.
// Stripe SDK: transfer.New(params)
func (h *Handler) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", err.Error())
		return
	}

	amountStr := r.FormValue("amount")
	if amountStr == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing",
			"Missing required param: amount.")
		return
	}
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_invalid",
			"Invalid integer: "+amountStr)
		return
	}

	destination := r.FormValue("destination")
	if destination == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing",
			"Missing required param: destination.")
		return
	}

	// Verify destination account exists
	if _, ok := h.store.Accounts.Get(destination); !ok {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "resource_missing",
			"No such account: '"+destination+"'")
		return
	}

	currency := r.FormValue("currency")
	if currency == "" {
		currency = "usd"
	}

	id := h.store.Transfers.NextID()
	transfer := store.Transfer{
		ID:                id,
		Object:            "transfer",
		Amount:            amount,
		Currency:          currency,
		Description:       r.FormValue("description"),
		Destination:       destination,
		TransferGroup:     r.FormValue("transfer_group"),
		SourceTransaction: r.FormValue("source_transaction"),
		Metadata:          extractMetadata(r),
		Livemode:          false,
		Created:           store.Now(),
	}

	h.store.Transfers.Set(id, transfer)

	// Credit the destination account balance
	h.store.CreditBalance(destination, currency, amount)

	// Debit the platform balance (transfers move funds platform -> connected account)
	if err := h.store.DebitBalance("", currency, amount); err != nil {
		slog.Warn("platform balance insufficient for transfer", "amount", amount, "error", err)
	}

	// Record balance transactions
	h.store.RecordBalanceTransaction("transfer", transfer.ID, currency, -amount, 0) // platform debit
	h.store.RecordBalanceTransaction("transfer", transfer.ID, currency, amount, 0)  // destination credit

	// Emit transfer.created event
	h.emitEvent("transfer.created", transferToMap(transfer))

	// Emit transfer.paid event (in sim, transfers complete instantly)
	h.emitEvent("transfer.paid", transferToMap(transfer))

	twincore.JSON(w, http.StatusOK, transfer)
}

// GetTransfer handles GET /v1/transfers/{id}.
// Stripe SDK: transfer.Get(id, nil)
func (h *Handler) GetTransfer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	transfer, ok := h.store.Transfers.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such transfer: '"+id+"'")
		return
	}

	twincore.JSON(w, http.StatusOK, transfer)
}

// ListTransfers handles GET /v1/transfers.
func (h *Handler) ListTransfers(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	page := h.store.Transfers.Paginate(cursor, limit)

	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/transfers",
		"data":     page.Data,
		"has_more": page.HasMore,
	})
}

func transferToMap(t store.Transfer) map[string]any {
	data, _ := json.Marshal(t)
	var m map[string]any
	json.Unmarshal(data, &m)
	return m
}
