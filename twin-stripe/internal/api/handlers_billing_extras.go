package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// --- Coupons ---

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
		Created:  h.store.Now(),
	}
	if v := r.FormValue("amount_off"); v != "" {
		coup.AmountOff, _ = strconv.ParseInt(v, 10, 64)
		coup.Currency = r.FormValue("currency")
	}
	if v := r.FormValue("percent_off"); v != "" {
		coup.PercentOff, _ = strconv.ParseFloat(v, 64)
	}
	if v := r.FormValue("duration_in_months"); v != "" {
		coup.DurationInMonths, _ = strconv.Atoi(v)
	}
	if v := r.FormValue("max_redemptions"); v != "" {
		coup.MaxRedemptions, _ = strconv.Atoi(v)
	}

	h.store.Coupons.Set(id, coup)
	h.emitEvent("coupon.created", mapFromJSON(coup))
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
	h.emitEvent("coupon.deleted", map[string]any{"id": id})
	twincore.JSON(w, http.StatusOK, map[string]any{"id": id, "object": "coupon", "deleted": true})
}

func (h *Handler) ListCoupons(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.Coupons.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/coupons", "has_more": page.HasMore, "data": page.Data,
	})
}

// --- Setup Intents ---

func (h *Handler) CreateSetupIntent(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	id := h.store.SetupIntents.NextID()
	si := store.SetupIntent{
		ID:           id,
		Object:       "setup_intent",
		Customer:     r.FormValue("customer"),
		Status:       "requires_payment_method",
		ClientSecret: id + "_secret_" + h.randomHex(12),
		Usage:        "off_session",
		Livemode:     false,
		Metadata:     parseMetadata(r),
		Created:      h.store.Now(),
	}
	if u := r.FormValue("usage"); u != "" {
		si.Usage = u
	}
	if pm := r.FormValue("payment_method"); pm != "" {
		si.PaymentMethod = pm
		si.Status = "requires_confirmation"
	}
	if r.FormValue("confirm") == "true" && si.PaymentMethod != "" {
		si.Status = "succeeded"
	}

	h.store.SetupIntents.Set(id, si)
	h.emitEvent("setup_intent.created", mapFromJSON(si))
	if si.Status == "succeeded" {
		h.emitEvent("setup_intent.succeeded", mapFromJSON(si))
	}
	twincore.JSON(w, http.StatusOK, si)
}

func (h *Handler) GetSetupIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	si, ok := h.store.SetupIntents.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such setup_intent: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, si)
}

func (h *Handler) ConfirmSetupIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	si, ok := h.store.SetupIntents.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such setup_intent: "+id)
		return
	}
	if err := parseFormOrJSON(r); err == nil {
		if pm := r.FormValue("payment_method"); pm != "" {
			si.PaymentMethod = pm
		}
	}
	si.Status = "succeeded"
	h.store.SetupIntents.Set(id, si)
	h.emitEvent("setup_intent.succeeded", mapFromJSON(si))
	twincore.JSON(w, http.StatusOK, si)
}

func (h *Handler) CancelSetupIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	si, ok := h.store.SetupIntents.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such setup_intent: "+id)
		return
	}
	si.Status = "canceled"
	h.store.SetupIntents.Set(id, si)
	h.emitEvent("setup_intent.canceled", mapFromJSON(si))
	twincore.JSON(w, http.StatusOK, si)
}

func (h *Handler) ListSetupIntents(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.SetupIntents.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/setup_intents", "has_more": page.HasMore, "data": page.Data,
	})
}

// --- Tax Rates ---

func (h *Handler) CreateTaxRate(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}
	displayName := r.FormValue("display_name")
	percentageStr := r.FormValue("percentage")
	if displayName == "" || percentageStr == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required params: display_name, percentage.")
		return
	}
	pct, _ := strconv.ParseFloat(percentageStr, 64)

	id := h.store.TaxRates.NextID()
	tr := store.TaxRate{
		ID:           id,
		Object:       "tax_rate",
		Active:       true,
		DisplayName:  displayName,
		Description:  r.FormValue("description"),
		Inclusive:    r.FormValue("inclusive") == "true",
		Percentage:   pct,
		Country:      r.FormValue("country"),
		State:        r.FormValue("state"),
		Jurisdiction: r.FormValue("jurisdiction"),
		Livemode:     false,
		Metadata:     parseMetadata(r),
		Created:      h.store.Now(),
	}

	h.store.TaxRates.Set(id, tr)
	twincore.JSON(w, http.StatusOK, tr)
}

func (h *Handler) GetTaxRate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tr, ok := h.store.TaxRates.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such tax_rate: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, tr)
}

func (h *Handler) ListTaxRates(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.TaxRates.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/tax_rates", "has_more": page.HasMore, "data": page.Data,
	})
}

// --- Disputes ---

func (h *Handler) GetDispute(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	dp, ok := h.store.Disputes.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such dispute: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, dp)
}

// UpdateDispute handles POST /v1/disputes/{id} — submit evidence.
func (h *Handler) UpdateDispute(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	dp, ok := h.store.Disputes.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such dispute: "+id)
		return
	}
	if dp.Status != "needs_response" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "dispute_not_updatable",
			"Dispute status is "+dp.Status+", which cannot accept evidence.")
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	// Parse evidence fields.
	if dp.Evidence == nil {
		dp.Evidence = make(map[string]string)
	}
	for _, field := range []string{
		"uncategorized_text", "customer_name", "customer_email_address",
		"product_description", "shipping_tracking_number",
	} {
		if v := r.FormValue("evidence[" + field + "]"); v != "" {
			dp.Evidence[field] = v
		}
	}

	if submit := r.FormValue("submit"); submit == "true" {
		dp.Status = "under_review"
		h.emitEvent("charge.dispute.updated", mapFromJSON(dp))
	}

	if meta := parseMetadata(r); len(meta) > 0 {
		dp.Metadata = meta
	}

	h.store.Disputes.Set(id, dp)
	h.emitEvent("charge.dispute.updated", mapFromJSON(dp))
	twincore.JSON(w, http.StatusOK, dp)
}

// CloseDispute handles POST /v1/disputes/{id}/close.
// Accepts the dispute (loses it). Won disputes are handled via admin.
func (h *Handler) CloseDispute(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	dp, ok := h.store.Disputes.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such dispute: "+id)
		return
	}
	if dp.Status == "won" || dp.Status == "lost" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "dispute_already_closed",
			"Dispute has already been closed with status: "+dp.Status)
		return
	}
	dp.Status = "lost"
	h.store.Disputes.Set(id, dp)
	h.emitEvent("charge.dispute.closed", mapFromJSON(dp))
	twincore.JSON(w, http.StatusOK, dp)
}

func (h *Handler) ListDisputes(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.Disputes.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/disputes", "has_more": page.HasMore, "data": page.Data,
	})
}
