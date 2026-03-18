package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

func (h *Handler) CreateCoupon(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	duration := r.FormValue("duration")
	if duration == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required param: duration.")
		return
	}

	id := r.FormValue("id")
	if id == "" {
		id = h.store.Coupons.NextID()
	}

	coup := store.Coupon{
		ID:       id,
		Object:   "coupon",
		Duration: duration,
		Name:     r.FormValue("name"),
		Valid:    true,
		Livemode: false,
		Metadata: parseMetadata(r),
		Created:  store.Now(),
	}

	if v := r.FormValue("amount_off"); v != "" {
		if a, err := strconv.ParseInt(v, 10, 64); err == nil {
			coup.AmountOff = a
		}
	}
	if v := r.FormValue("percent_off"); v != "" {
		if p, err := strconv.ParseFloat(v, 64); err == nil {
			coup.PercentOff = p
		}
	}
	if v := r.FormValue("currency"); v != "" {
		coup.Currency = v
	}
	if v := r.FormValue("duration_in_months"); v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			coup.DurationInMonths = d
		}
	}
	if v := r.FormValue("max_redemptions"); v != "" {
		if m, err := strconv.Atoi(v); err == nil {
			coup.MaxRedemptions = m
		}
	}

	h.store.Coupons.Set(id, coup)
	h.dispatcher.Enqueue("coupon.created", mapFromJSON(coup))
	twincore.JSON(w, http.StatusOK, coup)
}

func (h *Handler) GetCoupon(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	coup, ok := h.store.Coupons.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such coupon: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, coup)
}

func (h *Handler) DeleteCoupon(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.Coupons.Delete(id) {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such coupon: "+id)
		return
	}
	h.dispatcher.Enqueue("coupon.deleted", map[string]any{"id": id})
	twincore.JSON(w, http.StatusOK, map[string]any{"id": id, "object": "coupon", "deleted": true})
}

func (h *Handler) ListCoupons(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.Coupons.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/coupons",
		"has_more": page.HasMore,
		"data":     page.Data,
	})
}
