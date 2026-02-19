package api

import (
	"encoding/json"
	"math"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-smile/internal/store"
)

// GetCustomer handles GET /v1/customers/{id}.
func (h *Handler) GetCustomer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := h.store.Customers.Get(id)
	if !ok {
		twincore.Error(w, http.StatusNotFound, "customer not found")
		return
	}
	twincore.JSON(w, http.StatusOK, c)
}

// RedeemPoints handles POST /v1/points/redeem.
func (h *Handler) RedeemPoints(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CustomerID     string `json:"customer_id"`
		Points         int64  `json:"points"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.CustomerID == "" {
		twincore.Error(w, http.StatusUnprocessableEntity, "customer_id is required")
		return
	}
	if req.Points <= 0 {
		twincore.Error(w, http.StatusUnprocessableEntity, "points must be a positive integer")
		return
	}

	// Idempotency check: if a redemption with this key already exists, return it
	if req.IdempotencyKey != "" {
		if existing := h.store.FindRedemptionByIdempotencyKey(req.IdempotencyKey); existing != nil {
			twincore.JSON(w, http.StatusOK, existing)
			return
		}
	}

	// Validate customer exists
	c, ok := h.store.Customers.Get(req.CustomerID)
	if !ok {
		twincore.Error(w, http.StatusNotFound, "customer not found")
		return
	}

	// Validate sufficient points
	if c.PointsBalance < req.Points {
		twincore.Error(w, http.StatusUnprocessableEntity, "insufficient points balance")
		return
	}

	// Calculate value_cents = points * 100 / points_per_dollar
	ppd := c.PointsPerDollar
	if ppd <= 0 {
		ppd = 100 // default: 100 points = $1
	}
	valueCents := int64(math.Round(float64(req.Points) * 100 / ppd))

	// Debit points from customer
	now := h.store.Clock.Now().Unix()
	c.PointsBalance -= req.Points
	c.UpdatedAt = now
	h.store.Customers.Set(req.CustomerID, c)

	// Create redemption record
	redID := h.store.Redemptions.NextID()
	redemption := store.Redemption{
		ID:             redID,
		CustomerID:     req.CustomerID,
		Points:         req.Points,
		ValueCents:     valueCents,
		Status:         "completed",
		IdempotencyKey: req.IdempotencyKey,
		CreatedAt:      now,
	}
	h.store.Redemptions.Set(redID, redemption)

	twincore.JSON(w, http.StatusCreated, redemption)
}

// RefundPoints handles POST /v1/points/refund.
func (h *Handler) RefundPoints(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CustomerID   string `json:"customer_id"`
		Points       int64  `json:"points"`
		RedemptionID string `json:"redemption_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.RedemptionID == "" {
		twincore.Error(w, http.StatusUnprocessableEntity, "redemption_id is required")
		return
	}

	// Validate redemption exists
	redemption, ok := h.store.Redemptions.Get(req.RedemptionID)
	if !ok {
		twincore.Error(w, http.StatusNotFound, "redemption not found")
		return
	}

	if redemption.Status == "refunded" {
		twincore.Error(w, http.StatusUnprocessableEntity, "redemption already refunded")
		return
	}

	// Use points from the redemption if not provided in the request
	refundPoints := req.Points
	if refundPoints <= 0 {
		refundPoints = redemption.Points
	}

	// Credit points back to customer
	c, ok := h.store.Customers.Get(redemption.CustomerID)
	if !ok {
		twincore.Error(w, http.StatusNotFound, "customer not found")
		return
	}

	now := h.store.Clock.Now().Unix()
	c.PointsBalance += refundPoints
	c.UpdatedAt = now
	h.store.Customers.Set(redemption.CustomerID, c)

	// Update redemption status
	redemption.Status = "refunded"
	h.store.Redemptions.Set(req.RedemptionID, redemption)

	twincore.JSON(w, http.StatusOK, redemption)
}
