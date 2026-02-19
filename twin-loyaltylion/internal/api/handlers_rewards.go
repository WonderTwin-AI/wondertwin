package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-loyaltylion/internal/store"
)

// ListAvailableRewards handles GET /v2/customers/{merchant_id}/available_rewards.
func (h *Handler) ListAvailableRewards(w http.ResponseWriter, r *http.Request) {
	apiKey := getAPIKey(r)
	merchantID := chi.URLParam(r, "merchant_id")

	// Verify customer exists
	c := h.store.GetCustomerByMerchantID(apiKey, merchantID)
	if c == nil {
		twincore.Error(w, http.StatusNotFound, "customer not found")
		return
	}

	rewards := h.store.GetRewardsByMerchant(apiKey)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"rewards": rewards,
	})
}

// ClaimReward handles POST /v2/customers/{merchant_id}/claimed_rewards.
func (h *Handler) ClaimReward(w http.ResponseWriter, r *http.Request) {
	apiKey := getAPIKey(r)
	merchantID := chi.URLParam(r, "merchant_id")

	var req struct {
		RewardID   int `json:"reward_id"`
		Multiplier int `json:"multiplier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Multiplier <= 0 {
		req.Multiplier = 1
	}

	c := h.store.GetCustomerByMerchantID(apiKey, merchantID)
	if c == nil {
		twincore.Error(w, http.StatusNotFound, "customer not found")
		return
	}

	reward, ok := h.store.Rewards.Get(store.RewardKey(req.RewardID))
	if !ok || reward.APIKey != apiKey {
		twincore.Error(w, http.StatusNotFound, "reward not found")
		return
	}

	totalCost := reward.PointCost * req.Multiplier
	now := h.store.Clock.Now()

	// Idempotency check
	if existing := h.store.FindIdempotentClaim(c.ID, req.RewardID, req.Multiplier, apiKey, now); existing != nil {
		twincore.JSON(w, http.StatusOK, map[string]any{
			"claimed_reward": existing,
		})
		return
	}

	// Check balance
	if c.PointsApproved < totalCost {
		twincore.JSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "insufficient_points",
		})
		return
	}

	// Debit points
	c.PointsApproved -= totalCost
	c.PointsSpent += totalCost
	c.UpdatedAt = now.Format(time.RFC3339)
	h.store.Customers.Set(store.CustomerKey(c.ID), *c)

	// Record transaction
	txnID := h.store.NextTransactionID()
	h.store.Transactions.Set(fmt.Sprintf("%d", txnID), store.PointsTransaction{
		ID:         txnID,
		CustomerID: c.ID,
		Type:       "spend",
		Amount:     totalCost,
		Reason:     fmt.Sprintf("Redeemed: %s", reward.Title),
		Timestamp:  now.Format(time.RFC3339),
		APIKey:     apiKey,
	})

	// Create claimed reward
	claimID := h.store.NextClaimedRewardID()
	claimed := store.ClaimedReward{
		ID:         claimID,
		RewardID:   req.RewardID,
		PointCost:  totalCost,
		Redeemable: store.Redeemable{Code: generateDiscountCode(), Fulfilled: false},
		Refunded:   false,
		CreatedAt:  now.Format(time.RFC3339),
		CustomerID: c.ID,
		APIKey:     apiKey,
		Multiplier: req.Multiplier,
	}
	h.store.ClaimedRewards.Set(store.ClaimedRewardKey(claimID), claimed)

	twincore.JSON(w, http.StatusCreated, map[string]any{
		"claimed_reward": claimed,
	})
}

// RefundClaimedReward handles POST /v2/customers/{merchant_id}/claimed_rewards/{id}/refund.
func (h *Handler) RefundClaimedReward(w http.ResponseWriter, r *http.Request) {
	apiKey := getAPIKey(r)
	merchantID := chi.URLParam(r, "merchant_id")
	idStr := chi.URLParam(r, "id")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid claimed reward ID")
		return
	}

	c := h.store.GetCustomerByMerchantID(apiKey, merchantID)
	if c == nil {
		twincore.Error(w, http.StatusNotFound, "customer not found")
		return
	}

	claimed, ok := h.store.ClaimedRewards.Get(store.ClaimedRewardKey(id))
	if !ok || claimed.APIKey != apiKey || claimed.CustomerID != c.ID {
		twincore.Error(w, http.StatusNotFound, "claimed reward not found")
		return
	}

	if claimed.Refunded {
		twincore.Error(w, http.StatusUnprocessableEntity, "already refunded")
		return
	}

	now := h.store.Clock.Now()

	// Restore points
	c.PointsApproved += claimed.PointCost
	c.PointsSpent -= claimed.PointCost
	if c.PointsSpent < 0 {
		c.PointsSpent = 0
	}
	c.UpdatedAt = now.Format(time.RFC3339)
	h.store.Customers.Set(store.CustomerKey(c.ID), *c)

	// Record transaction
	txnID := h.store.NextTransactionID()
	h.store.Transactions.Set(fmt.Sprintf("%d", txnID), store.PointsTransaction{
		ID:         txnID,
		CustomerID: c.ID,
		Type:       "adjust",
		Amount:     claimed.PointCost,
		Reason:     "Redemption refund",
		Timestamp:  now.Format(time.RFC3339),
		APIKey:     apiKey,
	})

	// Mark as refunded
	claimed.Refunded = true
	h.store.ClaimedRewards.Set(store.ClaimedRewardKey(id), claimed)

	twincore.JSON(w, http.StatusOK, map[string]any{
		"claimed_reward": claimed,
	})
}

// generateDiscountCode creates a unique code in the format LOYAL-XXXX-XXXX.
func generateDiscountCode() string {
	b := make([]byte, 4)
	rand.Read(b)
	hex := strings.ToUpper(hex.EncodeToString(b))
	return fmt.Sprintf("LOYAL-%s-%s", hex[:4], hex[4:])
}
