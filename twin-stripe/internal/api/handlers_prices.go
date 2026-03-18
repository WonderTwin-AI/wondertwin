package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

func (h *Handler) CreatePrice(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	currency := r.FormValue("currency")
	product := r.FormValue("product")
	if currency == "" || product == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required params: currency, product.")
		return
	}

	id := h.store.Prices.NextID()
	price := store.Price{
		ID:            id,
		Object:        "price",
		Active:        true,
		Currency:      currency,
		Product:       product,
		Type:          "one_time",
		BillingScheme: "per_unit",
		Nickname:      r.FormValue("nickname"),
		Livemode:      false,
		Metadata:      parseMetadata(r),
		Created:       store.Now(),
	}

	if v := r.FormValue("unit_amount"); v != "" {
		if amt, err := strconv.ParseInt(v, 10, 64); err == nil {
			price.UnitAmount = amt
		}
	}

	// Recurring configuration.
	interval := r.FormValue("recurring[interval]")
	if interval != "" {
		price.Type = "recurring"
		intervalCount := 1
		if v := r.FormValue("recurring[interval_count]"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				intervalCount = n
			}
		}
		usageType := r.FormValue("recurring[usage_type]")
		if usageType == "" {
			usageType = "licensed"
		}
		price.Recurring = &store.PriceRecurring{
			Interval:      interval,
			IntervalCount: intervalCount,
			UsageType:     usageType,
		}
	}

	h.store.Prices.Set(id, price)
	h.emitEvent("price.created", mapFromJSON(price))
	twincore.JSON(w, http.StatusOK, price)
}

func (h *Handler) GetPrice(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	price, ok := h.store.Prices.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such price: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, price)
}

func (h *Handler) UpdatePrice(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	price, ok := h.store.Prices.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such price: "+id)
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if v := r.FormValue("active"); v == "false" {
		price.Active = false
	} else if v == "true" {
		price.Active = true
	}
	if v := r.FormValue("nickname"); v != "" {
		price.Nickname = v
	}
	if meta := parseMetadata(r); len(meta) > 0 {
		price.Metadata = meta
	}

	h.store.Prices.Set(id, price)
	h.emitEvent("price.updated", mapFromJSON(price))
	twincore.JSON(w, http.StatusOK, price)
}

func (h *Handler) ListPrices(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)

	// Optional product filter.
	productFilter := r.URL.Query().Get("product")
	if productFilter != "" {
		items := h.store.Prices.Filter(func(_ string, p store.Price) bool {
			return p.Product == productFilter
		})
		twincore.JSON(w, http.StatusOK, map[string]any{
			"object":   "list",
			"url":      "/v1/prices",
			"has_more": false,
			"data":     items,
		})
		return
	}

	page := h.store.Prices.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/prices",
		"has_more": page.HasMore,
		"data":     page.Data,
	})
}
