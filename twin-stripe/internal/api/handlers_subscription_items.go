package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func (h *Handler) CreateSubscriptionItem(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	subID := r.FormValue("subscription")
	priceID := r.FormValue("price")
	if subID == "" || priceID == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required params: subscription, price.")
		return
	}

	sub, ok := h.store.Subscriptions.Get(subID)
	if !ok {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "resource_missing", "No such subscription: "+subID)
		return
	}

	price, ok := h.store.Prices.Get(priceID)
	if !ok {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "resource_missing", "No such price: "+priceID)
		return
	}

	qty := int64(1)
	if v := r.FormValue("quantity"); v != "" {
		if q, err := strconv.ParseInt(v, 10, 64); err == nil {
			qty = q
		}
	}

	id := h.store.SubItems.NextID()
	si := store.SubscriptionItem{
		ID:           id,
		Object:       "subscription_item",
		Price:        price,
		Quantity:     qty,
		Subscription: subID,
		Metadata:     parseMetadata(r),
		Created:      h.store.Now(),
	}

	h.store.SubItems.Set(id, si)

	// Update parent subscription's items list.
	if sub.Items == nil {
		sub.Items = &store.SubscriptionItems{
			Object: "list",
			URL:    "/v1/subscription_items?subscription=" + subID,
		}
	}
	sub.Items.Data = append(sub.Items.Data, si)
	h.store.Subscriptions.Set(subID, sub)

	h.emitEvent("customer.subscription.updated", mapFromJSON(sub))
	twincore.JSON(w, http.StatusOK, si)
}

func (h *Handler) GetSubscriptionItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	si, ok := h.store.SubItems.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such subscription item: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, si)
}

func (h *Handler) UpdateSubscriptionItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	si, ok := h.store.SubItems.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such subscription item: "+id)
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if v := r.FormValue("quantity"); v != "" {
		if q, err := strconv.ParseInt(v, 10, 64); err == nil {
			si.Quantity = q
		}
	}
	if priceID := r.FormValue("price"); priceID != "" {
		if price, ok := h.store.Prices.Get(priceID); ok {
			si.Price = price
		}
	}
	if meta := parseMetadata(r); len(meta) > 0 {
		si.Metadata = meta
	}

	h.store.SubItems.Set(id, si)
	twincore.JSON(w, http.StatusOK, si)
}

func (h *Handler) DeleteSubscriptionItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	si, ok := h.store.SubItems.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such subscription item: "+id)
		return
	}

	// Remove from parent subscription's items list.
	if si.Subscription != "" {
		if sub, ok := h.store.Subscriptions.Get(si.Subscription); ok && sub.Items != nil {
			var updated []store.SubscriptionItem
			for _, item := range sub.Items.Data {
				if item.ID != id {
					updated = append(updated, item)
				}
			}
			sub.Items.Data = updated
			h.store.Subscriptions.Set(si.Subscription, sub)
		}
	}

	h.store.SubItems.Delete(id)
	twincore.JSON(w, http.StatusOK, map[string]any{"id": id, "object": "subscription_item", "deleted": true})
}

func (h *Handler) ListSubscriptionItems(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	subFilter := r.URL.Query().Get("subscription")
	if subFilter != "" {
		items := h.store.SubItems.Filter(func(_ string, si store.SubscriptionItem) bool {
			return si.Subscription == subFilter
		})
		twincore.JSON(w, http.StatusOK, map[string]any{
			"object": "list", "url": "/v1/subscription_items", "has_more": false, "data": items,
		})
		return
	}
	page := h.store.SubItems.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/subscription_items", "has_more": page.HasMore, "data": page.Data,
	})
}
