package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// --- Quotes ---

func (h *Handler) CreateQuote(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	id := h.store.Quotes.NextID()
	now := store.Now()

	qt := store.Quote{
		ID:          id,
		Object:      "quote",
		Customer:    r.FormValue("customer"),
		Status:      "draft",
		Currency:    r.FormValue("currency"),
		Description: r.FormValue("description"),
		Header:      r.FormValue("header"),
		Footer:      r.FormValue("footer"),
		Livemode:    false,
		Metadata:    parseMetadata(r),
		Created:     now,
	}

	if v := r.FormValue("amount_total"); v != "" {
		qt.AmountTotal, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := r.FormValue("amount_subtotal"); v != "" {
		qt.AmountSubtotal, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := r.FormValue("expires_at"); v != "" {
		qt.ExpiresAt, _ = strconv.ParseInt(v, 10, 64)
	}

	h.store.Quotes.Set(id, qt)
	h.emitEvent("quote.created", mapFromJSON(qt))
	twincore.JSON(w, http.StatusOK, qt)
}

func (h *Handler) GetQuote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	qt, ok := h.store.Quotes.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such quote: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, qt)
}

func (h *Handler) UpdateQuote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	qt, ok := h.store.Quotes.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such quote: "+id)
		return
	}
	if qt.Status != "draft" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_operation", "Quote can only be updated in draft status.")
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if v := r.FormValue("description"); v != "" {
		qt.Description = v
	}
	if v := r.FormValue("header"); v != "" {
		qt.Header = v
	}
	if v := r.FormValue("footer"); v != "" {
		qt.Footer = v
	}
	if v := r.FormValue("customer"); v != "" {
		qt.Customer = v
	}
	if meta := parseMetadata(r); len(meta) > 0 {
		qt.Metadata = meta
	}

	h.store.Quotes.Set(id, qt)
	twincore.JSON(w, http.StatusOK, qt)
}

func (h *Handler) FinalizeQuote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	qt, ok := h.store.Quotes.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such quote: "+id)
		return
	}
	if qt.Status != "draft" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_operation", "Quote can only be finalized from draft status.")
		return
	}

	qt.Status = "open"
	qt.ExpiresAt = time.Now().AddDate(0, 0, 30).Unix()
	h.store.Quotes.Set(id, qt)
	h.emitEvent("quote.finalized", mapFromJSON(qt))
	twincore.JSON(w, http.StatusOK, qt)
}

func (h *Handler) AcceptQuote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	qt, ok := h.store.Quotes.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such quote: "+id)
		return
	}
	if qt.Status != "open" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_operation", "Quote can only be accepted from open status.")
		return
	}

	qt.Status = "accepted"
	h.store.Quotes.Set(id, qt)
	h.emitEvent("quote.accepted", mapFromJSON(qt))
	twincore.JSON(w, http.StatusOK, qt)
}

func (h *Handler) CancelQuote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	qt, ok := h.store.Quotes.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such quote: "+id)
		return
	}
	if qt.Status != "draft" && qt.Status != "open" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_operation", "Quote can only be canceled from draft or open status.")
		return
	}

	qt.Status = "canceled"
	h.store.Quotes.Set(id, qt)
	h.emitEvent("quote.canceled", mapFromJSON(qt))
	twincore.JSON(w, http.StatusOK, qt)
}

func (h *Handler) ListQuotes(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.Quotes.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/quotes", "has_more": page.HasMore, "data": page.Data,
	})
}

// --- Billing Portal Sessions ---

func (h *Handler) CreateBillingPortalSession(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	customer := r.FormValue("customer")
	if customer == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required param: customer.")
		return
	}

	id := h.store.BillingPortalSessions.NextID()
	bps := store.BillingPortalSession{
		ID:        id,
		Object:    "billing_portal.session",
		Customer:  customer,
		URL:       fmt.Sprintf("https://billing.stripe.com/session/%s", id),
		ReturnURL: r.FormValue("return_url"),
		Livemode:  false,
		Created:   store.Now(),
	}

	h.store.BillingPortalSessions.Set(id, bps)
	twincore.JSON(w, http.StatusOK, bps)
}
