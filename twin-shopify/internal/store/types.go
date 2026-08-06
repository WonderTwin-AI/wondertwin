// Package store defines the Shopify twin's state types and in-memory store.
package store

// Product represents a Shopify product.
type Product struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	BodyHTML    string    `json:"body_html"`
	Vendor      string    `json:"vendor"`
	ProductType string    `json:"product_type"`
	Handle      string    `json:"handle"`
	Status      string    `json:"status"` // "active", "draft", "archived"
	Tags        string    `json:"tags"`
	Variants    []Variant `json:"variants,omitempty"`
	Images      []Image   `json:"images,omitempty"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
	PublishedAt string    `json:"published_at,omitempty"`
}

// Variant represents a product variant.
type Variant struct {
	ID                int64   `json:"id"`
	ProductID         int64   `json:"product_id"`
	Title             string  `json:"title"`
	Price             string  `json:"price"`
	CompareAtPrice    string  `json:"compare_at_price,omitempty"`
	SKU               string  `json:"sku"`
	Barcode           string  `json:"barcode,omitempty"`
	Position          int     `json:"position"`
	InventoryItemID   int64   `json:"inventory_item_id"`
	InventoryQuantity int     `json:"inventory_quantity"`
	Weight            float64 `json:"weight"`
	WeightUnit        string  `json:"weight_unit"`
	Option1           string  `json:"option1,omitempty"`
	Option2           string  `json:"option2,omitempty"`
	Option3           string  `json:"option3,omitempty"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

// Image represents a product image.
type Image struct {
	ID        int64  `json:"id"`
	ProductID int64  `json:"product_id"`
	Position  int    `json:"position"`
	Src       string `json:"src"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Alt       string `json:"alt,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Order represents a Shopify order.
type Order struct {
	ID                int64      `json:"id"`
	Name              string     `json:"name"` // "#1001"
	OrderNumber       int        `json:"order_number"`
	Email             string     `json:"email"`
	FinancialStatus   string     `json:"financial_status"`             // "pending", "paid", "refunded", etc.
	FulfillmentStatus string     `json:"fulfillment_status,omitempty"` // null, "fulfilled", "partial"
	Currency          string     `json:"currency"`
	TotalPrice        string     `json:"total_price"`
	SubtotalPrice     string     `json:"subtotal_price"`
	TotalTax          string     `json:"total_tax"`
	TotalDiscounts    string     `json:"total_discounts"`
	LineItems         []LineItem `json:"line_items,omitempty"`
	Customer          *Customer  `json:"customer,omitempty"`
	ShippingAddress   *Address   `json:"shipping_address,omitempty"`
	BillingAddress    *Address   `json:"billing_address,omitempty"`
	Note              string     `json:"note,omitempty"`
	Tags              string     `json:"tags,omitempty"`
	Cancelled         bool       `json:"cancelled"`
	CancelledAt       string     `json:"cancelled_at,omitempty"`
	ClosedAt          string     `json:"closed_at,omitempty"`
	CreatedAt         string     `json:"created_at"`
	UpdatedAt         string     `json:"updated_at"`
}

// LineItem represents an order line item.
type LineItem struct {
	ID        int64  `json:"id"`
	ProductID int64  `json:"product_id"`
	VariantID int64  `json:"variant_id"`
	Title     string `json:"title"`
	Quantity  int    `json:"quantity"`
	Price     string `json:"price"`
	SKU       string `json:"sku,omitempty"`
	Name      string `json:"name"`
}

// Customer represents a Shopify customer.
type Customer struct {
	ID             int64     `json:"id"`
	Email          string    `json:"email"`
	FirstName      string    `json:"first_name"`
	LastName       string    `json:"last_name"`
	Phone          string    `json:"phone,omitempty"`
	OrdersCount    int       `json:"orders_count"`
	TotalSpent     string    `json:"total_spent"`
	State          string    `json:"state"` // "enabled", "disabled", "invited"
	Tags           string    `json:"tags,omitempty"`
	Note           string    `json:"note,omitempty"`
	Addresses      []Address `json:"addresses,omitempty"`
	DefaultAddress *Address  `json:"default_address,omitempty"`
	CreatedAt      string    `json:"created_at"`
	UpdatedAt      string    `json:"updated_at"`
}

// Address represents a customer or order address.
type Address struct {
	ID           int64  `json:"id,omitempty"`
	CustomerID   int64  `json:"customer_id,omitempty"`
	FirstName    string `json:"first_name,omitempty"`
	LastName     string `json:"last_name,omitempty"`
	Address1     string `json:"address1,omitempty"`
	Address2     string `json:"address2,omitempty"`
	City         string `json:"city,omitempty"`
	Province     string `json:"province,omitempty"`
	ProvinceCode string `json:"province_code,omitempty"`
	Country      string `json:"country,omitempty"`
	CountryCode  string `json:"country_code,omitempty"`
	Zip          string `json:"zip,omitempty"`
	Phone        string `json:"phone,omitempty"`
	Default      bool   `json:"default,omitempty"`
}

// Collection represents a custom or smart collection.
type Collection struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	BodyHTML    string `json:"body_html,omitempty"`
	Handle      string `json:"handle"`
	SortOrder   string `json:"sort_order,omitempty"`
	Published   bool   `json:"published"`
	PublishedAt string `json:"published_at,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	IsSmart     bool   `json:"-"` // internal: smart vs custom
}

// Collect represents a product-to-collection link.
type Collect struct {
	ID           int64  `json:"id"`
	CollectionID int64  `json:"collection_id"`
	ProductID    int64  `json:"product_id"`
	Position     int    `json:"position"`
	CreatedAt    string `json:"created_at"`
}

// InventoryItem represents an inventory item.
type InventoryItem struct {
	ID                  int64  `json:"id"`
	SKU                 string `json:"sku"`
	Tracked             bool   `json:"tracked"`
	Cost                string `json:"cost,omitempty"`
	CountryCodeOfOrigin string `json:"country_code_of_origin,omitempty"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

// InventoryLevel represents inventory at a location.
type InventoryLevel struct {
	InventoryItemID int64  `json:"inventory_item_id"`
	LocationID      int64  `json:"location_id"`
	Available       int    `json:"available"`
	UpdatedAt       string `json:"updated_at"`
}

// Location represents a Shopify location.
type Location struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Address1 string `json:"address1,omitempty"`
	City     string `json:"city,omitempty"`
	Country  string `json:"country,omitempty"`
	Active   bool   `json:"active"`
	Legacy   bool   `json:"legacy"`
}

