package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-loyaltylion/internal/store"
)

// GetPoints handles GET /v2/customers/{merchant_id}/points.
func (h *Handler) GetPoints(w http.ResponseWriter, r *http.Request) {
	apiKey := getAPIKey(r)
	merchantID := chi.URLParam(r, "merchant_id")

	h.store.ProcessExpiredPoints(h.store.Clock.Now())

	c := h.store.GetCustomerByMerchantID(apiKey, merchantID)
	if c == nil {
		twincore.Error(w, http.StatusNotFound, "customer not found")
		return
	}

	twincore.JSON(w, http.StatusOK, map[string]int{
		"points_approved": c.PointsApproved,
		"points_pending":  c.PointsPending,
		"points_spent":    c.PointsSpent,
		"points_expired":  c.PointsExpired,
	})
}

// AddPoints handles POST /v2/customers/{merchant_id}/points.
func (h *Handler) AddPoints(w http.ResponseWriter, r *http.Request) {
	apiKey := getAPIKey(r)
	merchantID := chi.URLParam(r, "merchant_id")

	var req struct {
		Points int    `json:"points"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Points <= 0 {
		twincore.Error(w, http.StatusUnprocessableEntity, "points must be positive")
		return
	}

	c := h.store.GetCustomerByMerchantID(apiKey, merchantID)
	if c == nil {
		twincore.Error(w, http.StatusNotFound, "customer not found")
		return
	}

	now := h.store.Clock.Now()
	c.PointsApproved += req.Points
	c.UpdatedAt = now.Format(time.RFC3339)
	h.store.Customers.Set(store.CustomerKey(c.ID), *c)

	// Record transaction
	txnID := h.store.NextTransactionID()
	h.store.Transactions.Set(fmt.Sprintf("%d", txnID), store.PointsTransaction{
		ID:         txnID,
		CustomerID: c.ID,
		Type:       "earn",
		Amount:     req.Points,
		Reason:     req.Reason,
		Timestamp:  now.Format(time.RFC3339),
		APIKey:     apiKey,
	})

	twincore.JSON(w, http.StatusOK, map[string]int{
		"points_approved": c.PointsApproved,
		"points_pending":  c.PointsPending,
		"points_spent":    c.PointsSpent,
		"points_expired":  c.PointsExpired,
	})
}

// RemovePoints handles POST /v2/customers/{merchant_id}/points/remove.
func (h *Handler) RemovePoints(w http.ResponseWriter, r *http.Request) {
	apiKey := getAPIKey(r)
	merchantID := chi.URLParam(r, "merchant_id")

	var req struct {
		Points int    `json:"points"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Points <= 0 {
		twincore.Error(w, http.StatusUnprocessableEntity, "points must be positive")
		return
	}

	c := h.store.GetCustomerByMerchantID(apiKey, merchantID)
	if c == nil {
		twincore.Error(w, http.StatusNotFound, "customer not found")
		return
	}

	if c.PointsApproved < req.Points {
		twincore.Error(w, http.StatusUnprocessableEntity, "insufficient_points")
		return
	}

	now := h.store.Clock.Now()
	c.PointsApproved -= req.Points
	c.PointsSpent += req.Points
	c.UpdatedAt = now.Format(time.RFC3339)
	h.store.Customers.Set(store.CustomerKey(c.ID), *c)

	// Record transaction
	txnID := h.store.NextTransactionID()
	h.store.Transactions.Set(fmt.Sprintf("%d", txnID), store.PointsTransaction{
		ID:         txnID,
		CustomerID: c.ID,
		Type:       "spend",
		Amount:     req.Points,
		Reason:     req.Reason,
		Timestamp:  now.Format(time.RFC3339),
		APIKey:     apiKey,
	})

	twincore.JSON(w, http.StatusOK, map[string]int{
		"points_approved": c.PointsApproved,
		"points_pending":  c.PointsPending,
		"points_spent":    c.PointsSpent,
		"points_expired":  c.PointsExpired,
	})
}
