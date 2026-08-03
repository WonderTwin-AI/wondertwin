package store

import (
	"encoding/json"
	"sync/atomic"

	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// MemoryStore holds all Shopify twin state in memory.
type MemoryStore struct {
	Products          *pkgstate.Store[Product]
	Variants          *pkgstate.Store[Variant]
	Images            *pkgstate.Store[Image]
	Orders            *pkgstate.Store[Order]
	Customers         *pkgstate.Store[Customer]
	Addresses         *pkgstate.Store[Address]
	CustomCollections *pkgstate.Store[Collection]
	SmartCollections  *pkgstate.Store[Collection]
	Collects          *pkgstate.Store[Collect]
	InventoryItems    *pkgstate.Store[InventoryItem]
	InventoryLevels   *pkgstate.Store[InventoryLevel]
	Locations         *pkgstate.Store[Location]
	Fulfillments      *pkgstate.Store[Fulfillment]
	FulfillmentOrders *pkgstate.Store[FulfillmentOrder]
	Transactions      *pkgstate.Store[Transaction]
	Refunds           *pkgstate.Store[Refund]
	DraftOrders       *pkgstate.Store[DraftOrder]
	Webhooks          *pkgstate.Store[Webhook]
	Metafields        *pkgstate.Store[Metafield]
	Themes            *pkgstate.Store[Theme]
	Assets            *pkgstate.Store[Asset]
	Pages             *pkgstate.Store[Page]
	PriceRules        *pkgstate.Store[PriceRule]
	DiscountCodes     *pkgstate.Store[DiscountCode]
	GiftCards         *pkgstate.Store[GiftCard]
	ScriptTags        *pkgstate.Store[ScriptTag]
	Redirects         *pkgstate.Store[Redirect]
	Blogs             *pkgstate.Store[Blog]
	Articles          *pkgstate.Store[Article]
	Clock             *pkgstate.Clock

	Shop Shop

	idCounter    atomic.Int64
	orderCounter atomic.Int64
}

func New() *MemoryStore {
	return &MemoryStore{
		Products:          pkgstate.New[Product]("prod"),
		Variants:          pkgstate.New[Variant]("var"),
		Images:            pkgstate.New[Image]("img"),
		Orders:            pkgstate.New[Order]("order"),
		Customers:         pkgstate.New[Customer]("cust"),
		Addresses:         pkgstate.New[Address]("addr"),
		CustomCollections: pkgstate.New[Collection]("cc"),
		SmartCollections:  pkgstate.New[Collection]("sc"),
		Collects:          pkgstate.New[Collect]("col"),
		InventoryItems:    pkgstate.New[InventoryItem]("ii"),
		InventoryLevels:   pkgstate.New[InventoryLevel]("il"),
		Locations:         pkgstate.New[Location]("loc"),
		Fulfillments:      pkgstate.New[Fulfillment]("ff"),
		FulfillmentOrders: pkgstate.New[FulfillmentOrder]("fo"),
		Transactions:      pkgstate.New[Transaction]("txn"),
		Refunds:           pkgstate.New[Refund]("ref"),
		DraftOrders:       pkgstate.New[DraftOrder]("do"),
		Webhooks:          pkgstate.New[Webhook]("wh"),
		Metafields:        pkgstate.New[Metafield]("mf"),
		Themes:            pkgstate.New[Theme]("theme"),
		Assets:            pkgstate.New[Asset]("asset"),
		Pages:             pkgstate.New[Page]("page"),
		PriceRules:        pkgstate.New[PriceRule]("pr"),
		DiscountCodes:     pkgstate.New[DiscountCode]("dc"),
		GiftCards:         pkgstate.New[GiftCard]("gc"),
		ScriptTags:        pkgstate.New[ScriptTag]("st"),
		Redirects:         pkgstate.New[Redirect]("rdir"),
		Blogs:             pkgstate.New[Blog]("blog"),
		Articles:          pkgstate.New[Article]("art"),
		Clock:             pkgstate.NewClock(),
		Shop: Shop{
			ID: 1, Name: "WonderTwin Store", Email: "shop@wondertwin.dev",
			Domain: "wondertwin.myshopify.com", MyshopifyDomain: "wondertwin.myshopify.com",
			Currency: "USD", PlanName: "developer", Country: "United States",
			Timezone: "(GMT-08:00) America/Los_Angeles",
		},
	}
}

func (s *MemoryStore) NextID() int64 {
	return s.idCounter.Add(1)
}

func (s *MemoryStore) NextOrderNumber() int {
	return int(s.orderCounter.Add(1)) + 1000
}

func (s *MemoryStore) Now() string {
	return s.Clock.Now().UTC().Format("2006-01-02T15:04:05-07:00")
}

type stateSnapshot struct {
	Products          map[string]Product    `json:"products,omitempty"`
	Orders            map[string]Order      `json:"orders,omitempty"`
	Customers         map[string]Customer   `json:"customers,omitempty"`
	CustomCollections map[string]Collection `json:"custom_collections,omitempty"`
	Webhooks          map[string]Webhook    `json:"webhooks,omitempty"`
	Locations         map[string]Location   `json:"locations,omitempty"`
	Shop              *Shop                 `json:"shop,omitempty"`
}

func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Products:          s.Products.Snapshot(),
		Orders:            s.Orders.Snapshot(),
		Customers:         s.Customers.Snapshot(),
		CustomCollections: s.CustomCollections.Snapshot(),
		Webhooks:          s.Webhooks.Snapshot(),
		Locations:         s.Locations.Snapshot(),
		Shop:              &s.Shop,
	}
}