// Fulfillment represents a fulfillment on an order.
type Fulfillment struct {
	ID              int64  `json:"id"`
	OrderID         int64  `json:"order_id"`
	Status          string `json:"status"` // "success", "cancelled", "error", "failure"
	TrackingCompany string `json:"tracking_company,omitempty"`
	TrackingNumber  string `json:"tracking_number,omitempty"`
	TrackingURL     string `json:"tracking_url,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// FulfillmentOrder represents a fulfillment order.
type FulfillmentOrder struct {
	ID                 int64  `json:"id"`
	OrderID            int64  `json:"order_id"`
	Status             string `json:"status"` // "open", "in_progress", "closed", "cancelled"
	AssignedLocationID int64  `json:"assigned_location_id"`
}

// Transaction represents a payment transaction on an order.
type Transaction struct {
	ID        int64  `json:"id"`
	OrderID   int64  `json:"order_id"`
	Kind      string `json:"kind"`   // "sale", "capture", "authorization", "void", "refund"
	Status    string `json:"status"` // "success", "failure", "pending", "error"
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Gateway   string `json:"gateway"`
	CreatedAt string `json:"created_at"`
}

// Refund represents a refund on an order.
type Refund struct {
	ID        int64  `json:"id"`
	OrderID   int64  `json:"order_id"`
	Note      string `json:"note,omitempty"`
	CreatedAt string `json:"created_at"`
}

// DraftOrder represents a draft order.
type DraftOrder struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Email         string `json:"email,omitempty"`
	Status        string `json:"status"` // "open", "invoice_sent", "completed"
	TotalPrice    string `json:"total_price"`
	SubtotalPrice string `json:"subtotal_price"`
	Currency      string `json:"currency"`
	Note          string `json:"note,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// Webhook represents a Shopify webhook.
type Webhook struct {
	ID        int64  `json:"id"`
	Topic     string `json:"topic"`
	Address   string `json:"address"`
	Format    string `json:"format"` // "json", "xml"
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Metafield represents a metafield on any resource.
type Metafield struct {
	ID            int64  `json:"id"`
	Namespace     string `json:"namespace"`
	Key           string `json:"key"`
	Value         string `json:"value"`
	Type          string `json:"type"` // "single_line_text_field", "number_integer", etc.
	OwnerID       int64  `json:"owner_id"`
	OwnerResource string `json:"owner_resource"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// Theme represents a Shopify theme.
type Theme struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"` // "main", "unpublished"
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Asset represents a theme asset.
type Asset struct {
	Key         string `json:"key"`
	Value       string `json:"value,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	ThemeID     int64  `json:"theme_id"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// Page represents a Shopify page.
type Page struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	BodyHTML  string `json:"body_html"`
	Handle    string `json:"handle"`
	Author    string `json:"author,omitempty"`
	Published bool   `json:"published"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// PriceRule represents a Shopify price rule.
type PriceRule struct {
	ID              int64  `json:"id"`
	Title           string `json:"title"`
	TargetType      string `json:"target_type"` // "line_item", "shipping_line"
	ValueType       string `json:"value_type"`  // "fixed_amount", "percentage"
	Value           string `json:"value"`
	OncePerCustomer bool   `json:"once_per_customer"`
	UsageLimit      *int   `json:"usage_limit,omitempty"`
	StartsAt        string `json:"starts_at"`
	EndsAt          string `json:"ends_at,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// DiscountCode represents a discount code.
type DiscountCode struct {
	ID          int64  `json:"id"`
	PriceRuleID int64  `json:"price_rule_id"`
	Code        string `json:"code"`
	UsageCount  int    `json:"usage_count"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// GiftCard represents a Shopify gift card.
type GiftCard struct {
	ID           int64  `json:"id"`
	Code         string `json:"code"`
	Balance      string `json:"balance"`
	InitialValue string `json:"initial_value"`
	Currency     string `json:"currency"`
	Disabled     bool   `json:"disabled_at,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// Shop represents shop info.
type Shop struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	Domain          string `json:"domain"`
	MyshopifyDomain string `json:"myshopify_domain"`
	Currency        string `json:"currency"`
	PlanName        string `json:"plan_name"`
	Country         string `json:"country_name"`
	Timezone        string `json:"timezone"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// ScriptTag represents a script tag.
type ScriptTag struct {
	ID        int64  `json:"id"`
	Src       string `json:"src"`
	Event     string `json:"event"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Redirect represents a URL redirect.
type Redirect struct {
	ID     int64  `json:"id"`
	Path   string `json:"path"`
	Target string `json:"target"`
}

// Blog represents a Shopify blog.
type Blog struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Handle    string `json:"handle"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Article represents a blog article.
type Article struct {
	ID        int64  `json:"id"`
	BlogID    int64  `json:"blog_id"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	BodyHTML  string `json:"body_html"`
	Handle    string `json:"handle"`
	Published bool   `json:"published"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
