package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-shopify/internal/store"
)

// --- Inventory Items ---

func (h *Handler) ListInventoryItems(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"inventory_items": h.store.InventoryItems.List()})
}

func (h *Handler) GetInventoryItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	item, ok := h.store.InventoryItems.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"inventory_item": item})
}

func (h *Handler) UpdateInventoryItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	item, ok := h.store.InventoryItems.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		InventoryItem store.InventoryItem `json:"inventory_item"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.InventoryItem.SKU != "" {
		item.SKU = req.InventoryItem.SKU
	}
	if req.InventoryItem.Cost != "" {
		item.Cost = req.InventoryItem.Cost
	}
	if req.InventoryItem.CountryCodeOfOrigin != "" {
		item.CountryCodeOfOrigin = req.InventoryItem.CountryCodeOfOrigin
	}
	item.Tracked = req.InventoryItem.Tracked
	item.UpdatedAt = h.store.Now()
	h.store.InventoryItems.Set(id, item)
	shopifyJSON(w, http.StatusOK, map[string]any{"inventory_item": item})
}

// --- Inventory Levels ---

func (h *Handler) ListInventoryLevels(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"inventory_levels": h.store.InventoryLevels.List()})
}

func (h *Handler) AdjustInventoryLevel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InventoryItemID int64 `json:"inventory_item_id"`
		LocationID      int64 `json:"location_id"`
		Adjustment      int   `json:"available_adjustment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	key := fmt.Sprintf("%d-%d", req.InventoryItemID, req.LocationID)
	level, ok := h.store.InventoryLevels.Get(key)
	if !ok {
		level = store.InventoryLevel{
			InventoryItemID: req.InventoryItemID,
			LocationID:      req.LocationID,
		}
	}
	level.Available += req.Adjustment
	level.UpdatedAt = h.store.Now()
	h.store.InventoryLevels.Set(key, level)
	shopifyJSON(w, http.StatusOK, map[string]any{"inventory_level": level})
}

func (h *Handler) SetInventoryLevel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InventoryItemID int64 `json:"inventory_item_id"`
		LocationID      int64 `json:"location_id"`
		Available       int   `json:"available"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	key := fmt.Sprintf("%d-%d", req.InventoryItemID, req.LocationID)
	level := store.InventoryLevel{
		InventoryItemID: req.InventoryItemID,
		LocationID:      req.LocationID,
		Available:       req.Available,
		UpdatedAt:       h.store.Now(),
	}
	h.store.InventoryLevels.Set(key, level)
	shopifyJSON(w, http.StatusOK, map[string]any{"inventory_level": level})
}

func (h *Handler) ConnectInventoryLevel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InventoryItemID int64 `json:"inventory_item_id"`
		LocationID      int64 `json:"location_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	key := fmt.Sprintf("%d-%d", req.InventoryItemID, req.LocationID)
	level := store.InventoryLevel{
		InventoryItemID: req.InventoryItemID,
		LocationID:      req.LocationID,
		Available:       0,
		UpdatedAt:       h.store.Now(),
	}
	h.store.InventoryLevels.Set(key, level)
	shopifyJSON(w, http.StatusCreated, map[string]any{"inventory_level": level})
}

func (h *Handler) DeleteInventoryLevel(w http.ResponseWriter, r *http.Request) {
	itemID := r.URL.Query().Get("inventory_item_id")
	locID := r.URL.Query().Get("location_id")
	if itemID == "" || locID == "" {
		shopifyError(w, http.StatusBadRequest, "inventory_item_id and location_id are required")
		return
	}
	key := fmt.Sprintf("%s-%s", itemID, locID)
	if !h.store.InventoryLevels.Delete(key) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- Locations ---

func (h *Handler) ListLocations(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"locations": h.store.Locations.List()})
}

func (h *Handler) GetLocation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	loc, ok := h.store.Locations.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"location": loc})
}

// --- Fulfillments ---

func (h *Handler) ListFulfillments(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "order_id")
	oid, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	ffs := h.store.Fulfillments.Filter(func(_ string, f store.Fulfillment) bool {
		return f.OrderID == oid
	})
	shopifyJSON(w, http.StatusOK, map[string]any{"fulfillments": ffs})
}

func (h *Handler) CreateFulfillment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Fulfillment store.Fulfillment `json:"fulfillment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	f := req.Fulfillment
	f.ID = h.store.NextID()
	now := h.store.Now()
	f.CreatedAt = now
	f.UpdatedAt = now
	if f.Status == "" {
		f.Status = "success"
	}

	h.store.Fulfillments.Set(fmt.Sprintf("%d", f.ID), f)
	shopifyJSON(w, http.StatusCreated, map[string]any{"fulfillment": f})
}

// --- Fulfillment Orders ---

func (h *Handler) ListFulfillmentOrders(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "order_id")
	oid, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	fos := h.store.FulfillmentOrders.Filter(func(_ string, fo store.FulfillmentOrder) bool {
		return fo.OrderID == oid
	})
	shopifyJSON(w, http.StatusOK, map[string]any{"fulfillment_orders": fos})
}
