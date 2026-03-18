package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// ── Application Fees ─────────────────────────────────────────────────

// GetApplicationFee handles GET /v1/application_fees/{id}.
func (h *Handler) GetApplicationFee(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	fee, ok := h.store.ApplicationFees.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such application fee: '"+id+"'")
		return
	}
	twincore.JSON(w, http.StatusOK, fee)
}

// ListApplicationFees handles GET /v1/application_fees.
func (h *Handler) ListApplicationFees(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)

	page := h.store.ApplicationFees.Paginate(cursor, limit)

	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/application_fees",
		"data":     page.Data,
		"has_more": page.HasMore,
	})
}

// ── Application Fee Refunds ──────────────────────────────────────────

// CreateApplicationFeeRefund handles POST /v1/application_fees/{fee_id}/refunds.
func (h *Handler) CreateApplicationFeeRefund(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	feeID := chi.URLParam(r, "fee_id")
	fee, ok := h.store.ApplicationFees.Get(feeID)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such application fee: '"+feeID+"'")
		return
	}

	// Determine refund amount
	remaining := fee.Amount - fee.AmountRefunded
	amount := remaining
	if amtStr := r.FormValue("amount"); amtStr != "" {
		parsed, err := strconv.ParseInt(amtStr, 10, 64)
		if err != nil {
			twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_invalid",
				"Invalid integer: "+amtStr)
			return
		}
		if parsed > remaining {
			twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_invalid",
				"Refund amount exceeds remaining refundable amount.")
			return
		}
		amount = parsed
	}

	if amount <= 0 {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_invalid",
			"Application fee has already been fully refunded.")
		return
	}

	id := h.store.ApplicationFeeRefunds.NextID()
	refund := store.ApplicationFeeRefund{
		ID:       id,
		Object:   "fee_refund",
		Amount:   amount,
		Currency: fee.Currency,
		Fee:      feeID,
		Metadata: parseMetadata(r),
		Created:  store.Now(),
	}
	h.store.ApplicationFeeRefunds.Set(id, refund)

	// Update parent fee
	fee.AmountRefunded += amount
	if fee.AmountRefunded >= fee.Amount {
		fee.Refunded = true
	}
	h.store.ApplicationFees.Set(feeID, fee)

	h.emitEvent("application_fee.refunded", mapFromJSON(refund))

	twincore.JSON(w, http.StatusOK, refund)
}

// GetApplicationFeeRefund handles GET /v1/application_fees/{fee_id}/refunds/{id}.
func (h *Handler) GetApplicationFeeRefund(w http.ResponseWriter, r *http.Request) {
	feeID := chi.URLParam(r, "fee_id")
	if _, ok := h.store.ApplicationFees.Get(feeID); !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such application fee: '"+feeID+"'")
		return
	}

	id := chi.URLParam(r, "id")
	refund, ok := h.store.ApplicationFeeRefunds.Get(id)
	if !ok || refund.Fee != feeID {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such application fee refund: '"+id+"'")
		return
	}
	twincore.JSON(w, http.StatusOK, refund)
}

// ListApplicationFeeRefunds handles GET /v1/application_fees/{fee_id}/refunds.
func (h *Handler) ListApplicationFeeRefunds(w http.ResponseWriter, r *http.Request) {
	feeID := chi.URLParam(r, "fee_id")
	if _, ok := h.store.ApplicationFees.Get(feeID); !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such application fee: '"+feeID+"'")
		return
	}

	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)

	// Get all refunds and filter by fee
	page := h.store.ApplicationFeeRefunds.Paginate(cursor, limit)
	filtered := make([]store.ApplicationFeeRefund, 0)
	for _, ref := range page.Data {
		if ref.Fee == feeID {
			filtered = append(filtered, ref)
		}
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/application_fees/" + feeID + "/refunds",
		"data":     filtered,
		"has_more": false,
	})
}
