package store

// Merchant represents a LoyaltyLion merchant identified by API credentials.
type Merchant struct {
	APIKey    string            `json:"api_key"`
	APISecret string            `json:"api_secret"`
	Name      string            `json:"name"`
	Settings  map[string]string `json:"settings,omitempty"`
}

// Customer represents a loyalty program customer scoped to a merchant.
type Customer struct {
	ID             int               `json:"id"`
	MerchantID     string            `json:"merchant_id"`
	Email          string            `json:"email"`
	PointsApproved int               `json:"points_approved"`
	PointsPending  int               `json:"points_pending"`
	PointsSpent    int               `json:"points_spent"`
	PointsExpired  int               `json:"points_expired"`
	Properties     map[string]string `json:"properties,omitempty"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
	// APIKey is the owning merchant's API key (used for scoping, not serialized to API responses).
	APIKey string `json:"-"`
}

// PointsTransaction records a single points state change.
type PointsTransaction struct {
	ID         int    `json:"id"`
	CustomerID int    `json:"customer_id"`
	Type       string `json:"type"` // earn, spend, adjust, expire
	Amount     int    `json:"amount"`
	Reason     string `json:"reason"`
	Timestamp  string `json:"timestamp"`
	APIKey     string `json:"-"`
}

// Reward represents a redeemable reward in a merchant's catalog.
type Reward struct {
	ID             int    `json:"id"`
	Title          string `json:"title"`
	PointCost      int    `json:"point_cost"`
	DiscountType   string `json:"discount_type"`   // flat, percentage
	DiscountAmount int    `json:"discount_amount"`
	APIKey         string `json:"-"`
}

// ClaimedReward represents a customer's reward redemption.
type ClaimedReward struct {
	ID        int        `json:"id"`
	RewardID  int        `json:"reward_id"`
	PointCost int        `json:"point_cost"`
	Redeemable Redeemable `json:"redeemable"`
	Refunded  bool       `json:"refunded"`
	CreatedAt string     `json:"created_at"`
	// Internal fields
	CustomerID int    `json:"-"`
	APIKey     string `json:"-"`
	Multiplier int    `json:"-"`
}

// Redeemable holds the generated discount code for a claimed reward.
type Redeemable struct {
	Code      string `json:"code"`
	Fulfilled bool   `json:"fulfilled"`
}

// Activity represents a recorded customer activity.
type Activity struct {
	ID         int               `json:"id"`
	Name       string            `json:"name"`
	MerchantID string            `json:"merchant_id"` // customer's merchant_id
	Properties map[string]string `json:"properties,omitempty"`
	Timestamp  string            `json:"timestamp"`
	APIKey     string            `json:"-"`
}

// ExpiringPoints tracks points with an expiration date for a customer.
type ExpiringPoints struct {
	ID         int    `json:"id"`
	CustomerID int    `json:"customer_id"`
	Amount     int    `json:"amount"`
	ExpiresAt  string `json:"expires_at"`
	Expired    bool   `json:"expired"`
	APIKey     string `json:"-"`
}
