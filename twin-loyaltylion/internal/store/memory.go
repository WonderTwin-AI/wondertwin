package store

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	pkgstore "github.com/wondertwin-ai/wondertwin/twinkit/store"
)

// MemoryStore holds all twin state in memory.
type MemoryStore struct {
	Merchants      *pkgstore.Store[Merchant]
	Customers      *pkgstore.Store[Customer]
	Transactions   *pkgstore.Store[PointsTransaction]
	Rewards        *pkgstore.Store[Reward]
	ClaimedRewards *pkgstore.Store[ClaimedReward]
	Activities     *pkgstore.Store[Activity]
	ExpiringPoints *pkgstore.Store[ExpiringPoints]
	Clock          *pkgstore.Clock

	customerCounter      atomic.Int64
	transactionCounter   atomic.Int64
	rewardCounter        atomic.Int64
	claimedRewardCounter atomic.Int64
	activityCounter      atomic.Int64
	expiringCounter      atomic.Int64
}

// New creates a new MemoryStore.
func New() *MemoryStore {
	return &MemoryStore{
		Merchants:      pkgstore.New[Merchant]("merchant"),
		Customers:      pkgstore.New[Customer]("cust"),
		Transactions:   pkgstore.New[PointsTransaction]("txn"),
		Rewards:        pkgstore.New[Reward]("reward"),
		ClaimedRewards: pkgstore.New[ClaimedReward]("claim"),
		Activities:     pkgstore.New[Activity]("act"),
		ExpiringPoints: pkgstore.New[ExpiringPoints]("exp"),
		Clock:          pkgstore.NewClock(),
	}
}

// NextCustomerID returns the next auto-increment customer ID.
func (s *MemoryStore) NextCustomerID() int {
	return int(s.customerCounter.Add(1))
}

// NextTransactionID returns the next auto-increment transaction ID.
func (s *MemoryStore) NextTransactionID() int {
	return int(s.transactionCounter.Add(1))
}

// NextRewardID returns the next auto-increment reward ID.
func (s *MemoryStore) NextRewardID() int {
	return int(s.rewardCounter.Add(1))
}

// NextClaimedRewardID returns the next auto-increment claimed reward ID.
func (s *MemoryStore) NextClaimedRewardID() int {
	return int(s.claimedRewardCounter.Add(1))
}

// NextActivityID returns the next auto-increment activity ID.
func (s *MemoryStore) NextActivityID() int {
	return int(s.activityCounter.Add(1))
}

// NextExpiringID returns the next auto-increment expiring points ID.
func (s *MemoryStore) NextExpiringID() int {
	return int(s.expiringCounter.Add(1))
}

// CustomerKey returns the store key for a customer ID.
func CustomerKey(id int) string {
	return fmt.Sprintf("%d", id)
}

// RewardKey returns the store key for a reward ID.
func RewardKey(id int) string {
	return fmt.Sprintf("%d", id)
}

// ClaimedRewardKey returns the store key for a claimed reward ID.
func ClaimedRewardKey(id int) string {
	return fmt.Sprintf("%d", id)
}

// GetMerchantByAPIKey returns the merchant for a given API key.
func (s *MemoryStore) GetMerchantByAPIKey(apiKey string) (Merchant, bool) {
	return s.Merchants.Get(apiKey)
}

// GetCustomersByMerchant returns all customers for a given merchant API key.
func (s *MemoryStore) GetCustomersByMerchant(apiKey string) []Customer {
	return s.Customers.Filter(func(_ string, c Customer) bool {
		return c.APIKey == apiKey
	})
}

// GetCustomerByEmail returns a customer by email scoped to a merchant.
func (s *MemoryStore) GetCustomerByEmail(apiKey, email string) *Customer {
	items := s.Customers.Filter(func(_ string, c Customer) bool {
		return c.APIKey == apiKey && c.Email == email
	})
	if len(items) == 0 {
		return nil
	}
	return &items[0]
}

// GetCustomerByMerchantID returns a customer by their merchant_id scoped to a merchant.
func (s *MemoryStore) GetCustomerByMerchantID(apiKey, merchantID string) *Customer {
	items := s.Customers.Filter(func(_ string, c Customer) bool {
		return c.APIKey == apiKey && c.MerchantID == merchantID
	})
	if len(items) == 0 {
		return nil
	}
	return &items[0]
}

// GetRewardsByMerchant returns all rewards for a merchant.
func (s *MemoryStore) GetRewardsByMerchant(apiKey string) []Reward {
	return s.Rewards.Filter(func(_ string, r Reward) bool {
		return r.APIKey == apiKey
	})
}

