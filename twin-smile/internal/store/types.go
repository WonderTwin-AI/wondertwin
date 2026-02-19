package store

// Customer represents a Smile.io rewards customer.
type Customer struct {
	ID              string            `json:"id"`
	Email           string            `json:"email"`
	FirstName       string            `json:"first_name"`
	LastName        string            `json:"last_name"`
	PointsBalance   int64             `json:"points_balance"`
	Tier            string            `json:"tier"`             // "member", "silver", "gold", "vip"
	PointsPerDollar float64           `json:"points_per_dollar"` // conversion rate
	Metadata        map[string]string `json:"metadata,omitempty"`
	CreatedAt       int64             `json:"created_at"`
	UpdatedAt       int64             `json:"updated_at"`
}

// Redemption represents a points redemption transaction.
type Redemption struct {
	ID             string `json:"id"`
	CustomerID     string `json:"customer_id"`
	Points         int64  `json:"points"`
	ValueCents     int64  `json:"value_cents"` // calculated from points / points_per_dollar * 100
	Status         string `json:"status"`      // "completed", "refunded"
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	CreatedAt      int64  `json:"created_at"`
}
