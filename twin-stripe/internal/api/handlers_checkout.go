package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// ── Checkout Sessions ──────────────────────────────────────────────

func (h *Handler) CreateCheckoutSession(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	id := h.store.CheckoutSessions.NextID()
	now := store.Now()

	mode := r.FormValue("mode")
	if mode == "" {
		mode = "payment"
	}

	paymentStatus := "unpaid"

	var amountTotal int64
	if v := r.FormValue("amount_total"); v != "" {
		amountTotal, _ = strconv.ParseInt(v, 10, 64)
	}

	cs := store.CheckoutSession{
		ID:            id,
		Object:        "checkout.session",
		Mode:          mode,
		Status:        "open",
		URL:           fmt.Sprintf("https://checkout.stripe.com/c/pay/%s", id),
		SuccessURL:    r.FormValue("success_url"),
		CancelURL:     r.FormValue("cancel_url"),
		Customer:      r.FormValue("customer"),
		CustomerEmail: r.FormValue("customer_email"),
		PaymentStatus: paymentStatus,
		Currency:      r.FormValue("currency"),
		AmountTotal:   amountTotal,
		ExpiresAt:     now + 86400, // 24 hours
		Livemode:      false,
		Metadata:      parseMetadata(r),
		Created:       now,
	}

	h.store.CheckoutSessions.Set(id, cs)
	h.dispatcher.Enqueue("checkout.session.created", mapFromJSON(cs))
	twincore.JSON(w, http.StatusOK, cs)
}

func (h *Handler) GetCheckoutSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cs, ok := h.store.CheckoutSessions.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such checkout session: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, cs)
}

func (h *Handler) ListCheckoutSessions(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.CheckoutSessions.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/checkout/sessions",
		"has_more": page.HasMore,
		"data":     page.Data,
	})
}

func (h *Handler) ExpireCheckoutSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cs, ok := h.store.CheckoutSessions.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such checkout session: "+id)
		return
	}
	if cs.Status != "open" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "resource_invalid",
			fmt.Sprintf("This Session can not be expired because it has a status of %s.", cs.Status))
		return
	}

	cs.Status = "expired"
	cs.URL = ""
	h.store.CheckoutSessions.Set(id, cs)
	h.dispatcher.Enqueue("checkout.session.expired", mapFromJSON(cs))
	twincore.JSON(w, http.StatusOK, cs)
}

// ── Payment Links ──────────────────────────────────────────────────

func (h *Handler) CreatePaymentLink(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	id := h.store.PaymentLinks.NextID()
	now := store.Now()

	pl := store.PaymentLink{
		ID:       id,
		Object:   "payment_link",
		Active:   true,
		URL:      fmt.Sprintf("https://buy.stripe.com/test_%s", id),
		Currency: r.FormValue("currency"),
		Livemode: false,
		Metadata: parseMetadata(r),
		Created:  now,
	}

	h.store.PaymentLinks.Set(id, pl)
	h.dispatcher.Enqueue("payment_link.created", mapFromJSON(pl))
	twincore.JSON(w, http.StatusOK, pl)
}

func (h *Handler) GetPaymentLink(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pl, ok := h.store.PaymentLinks.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment link: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, pl)
}

func (h *Handler) UpdatePaymentLink(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pl, ok := h.store.PaymentLinks.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment link: "+id)
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if v := r.FormValue("active"); v != "" {
		pl.Active = v == "true"
	}
	if meta := parseMetadata(r); len(meta) > 0 {
		pl.Metadata = meta
	}

	h.store.PaymentLinks.Set(id, pl)
	h.dispatcher.Enqueue("payment_link.updated", mapFromJSON(pl))
	twincore.JSON(w, http.StatusOK, pl)
}

func (h *Handler) ListPaymentLinks(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.PaymentLinks.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/payment_links",
		"has_more": page.HasMore,
		"data":     page.Data,
	})
}