// GetClaimedRewardsByCustomer returns all claimed rewards for a customer.
func (s *MemoryStore) GetClaimedRewardsByCustomer(customerID int, apiKey string) []ClaimedReward {
	return s.ClaimedRewards.Filter(func(_ string, cr ClaimedReward) bool {
		return cr.CustomerID == customerID && cr.APIKey == apiKey
	})
}

// FindIdempotentClaim checks for a recent identical claim (same customer, reward_id, multiplier within 60s).
func (s *MemoryStore) FindIdempotentClaim(customerID, rewardID, multiplier int, apiKey string, now time.Time) *ClaimedReward {
	items := s.ClaimedRewards.Filter(func(_ string, cr ClaimedReward) bool {
		if cr.CustomerID != customerID || cr.RewardID != rewardID || cr.Multiplier != multiplier || cr.APIKey != apiKey || cr.Refunded {
			return false
		}
		t, err := time.Parse(time.RFC3339, cr.CreatedAt)
		if err != nil {
			return false
		}
		return now.Sub(t) < 60*time.Second
	})
	if len(items) == 0 {
		return nil
	}
	return &items[0]
}

// ProcessExpiredPoints checks all expiring points and transitions expired ones.
func (s *MemoryStore) ProcessExpiredPoints(now time.Time) {
	ids, items := s.ExpiringPoints.FilterWithIDs(func(_ string, ep ExpiringPoints) bool {
		if ep.Expired || ep.Amount <= 0 {
			return false
		}
		t, err := time.Parse(time.RFC3339, ep.ExpiresAt)
		if err != nil {
			return false
		}
		return now.After(t)
	})

	for i, ep := range items {
		// Mark as expired
		ep.Expired = true
		s.ExpiringPoints.Set(ids[i], ep)

		// Update customer balance
		c, ok := s.Customers.Get(CustomerKey(ep.CustomerID))
		if !ok {
			continue
		}
		expired := ep.Amount
		if expired > c.PointsApproved {
			expired = c.PointsApproved
		}
		c.PointsApproved -= expired
		c.PointsExpired += expired
		c.UpdatedAt = now.Format(time.RFC3339)
		s.Customers.Set(CustomerKey(c.ID), c)

		// Record transaction
		txnID := s.NextTransactionID()
		s.Transactions.Set(fmt.Sprintf("%d", txnID), PointsTransaction{
			ID:         txnID,
			CustomerID: ep.CustomerID,
			Type:       "expire",
			Amount:     expired,
			Reason:     "Points expired",
			Timestamp:  now.Format(time.RFC3339),
			APIKey:     ep.APIKey,
		})
	}
}

type stateSnapshot struct {
	Merchants      map[string]Merchant         `json:"merchants"`
	Customers      map[string]Customer         `json:"customers"`
	Transactions   map[string]PointsTransaction `json:"transactions"`
	Rewards        map[string]Reward           `json:"rewards"`
	ClaimedRewards map[string]ClaimedReward    `json:"claimed_rewards"`
	Activities     map[string]Activity         `json:"activities"`
	ExpiringPoints map[string]ExpiringPoints   `json:"expiring_points"`
}

// Snapshot returns full state as JSON-serializable value.
func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Merchants:      s.Merchants.Snapshot(),
		Customers:      s.Customers.Snapshot(),
		Transactions:   s.Transactions.Snapshot(),
		Rewards:        s.Rewards.Snapshot(),
		ClaimedRewards: s.ClaimedRewards.Snapshot(),
		Activities:     s.Activities.Snapshot(),
		ExpiringPoints: s.ExpiringPoints.Snapshot(),
	}
}

// LoadState loads state from JSON.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if snap.Merchants != nil {
		s.Merchants.LoadSnapshot(snap.Merchants)
	}
	if snap.Customers != nil {
		s.Customers.LoadSnapshot(snap.Customers)
	}
	if snap.Transactions != nil {
		s.Transactions.LoadSnapshot(snap.Transactions)
	}
	if snap.Rewards != nil {
		s.Rewards.LoadSnapshot(snap.Rewards)
	}
	if snap.ClaimedRewards != nil {
		s.ClaimedRewards.LoadSnapshot(snap.ClaimedRewards)
	}
	if snap.Activities != nil {
		s.Activities.LoadSnapshot(snap.Activities)
	}
	if snap.ExpiringPoints != nil {
		s.ExpiringPoints.LoadSnapshot(snap.ExpiringPoints)
	}
	return nil
}

// Reset clears all state and reloads seed fixtures.
func (s *MemoryStore) Reset() {
	s.Merchants.Reset()
	s.Customers.Reset()
	s.Transactions.Reset()
	s.Rewards.Reset()
	s.ClaimedRewards.Reset()
	s.Activities.Reset()
	s.ExpiringPoints.Reset()
	s.Clock.Reset()
	s.customerCounter.Store(0)
	s.transactionCounter.Store(0)
	s.rewardCounter.Store(0)
	s.claimedRewardCounter.Store(0)
	s.activityCounter.Store(0)
	s.expiringCounter.Store(0)
	s.SeedDefaults()
}

