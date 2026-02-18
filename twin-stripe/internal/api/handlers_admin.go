package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// fundRequest is the JSON body for POST /admin/accounts/{id}/fund.
type fundRequest struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// AdminFundAccount handles POST /admin/accounts/{id}/fund.
// Adds funds to a connected account's available balance.
func (h *Handler) AdminFundAccount(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")

	// Verify account exists
	if _, ok := h.store.Accounts.Get(accountID); !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such account: '"+accountID+"'")
		return
	}

	var req fundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	if req.Amount <= 0 {
		twincore.Error(w, http.StatusBadRequest, "amount must be positive")
		return
	}
	if req.Currency == "" {
		req.Currency = "usd"
	}

	h.store.CreditBalance(accountID, req.Currency, req.Amount)

	// Record balance transaction for admin funding
	h.store.RecordBalanceTransaction("adjustment", "", req.Currency, req.Amount, 0)

	balance := h.store.GetBalance(accountID)
	twincore.JSON(w, http.StatusOK, balance)
}
