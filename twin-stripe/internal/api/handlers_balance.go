package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
)

// GetBalance handles GET /v1/balance.
// Stripe SDK: balance.Get(params)
// Uses Stripe-Account header to determine which account's balance to return.
func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	accountID := stripeAccountFromRequest(r)
	balance := h.store.GetBalance(accountID)
	twincore.JSON(w, http.StatusOK, balance)
}
