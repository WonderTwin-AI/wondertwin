// Package api implements the Shopify Admin REST API-compatible handlers for the twin.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-shopify/internal/store"
)

// Handler holds Shopify API state.
type Handler struct {
	store *store.MemoryStore
	mw    *twincore.Middleware
}

func NewHandler(s *store.MemoryStore, mw *twincore.Middleware) *Handler {
	return &Handler{store: s, mw: mw}
}

// Routes mounts the Shopify Admin REST API routes.
// Shopify uses /admin/api/{version}/{resource}.json paths.
// We accept any version segment.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/admin/api/{version}", func(r chi.Router) {
		r.Use(h.shopifyAuthMiddleware)
		r.Use(h.mw.FaultInjection)
		r.Use(h.rateLimitHeaders)

		// Shop
		r.Get("/shop.json", h.GetShop)

		// Products
		r.Get("/products.json", h.ListProducts)
		r.Post("/products.json", h.CreateProduct)
		r.Get("/products/{id}.json", h.GetProduct)
		r.Put("/products/{id}.json", h.UpdateProduct)
		r.Delete("/products/{id}.json", h.DeleteProduct)
		r.Get("/products/count.json", h.CountProducts)

		// Variants
		r.Get("/products/{product_id}/variants.json", h.ListVariants)
		r.Post("/products/{product_id}/variants.json", h.CreateVariant)
		r.Get("/variants/{id}.json", h.GetVariant)
		r.Put("/variants/{id}.json", h.UpdateVariant)
		r.Delete("/products/{product_id}/variants/{id}.json", h.DeleteVariant)

		// Product Images
		r.Get("/products/{product_id}/images.json", h.ListImages)
		r.Post("/products/{product_id}/images.json", h.CreateImage)
		r.Get("/products/{product_id}/images/{id}.json", h.GetImage)
		r.Put("/products/{product_id}/images/{id}.json", h.UpdateImage)
		r.Delete("/products/{product_id}/images/{id}.json", h.DeleteImage)

		// Orders
		r.Get("/orders.json", h.ListOrders)
		r.Post("/orders.json", h.CreateOrder)
		r.Get("/orders/{id}.json", h.GetOrder)
		r.Put("/orders/{id}.json", h.UpdateOrder)
		r.Delete("/orders/{id}.json", h.DeleteOrder)
		r.Post("/orders/{id}/close.json", h.CloseOrder)
		r.Post("/orders/{id}/open.json", h.OpenOrder)
		r.Post("/orders/{id}/cancel.json", h.CancelOrder)
		r.Get("/orders/count.json", h.CountOrders)

		// Transactions
		r.Get("/orders/{order_id}/transactions.json", h.ListTransactions)
		r.Post("/orders/{order_id}/transactions.json", h.CreateTransaction)
		r.Get("/orders/{order_id}/transactions/{id}.json", h.GetTransaction)

		// Refunds
		r.Get("/orders/{order_id}/refunds.json", h.ListRefunds)
		r.Post("/orders/{order_id}/refunds.json", h.CreateRefund)
		r.Get("/orders/{order_id}/refunds/{id}.json", h.GetRefund)

		// Customers
		r.Get("/customers.json", h.ListCustomers)
		r.Post("/customers.json", h.CreateCustomer)
		r.Get("/customers/{id}.json", h.GetCustomer)
		r.Put("/customers/{id}.json", h.UpdateCustomer)
		r.Delete("/customers/{id}.json", h.DeleteCustomer)
		r.Get("/customers/search.json", h.SearchCustomers)
		r.Get("/customers/count.json", h.CountCustomers)

		// Customer Addresses
		r.Get("/customers/{customer_id}/addresses.json", h.ListAddresses)
		r.Post("/customers/{customer_id}/addresses.json", h.CreateAddress)
		r.Get("/customers/{customer_id}/addresses/{id}.json", h.GetAddress)
		r.Put("/customers/{customer_id}/addresses/{id}.json", h.UpdateAddress)
		r.Delete("/customers/{customer_id}/addresses/{id}.json", h.DeleteAddress)

		// Collections
		r.Get("/custom_collections.json", h.ListCustomCollections)
		r.Post("/custom_collections.json", h.CreateCustomCollection)
		r.Get("/custom_collections/{id}.json", h.GetCustomCollection)
		r.Put("/custom_collections/{id}.json", h.UpdateCustomCollection)
		r.Delete("/custom_collections/{id}.json", h.DeleteCustomCollection)
		r.Get("/smart_collections.json", h.ListSmartCollections)
		r.Post("/smart_collections.json", h.CreateSmartCollection)
		r.Get("/smart_collections/{id}.json", h.GetSmartCollection)
		r.Put("/smart_collections/{id}.json", h.UpdateSmartCollection)
		r.Delete("/smart_collections/{id}.json", h.DeleteSmartCollection)

		// Collects
		r.Get("/collects.json", h.ListCollects)
		r.Post("/collects.json", h.CreateCollect)
		r.Delete("/collects/{id}.json", h.DeleteCollect)

		// Inventory
		r.Get("/inventory_items.json", h.ListInventoryItems)
		r.Get("/inventory_items/{id}.json", h.GetInventoryItem)
		r.Put("/inventory_items/{id}.json", h.UpdateInventoryItem)
		r.Get("/inventory_levels.json", h.ListInventoryLevels)
		r.Post("/inventory_levels/adjust.json", h.AdjustInventoryLevel)
		r.Post("/inventory_levels/set.json", h.SetInventoryLevel)
		r.Post("/inventory_levels/connect.json", h.ConnectInventoryLevel)
		r.Delete("/inventory_levels.json", h.DeleteInventoryLevel)
		r.Get("/locations.json", h.ListLocations)
		r.Get("/locations/{id}.json", h.GetLocation)

		// Fulfillments
		r.Get("/orders/{order_id}/fulfillments.json", h.ListFulfillments)
		r.Post("/fulfillments.json", h.CreateFulfillment)
		r.Get("/orders/{order_id}/fulfillment_orders.json", h.ListFulfillmentOrders)

		// Draft Orders
		r.Get("/draft_orders.json", h.ListDraftOrders)
		r.Post("/draft_orders.json", h.CreateDraftOrder)
		r.Get("/draft_orders/{id}.json", h.GetDraftOrder)
		r.Put("/draft_orders/{id}.json", h.UpdateDraftOrder)
		r.Delete("/draft_orders/{id}.json", h.DeleteDraftOrder)
		r.Post("/draft_orders/{id}/complete.json", h.CompleteDraftOrder)
		r.Post("/draft_orders/{id}/send_invoice.json", h.SendDraftOrderInvoice)

		// Webhooks
		r.Get("/webhooks.json", h.ListWebhooks)
		r.Post("/webhooks.json", h.CreateWebhook)
		r.Get("/webhooks/{id}.json", h.GetWebhook)
		r.Put("/webhooks/{id}.json", h.UpdateWebhook)
		r.Delete("/webhooks/{id}.json", h.DeleteWebhook)
		r.Get("/webhooks/count.json", h.CountWebhooks)

		// Metafields (shop-level)
		r.Get("/metafields.json", h.ListMetafields)
		r.Post("/metafields.json", h.CreateMetafield)
		r.Get("/metafields/{id}.json", h.GetMetafield)
		r.Put("/metafields/{id}.json", h.UpdateMetafield)
		r.Delete("/metafields/{id}.json", h.DeleteMetafield)

		// Themes + Assets
		r.Get("/themes.json", h.ListThemes)
		r.Post("/themes.json", h.CreateTheme)
		r.Get("/themes/{id}.json", h.GetTheme)
		r.Put("/themes/{id}.json", h.UpdateTheme)
		r.Delete("/themes/{id}.json", h.DeleteTheme)
		r.Get("/themes/{theme_id}/assets.json", h.ListAssets)
		r.Put("/themes/{theme_id}/assets.json", h.CreateOrUpdateAsset)
		r.Delete("/themes/{theme_id}/assets.json", h.DeleteAsset)

		// Pages
		r.Get("/pages.json", h.ListPages)
		r.Post("/pages.json", h.CreatePage)
		r.Get("/pages/{id}.json", h.GetPage)
		r.Put("/pages/{id}.json", h.UpdatePage)
		r.Delete("/pages/{id}.json", h.DeletePage)

		// Price Rules + Discount Codes
		r.Get("/price_rules.json", h.ListPriceRules)
		r.Post("/price_rules.json", h.CreatePriceRule)
		r.Get("/price_rules/{id}.json", h.GetPriceRule)
		r.Put("/price_rules/{id}.json", h.UpdatePriceRule)
		r.Delete("/price_rules/{id}.json", h.DeletePriceRule)
		r.Get("/price_rules/{price_rule_id}/discount_codes.json", h.ListDiscountCodes)
		r.Post("/price_rules/{price_rule_id}/discount_codes.json", h.CreateDiscountCode)
		r.Get("/price_rules/{price_rule_id}/discount_codes/{id}.json", h.GetDiscountCode)
		r.Put("/price_rules/{price_rule_id}/discount_codes/{id}.json", h.UpdateDiscountCode)
		r.Delete("/price_rules/{price_rule_id}/discount_codes/{id}.json", h.DeleteDiscountCode)

		// Gift Cards
		r.Get("/gift_cards.json", h.ListGiftCards)
		r.Post("/gift_cards.json", h.CreateGiftCard)
		r.Get("/gift_cards/{id}.json", h.GetGiftCard)
		r.Put("/gift_cards/{id}.json", h.UpdateGiftCard)

		// Blogs + Articles
		r.Get("/blogs.json", h.ListBlogs)
		r.Post("/blogs.json", h.CreateBlog)
		r.Get("/blogs/{id}.json", h.GetBlog)
		r.Put("/blogs/{id}.json", h.UpdateBlog)
		r.Delete("/blogs/{id}.json", h.DeleteBlog)
		r.Get("/blogs/{blog_id}/articles.json", h.ListArticles)
		r.Post("/blogs/{blog_id}/articles.json", h.CreateArticle)
		r.Get("/blogs/{blog_id}/articles/{id}.json", h.GetArticle)
		r.Put("/blogs/{blog_id}/articles/{id}.json", h.UpdateArticle)
		r.Delete("/blogs/{blog_id}/articles/{id}.json", h.DeleteArticle)

		// Script Tags
		r.Get("/script_tags.json", h.ListScriptTags)
		r.Post("/script_tags.json", h.CreateScriptTag)
		r.Get("/script_tags/{id}.json", h.GetScriptTag)
		r.Put("/script_tags/{id}.json", h.UpdateScriptTag)
		r.Delete("/script_tags/{id}.json", h.DeleteScriptTag)

		// Redirects
		r.Get("/redirects.json", h.ListRedirects)
		r.Post("/redirects.json", h.CreateRedirect)
		r.Get("/redirects/{id}.json", h.GetRedirect)
		r.Put("/redirects/{id}.json", h.UpdateRedirect)
		r.Delete("/redirects/{id}.json", h.DeleteRedirect)

		// Access Scopes
		r.Get("/access_scopes.json", h.ListAccessScopes)
	})

	// Admin extras
	r.Get("/admin/products", h.AdminListProducts)
	r.Get("/admin/orders", h.AdminListOrders)
	r.Get("/admin/customers", h.AdminListCustomers)
}

func (h *Handler) shopifyAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Shopify-Access-Token")
		if token == "" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{
				"errors": "[API] Invalid API key or access token (unrecognized login or wrong password)",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) rateLimitHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Shopify-Shop-Api-Call-Limit", "1/40")
		next.ServeHTTP(w, r)
	})
}

// shopifyJSON writes a Shopify-style JSON response (resource wrapped in key).
func shopifyJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// shopifyError writes a Shopify-style error response.
func shopifyError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"errors": msg})
}

// shopifyValidationError writes a 422 field-level validation error.
func shopifyValidationError(w http.ResponseWriter, field string, msgs []string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	json.NewEncoder(w).Encode(map[string]any{
		"errors": map[string]any{field: msgs},
	})
}