func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if snap.Products != nil {
		s.Products.LoadSnapshot(snap.Products)
	}
	if snap.Orders != nil {
		s.Orders.LoadSnapshot(snap.Orders)
	}
	if snap.Customers != nil {
		s.Customers.LoadSnapshot(snap.Customers)
	}
	if snap.CustomCollections != nil {
		s.CustomCollections.LoadSnapshot(snap.CustomCollections)
	}
	if snap.Webhooks != nil {
		s.Webhooks.LoadSnapshot(snap.Webhooks)
	}
	if snap.Locations != nil {
		s.Locations.LoadSnapshot(snap.Locations)
	}
	if snap.Shop != nil {
		s.Shop = *snap.Shop
	}
	return nil
}

func (s *MemoryStore) Reset() {
	s.Products.Reset()
	s.Variants.Reset()
	s.Images.Reset()
	s.Orders.Reset()
	s.Customers.Reset()
	s.Addresses.Reset()
	s.CustomCollections.Reset()
	s.SmartCollections.Reset()
	s.Collects.Reset()
	s.InventoryItems.Reset()
	s.InventoryLevels.Reset()
	s.Locations.Reset()
	s.Fulfillments.Reset()
	s.FulfillmentOrders.Reset()
	s.Transactions.Reset()
	s.Refunds.Reset()
	s.DraftOrders.Reset()
	s.Webhooks.Reset()
	s.Metafields.Reset()
	s.Themes.Reset()
	s.Assets.Reset()
	s.Pages.Reset()
	s.PriceRules.Reset()
	s.DiscountCodes.Reset()
	s.GiftCards.Reset()
	s.ScriptTags.Reset()
	s.Redirects.Reset()
	s.Blogs.Reset()
	s.Articles.Reset()
	s.Clock.Reset()
	s.idCounter.Store(0)
	s.orderCounter.Store(0)
	s.Shop = Shop{
		ID: 1, Name: "WonderTwin Store", Email: "shop@wondertwin.dev",
		Domain: "wondertwin.myshopify.com", MyshopifyDomain: "wondertwin.myshopify.com",
		Currency: "USD", PlanName: "developer",
	}
}
