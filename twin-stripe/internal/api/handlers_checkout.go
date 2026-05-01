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
	now := h.store.Now()

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

	// Parse line items: line_items[0][price], line_items[0][quantity], etc.
	for i := 0; i < 20; i++ {
		priceID := r.FormValue("line_items[" + strconv.Itoa(i) + "][price]")
		if priceID == "" {
			break
		}
		qty := int64(1)
		if v := r.FormValue("line_items[" + strconv.Itoa(i) + "][quantity]"); v != "" {
			if q, err := strconv.ParseInt(v, 10, 64); err == nil {
				qty = q
			}
		}
		cs.LineItems = append(cs.LineItems, store.CheckoutLineItem{
			Price:    priceID,
			Quantity: qty,
		})

		// Compute amount_total from line items if not explicitly set.
		if amountTotal == 0 {
			if p, ok := h.store.Prices.Get(priceID); ok {
				cs.AmountTotal += p.UnitAmount * qty
			}
		}
	}

	h.store.CheckoutSessions.Set(id, cs)
	h.emitEvent("checkout.session.created", mapFromJSON(cs))
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
	h.emitEvent("checkout.session.expired", mapFromJSON(cs))
	twincore.JSON(w, http.StatusOK, cs)
}

// AdminCompleteCheckoutSession handles POST /admin/checkout/sessions/{id}/complete.
// This is an admin endpoint since real Stripe uses a hosted UI for completion.
func (h *Handler) AdminCompleteCheckoutSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cs, ok := h.store.CheckoutSessions.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such checkout session: "+id)
		return
	}
	if cs.Status != "open" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "resource_invalid",
			fmt.Sprintf("This Session can not be completed because it has a status of %s.", cs.Status))
		return
	}

	switch cs.Mode {
	case "payment":
		// Create a PaymentIntent + Charge.
		var amount int64
		currency := cs.Currency
		if currency == "" {
			currency = "usd"
		}
		if cs.AmountTotal > 0 {
			amount = cs.AmountTotal
		} else {
			// Sum from line items.
			for _, li := range cs.LineItems {
				if p, ok := h.store.Prices.Get(li.Price); ok {
					amount += p.UnitAmount * li.Quantity
				}
			}
		}

		piID := h.store.PaymentIntents.NextID()
		pi := store.PaymentIntent{
			ID:             piID,
			Object:         "payment_intent",
			Amount:         amount,
			AmountReceived: amount,
			Currency:       currency,
			Customer:       cs.Customer,
			Status:         "succeeded",
			CaptureMethod:  "automatic",
			ClientSecret:   piID + "_secret_" + h.randomHex(12),
			Livemode:       false,
			Created:        h.store.Now(),
		}
		chargeID := h.createChargeForPI(&pi)
		pi.LatestCharge = chargeID
		h.store.PaymentIntents.Set(piID, pi)
		h.store.CreditBalance("", currency, amount)
		h.store.RecordBalanceTransaction("charge", chargeID, currency, amount, 0)
		h.dispatcher.Enqueue("payment_intent.succeeded", mapFromJSON(pi))

		cs.PaymentIntent = piID
		cs.PaymentStatus = "paid"

	case "subscription":
		// Create subscription from line items.
		if len(cs.LineItems) > 0 {
			subID := h.store.Subscriptions.NextID()
			var items []store.SubscriptionItem
			now := h.store.Now()
			for i, li := range cs.LineItems {
				price, ok := h.store.Prices.Get(li.Price)
				if !ok {
					continue
				}
				items = append(items, store.SubscriptionItem{
					ID:       subID + "_si_" + strconv.Itoa(i),
					Object:   "subscription_item",
					Price:    price,
					Quantity: li.Quantity,
					Created:  now,
				})
			}

			sub := store.Subscription{
				ID:                 subID,
				Object:             "subscription",
				Customer:           cs.Customer,
				Status:             "active",
				CurrentPeriodStart: now,
				CurrentPeriodEnd:   now + 30*86400, // ~1 month
				CollectionMethod:   "charge_automatically",
				Livemode:           false,
				Created:            now,
				Items: &store.SubscriptionItems{
					Object: "list", Data: items, URL: "/v1/subscription_items?subscription=" + subID,
				},
			}

			invoiceID := h.createSubscriptionInvoice(&sub, items)
			sub.LatestInvoice = invoiceID
			h.store.Subscriptions.Set(subID, sub)
			h.dispatcher.Enqueue("customer.subscription.created", mapFromJSON(sub))

			cs.Subscription = subID
		}
		cs.PaymentStatus = "paid"
	}

	cs.Status = "complete"
	cs.URL = ""
	h.store.CheckoutSessions.Set(id, cs)
	h.dispatcher.Enqueue("checkout.session.completed", mapFromJSON(cs))
	twincore.JSON(w, http.StatusOK, cs)
}

// ── Payment Links ──────────────────────────────────────────────────

func (h *Handler) CreatePaymentLink(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	id := h.store.PaymentLinks.NextID()
	now := h.store.Now()

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
	h.emitEvent("payment_link.created", mapFromJSON(pl))
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
	h.emitEvent("payment_link.updated", mapFromJSON(pl))
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