// SeedDefaults populates the store with default fixture data.
func (s *MemoryStore) SeedDefaults() {
	now := s.Clock.Now()
	thirtyDaysLater := now.Add(30 * 24 * time.Hour).Format(time.RFC3339)
	ts := now.Format(time.RFC3339)

	// Merchant A
	s.Merchants.Set("ll_test_key_alpha", Merchant{
		APIKey:    "ll_test_key_alpha",
		APISecret: "ll_test_secret_alpha",
		Name:      "Alpha Store",
	})

	// Merchant B
	s.Merchants.Set("ll_test_key_beta", Merchant{
		APIKey:    "ll_test_key_beta",
		APISecret: "ll_test_secret_beta",
		Name:      "Beta Store",
	})

	// --- Merchant A Customers ---
	sarahID := s.NextCustomerID()
	s.Customers.Set(CustomerKey(sarahID), Customer{
		ID: sarahID, MerchantID: "cust-001", Email: "sarah@example.com",
		PointsApproved: 4200, PointsPending: 100, PointsSpent: 3500, PointsExpired: 0,
		CreatedAt: ts, UpdatedAt: ts, APIKey: "ll_test_key_alpha",
	})

	// Sarah has 500 points expiring in 30 days
	expID := s.NextExpiringID()
	s.ExpiringPoints.Set(fmt.Sprintf("%d", expID), ExpiringPoints{
		ID: expID, CustomerID: sarahID, Amount: 500,
		ExpiresAt: thirtyDaysLater, Expired: false, APIKey: "ll_test_key_alpha",
	})

	alexID := s.NextCustomerID()
	s.Customers.Set(CustomerKey(alexID), Customer{
		ID: alexID, MerchantID: "cust-002", Email: "alex@example.com",
		PointsApproved: 0, PointsPending: 0, PointsSpent: 1000, PointsExpired: 0,
		CreatedAt: ts, UpdatedAt: ts, APIKey: "ll_test_key_alpha",
	})

	jamieID := s.NextCustomerID()
	s.Customers.Set(CustomerKey(jamieID), Customer{
		ID: jamieID, MerchantID: "cust-003", Email: "jamie@example.com",
		PointsApproved: 15000, PointsPending: 500, PointsSpent: 2000, PointsExpired: 0,
		CreatedAt: ts, UpdatedAt: ts, APIKey: "ll_test_key_alpha",
	})

	// --- Merchant A Rewards ---
	for _, r := range []struct {
		title          string
		cost           int
		discountType   string
		discountAmount int
	}{
		{"$5 Off", 500, "flat", 5},
		{"$10 Off", 1000, "flat", 10},
		{"$25 Off", 2500, "flat", 25},
		{"Free Shipping", 750, "flat", 0},
	} {
		id := s.NextRewardID()
		s.Rewards.Set(RewardKey(id), Reward{
			ID: id, Title: r.title, PointCost: r.cost,
			DiscountType: r.discountType, DiscountAmount: r.discountAmount,
			APIKey: "ll_test_key_alpha",
		})
	}

	// --- Merchant B Customers ---
	sarahBID := s.NextCustomerID()
	s.Customers.Set(CustomerKey(sarahBID), Customer{
		ID: sarahBID, MerchantID: "sw-sarah-001", Email: "sarah@example.com",
		PointsApproved: 1000, PointsPending: 0, PointsSpent: 0, PointsExpired: 0,
		CreatedAt: ts, UpdatedAt: ts, APIKey: "ll_test_key_beta",
	})

	morganID := s.NextCustomerID()
	s.Customers.Set(CustomerKey(morganID), Customer{
		ID: morganID, MerchantID: "sw-morgan-001", Email: "morgan@example.com",
		PointsApproved: 8000, PointsPending: 200, PointsSpent: 0, PointsExpired: 0,
		CreatedAt: ts, UpdatedAt: ts, APIKey: "ll_test_key_beta",
	})

	// --- Merchant B Rewards ---
	for _, r := range []struct {
		title          string
		cost           int
		discountType   string
		discountAmount int
	}{
		{"$10 Off", 1000, "flat", 10},
		{"$50 Off", 5000, "flat", 50},
	} {
		id := s.NextRewardID()
		s.Rewards.Set(RewardKey(id), Reward{
			ID: id, Title: r.title, PointCost: r.cost,
			DiscountType: r.discountType, DiscountAmount: r.discountAmount,
			APIKey: "ll_test_key_beta",
		})
	}
}
