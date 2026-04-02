package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-shopify/internal/store"
)

// --- Orders ---

func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"orders": h.store.Orders.List()})
}

func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	o, ok := h.store.Orders.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"order": o})
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Order store.Order `json:"order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	o := req.Order
	o.ID = h.store.NextID()
	orderNum := h.store.NextOrderNumber()
	o.OrderNumber = orderNum
	o.Name = fmt.Sprintf("#%d", orderNum)
	now := h.store.Now()
	o.CreatedAt = now
	o.UpdatedAt = now
	if o.FinancialStatus == "" {
		o.FinancialStatus = "pending"
	}
	if o.Currency == "" {
		o.Currency = "USD"
	}
	if o.TotalPrice == "" {
		o.TotalPrice = "0.00"
	}
	if o.SubtotalPrice == "" {
		o.SubtotalPrice = "0.00"
	}
	if o.TotalTax == "" {
		o.TotalTax = "0.00"
	}
	if o.TotalDiscounts == "" {
		o.TotalDiscounts = "0.00"
	}

	// Assign IDs to line items
	for i := range o.LineItems {
		o.LineItems[i].ID = h.store.NextID()
	}

	storeID := fmt.Sprintf("%d", o.ID)
	h.store.Orders.Set(storeID, o)
	shopifyJSON(w, http.StatusCreated, map[string]any{"order": o})
}

func (h *Handler) UpdateOrder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	o, ok := h.store.Orders.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Order store.Order `json:"order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Order.Email != "" {
		o.Email = req.Order.Email
	}
	if req.Order.Note != "" {
		o.Note = req.Order.Note
	}
	if req.Order.Tags != "" {
		o.Tags = req.Order.Tags
	}
	if req.Order.ShippingAddress != nil {
		o.ShippingAddress = req.Order.ShippingAddress
	}
	if req.Order.BillingAddress != nil {
		o.BillingAddress = req.Order.BillingAddress
	}
	o.UpdatedAt = h.store.Now()
	h.store.Orders.Set(id, o)
	shopifyJSON(w, http.StatusOK, map[string]any{"order": o})
}

func (h *Handler) DeleteOrder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.Orders.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) CloseOrder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	o, ok := h.store.Orders.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	now := h.store.Now()
	o.ClosedAt = now
	o.UpdatedAt = now
	h.store.Orders.Set(id, o)
	shopifyJSON(w, http.StatusOK, map[string]any{"order": o})
}

func (h *Handler) OpenOrder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	o, ok := h.store.Orders.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	o.ClosedAt = ""
	o.UpdatedAt = h.store.Now()
	h.store.Orders.Set(id, o)
	shopifyJSON(w, http.StatusOK, map[string]any{"order": o})
}

func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	o, ok := h.store.Orders.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	now := h.store.Now()
	o.Cancelled = true
	o.CancelledAt = now
	o.UpdatedAt = now
	o.FinancialStatus = "voided"
	h.store.Orders.Set(id, o)
	shopifyJSON(w, http.StatusOK, map[string]any{"order": o})
}

func (h *Handler) CountOrders(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"count": h.store.Orders.Count()})
}

// --- Transactions ---

func (h *Handler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "order_id")
	oid, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	txns := h.store.Transactions.Filter(func(_ string, t store.Transaction) bool {
		return t.OrderID == oid
	})
	shopifyJSON(w, http.StatusOK, map[string]any{"transactions": txns})
}

func (h *Handler) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "order_id")
	oid, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	_, ok := h.store.Orders.Get(orderID)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Transaction store.Transaction `json:"transaction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	t := req.Transaction
	t.ID = h.store.NextID()
	t.OrderID = oid
	t.CreatedAt = h.store.Now()
	if t.Kind == "" {
		t.Kind = "sale"
	}
	if t.Status == "" {
		t.Status = "success"
	}
	if t.Currency == "" {
		t.Currency = "USD"
	}
	if t.Gateway == "" {
		t.Gateway = "bogus"
	}

	h.store.Transactions.Set(fmt.Sprintf("%d", t.ID), t)
	shopifyJSON(w, http.StatusCreated, map[string]any{"transaction": t})
}

func (h *Handler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, ok := h.store.Transactions.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"transaction": t})
}

// --- Refunds ---

func (h *Handler) ListRefunds(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "order_id")
	oid, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	refunds := h.store.Refunds.Filter(func(_ string, ref store.Refund) bool {
		return ref.OrderID == oid
	})
	shopifyJSON(w, http.StatusOK, map[string]any{"refunds": refunds})
}

func (h *Handler) CreateRefund(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "order_id")
	oid, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	_, ok := h.store.Orders.Get(orderID)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Refund store.Refund `json:"refund"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	ref := req.Refund
	ref.ID = h.store.NextID()
	ref.OrderID = oid
	ref.CreatedAt = h.store.Now()

	h.store.Refunds.Set(fmt.Sprintf("%d", ref.ID), ref)
	shopifyJSON(w, http.StatusCreated, map[string]any{"refund": ref})
}

func (h *Handler) GetRefund(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ref, ok := h.store.Refunds.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"refund": ref})
}
