package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

func (h *Handler) CreateShippingRate(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	displayName := r.FormValue("display_name")
	if displayName == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required param: display_name.")
		return
	}

	id := h.store.ShippingRates.NextID()
	sr := store.ShippingRate{
		ID:          id,
		Object:      "shipping_rate",
		Active:      true,
		DisplayName: displayName,
		Type:        "fixed_amount",
		Livemode:    false,
		Metadata:    parseMetadata(r),
		Created:     h.store.Now(),
	}

	if amtStr := r.FormValue("fixed_amount[amount]"); amtStr != "" {
		amt, _ := strconv.ParseInt(amtStr, 10, 64)
		sr.FixedAmount = &store.ShippingFixed{
			Amount:   amt,
			Currency: r.FormValue("fixed_amount[currency]"),
		}
	}

	h.store.ShippingRates.Set(id, sr)
	twincore.JSON(w, http.StatusOK, sr)
}

func (h *Handler) GetShippingRate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sr, ok := h.store.ShippingRates.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such shipping_rate: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, sr)
}

func (h *Handler) UpdateShippingRate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sr, ok := h.store.ShippingRates.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such shipping_rate: "+id)
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if v := r.FormValue("active"); v == "true" {
		sr.Active = true
	} else if v == "false" {
		sr.Active = false
	}
	if meta := parseMetadata(r); len(meta) > 0 {
		sr.Metadata = meta
	}

	h.store.ShippingRates.Set(id, sr)
	twincore.JSON(w, http.StatusOK, sr)
}

func (h *Handler) ListShippingRates(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.ShippingRates.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/shipping_rates", "has_more": page.HasMore, "data": page.Data,
	})
}
