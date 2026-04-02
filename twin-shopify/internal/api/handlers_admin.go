package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// AdminListProducts handles GET /admin/products
func (h *Handler) AdminListProducts(w http.ResponseWriter, r *http.Request) {
	products := h.store.Products.List()
	twincore.JSON(w, http.StatusOK, map[string]any{
		"products": products,
		"total":    len(products),
	})
}

// AdminListOrders handles GET /admin/orders
func (h *Handler) AdminListOrders(w http.ResponseWriter, r *http.Request) {
	orders := h.store.Orders.List()
	twincore.JSON(w, http.StatusOK, map[string]any{
		"orders": orders,
		"total":  len(orders),
	})
}

// AdminListCustomers handles GET /admin/customers
func (h *Handler) AdminListCustomers(w http.ResponseWriter, r *http.Request) {
	customers := h.store.Customers.List()
	twincore.JSON(w, http.StatusOK, map[string]any{
		"customers": customers,
		"total":     len(customers),
	})
}
