package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

func (h *Handler) CreatePromotionCode(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	coupon := r.FormValue("coupon")
	if coupon == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required param: coupon.")
		return
	}

	code := r.FormValue("code")
	if code == "" {
		code = strings.ToUpper(randomHex(4))
	}

	id := h.store.PromotionCodes.NextID()
	pc := store.PromotionCode{
		ID:       id,
		Object:   "promotion_code",
		Code:     code,
		Coupon:   coupon,
		Active:   true,
		Livemode: false,
		Metadata: parseMetadata(r),
		Created:  store.Now(),
	}

	if v := r.FormValue("max_redemptions"); v != "" {
		pc.MaxRedemptions, _ = strconv.Atoi(v)
	}
	if v := r.FormValue("expires_at"); v != "" {
		pc.ExpiresAt, _ = strconv.ParseInt(v, 10, 64)
	}

	h.store.PromotionCodes.Set(id, pc)
	h.dispatcher.Enqueue("promotion_code.created", mapFromJSON(pc))
	twincore.JSON(w, http.StatusOK, pc)
}

func (h *Handler) GetPromotionCode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pc, ok := h.store.PromotionCodes.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such promotion code: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, pc)
}

func (h *Handler) UpdatePromotionCode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pc, ok := h.store.PromotionCodes.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such promotion code: "+id)
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if v := r.FormValue("active"); v == "true" {
		pc.Active = true
	} else if v == "false" {
		pc.Active = false
	}
	if meta := parseMetadata(r); len(meta) > 0 {
		pc.Metadata = meta
	}

	h.store.PromotionCodes.Set(id, pc)
	h.dispatcher.Enqueue("promotion_code.updated", mapFromJSON(pc))
	twincore.JSON(w, http.StatusOK, pc)
}

func (h *Handler) ListPromotionCodes(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.PromotionCodes.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/promotion_codes", "has_more": page.HasMore, "data": page.Data,
	})
}
