package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

func (h *Handler) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	customer := r.FormValue("customer")
	if customer == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required param: customer.")
		return
	}

	now := time.Now()
	id := h.store.Subscriptions.NextID()

	sub := store.Subscription{
		ID:                 id,
		Object:             "subscription",
		Customer:           customer,
		Status:             "active",
		CurrentPeriodStart: now.Unix(),
		CurrentPeriodEnd:   now.AddDate(0, 1, 0).Unix(), // default monthly
		CollectionMethod:   "charge_automatically",
		DefaultPaymentMethod: r.FormValue("default_payment_method"),
		Livemode:           false,
		Metadata:           parseMetadata(r),
		Created:            now.Unix(),
	}

	if cm := r.FormValue("collection_method"); cm != "" {
		sub.CollectionMethod = cm
	}

	// Parse trial period.
	if days := r.FormValue("trial_period_days"); days != "" {
		if d, err := strconv.Atoi(days); err == nil && d > 0 {
			sub.Status = "trialing"
			sub.TrialStart = now.Unix()
			sub.TrialEnd = now.AddDate(0, 0, d).Unix()
		}
	}

	// Build subscription items from items[0][price], items[1][price], etc.
	var items []store.SubscriptionItem
	for i := 0; i < 10; i++ {
		priceID := r.FormValue("items[" + strconv.Itoa(i) + "][price]")
		if priceID == "" {
			break
		}
		price, ok := h.store.Prices.Get(priceID)
		if !ok {
			twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "resource_missing", "No such price: "+priceID)
			return
		}
		qty := int64(1)
		if v := r.FormValue("items[" + strconv.Itoa(i) + "][quantity]"); v != "" {
			if q, err := strconv.ParseInt(v, 10, 64); err == nil {
				qty = q
			}
		}

		// Set period end based on price interval.
		if price.Recurring != nil {
			switch price.Recurring.Interval {
			case "year":
				sub.CurrentPeriodEnd = now.AddDate(price.Recurring.IntervalCount, 0, 0).Unix()
			case "month":
				sub.CurrentPeriodEnd = now.AddDate(0, price.Recurring.IntervalCount, 0).Unix()
			case "week":
				sub.CurrentPeriodEnd = now.AddDate(0, 0, 7*price.Recurring.IntervalCount).Unix()
			case "day":
				sub.CurrentPeriodEnd = now.AddDate(0, 0, price.Recurring.IntervalCount).Unix()
			}
		}

		siID := id + "_si_" + strconv.Itoa(i)
		items = append(items, store.SubscriptionItem{
			ID:       siID,
			Object:   "subscription_item",
			Price:    price,
			Quantity: qty,
			Created:  now.Unix(),
		})
	}

	sub.Items = &store.SubscriptionItems{
		Object:  "list",
		Data:    items,
		HasMore: false,
		URL:     "/v1/subscription_items?subscription=" + id,
	}

	// Apply coupon if provided.
	if couponID := r.FormValue("coupon"); couponID != "" {
		coup, ok := h.store.Coupons.Get(couponID)
		if !ok {
			twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "resource_missing", "No such coupon: "+couponID)
			return
		}
		coup.TimesRedeemed++
		h.store.Coupons.Set(couponID, coup)
		sub.Discount = &store.Discount{
			ID:           "di_" + id,
			Object:       "discount",
			Coupon:       &coup,
			Customer:     customer,
			Subscription: id,
			Start:        now.Unix(),
		}
	}

	// Parse default tax rates.
	for i := 0; i < 10; i++ {
		trID := r.FormValue("default_tax_rates[" + strconv.Itoa(i) + "]")
		if trID == "" {
			break
		}
		tr, ok := h.store.TaxRates.Get(trID)
		if !ok {
			continue
		}
		sub.DefaultTaxRates = append(sub.DefaultTaxRates, tr)
	}

	// Create initial invoice.
	invoiceID := h.createSubscriptionInvoice(&sub, items)
	sub.LatestInvoice = invoiceID

	h.store.Subscriptions.Set(id, sub)
	h.dispatcher.Enqueue("customer.subscription.created", mapFromJSON(sub))
	twincore.JSON(w, http.StatusOK, sub)
}

func (h *Handler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sub, ok := h.store.Subscriptions.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such subscription: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, sub)
}

func (h *Handler) UpdateSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sub, ok := h.store.Subscriptions.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such subscription: "+id)
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if v := r.FormValue("cancel_at_period_end"); v == "true" {
		sub.CancelAtPeriodEnd = true
	} else if v == "false" {
		sub.CancelAtPeriodEnd = false
	}
	if v := r.FormValue("default_payment_method"); v != "" {
		sub.DefaultPaymentMethod = v
	}
	if v := r.FormValue("collection_method"); v != "" {
		sub.CollectionMethod = v
	}
	if meta := parseMetadata(r); len(meta) > 0 {
		sub.Metadata = meta
	}

	h.store.Subscriptions.Set(id, sub)
	h.dispatcher.Enqueue("customer.subscription.updated", mapFromJSON(sub))
	twincore.JSON(w, http.StatusOK, sub)
}

func (h *Handler) CancelSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sub, ok := h.store.Subscriptions.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such subscription: "+id)
		return
	}

	sub.Status = "canceled"
	sub.CanceledAt = store.Now()
	h.store.Subscriptions.Set(id, sub)
	h.dispatcher.Enqueue("customer.subscription.deleted", mapFromJSON(sub))
	twincore.JSON(w, http.StatusOK, sub)
}

func (h *Handler) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	customerFilter := r.URL.Query().Get("customer")
	if customerFilter != "" {
		items := h.store.Subscriptions.Filter(func(_ string, s store.Subscription) bool {
			return s.Customer == customerFilter
		})
		twincore.JSON(w, http.StatusOK, map[string]any{
			"object": "list", "url": "/v1/subscriptions", "has_more": false, "data": items,
		})
		return
	}
	page := h.store.Subscriptions.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/subscriptions", "has_more": page.HasMore, "data": page.Data,
	})
}

func (h *Handler) createSubscriptionInvoice(sub *store.Subscription, items []store.SubscriptionItem) string {
	id := h.store.Invoices.NextID()
	var total int64
	var lines []store.InvoiceLine
	for i, item := range items {
		lineAmt := item.Price.UnitAmount * item.Quantity
		total += lineAmt
		lines = append(lines, store.InvoiceLine{
			ID:       id + "_il_" + strconv.Itoa(i),
			Object:   "line_item",
			Amount:   lineAmt,
			Currency: item.Price.Currency,
			Price:    &item.Price,
			Quantity: item.Quantity,
			Type:     "subscription",
			Period: &store.Period{
				Start: sub.CurrentPeriodStart,
				End:   sub.CurrentPeriodEnd,
			},
		})
	}

	currency := "usd"
	if len(items) > 0 {
		currency = items[0].Price.Currency
	}

	// Apply discount from subscription.
	var discountAmount int64
	var discount *store.Discount
	if sub.Discount != nil && sub.Discount.Coupon != nil {
		coup := sub.Discount.Coupon
		if coup.PercentOff > 0 {
			discountAmount = int64(float64(total) * coup.PercentOff / 100)
		} else if coup.AmountOff > 0 {
			discountAmount = coup.AmountOff
			if discountAmount > total {
				discountAmount = total
			}
		}
		discount = sub.Discount
	}

	// Apply tax rates from subscription.
	var taxAmount int64
	taxableAmount := total - discountAmount
	var taxAmounts []store.TaxAmount
	for _, tr := range sub.DefaultTaxRates {
		lineTax := int64(float64(taxableAmount) * tr.Percentage / 100)
		taxAmounts = append(taxAmounts, store.TaxAmount{
			Amount:    lineTax,
			Inclusive: tr.Inclusive,
			TaxRate:   tr.ID,
		})
		if !tr.Inclusive {
			taxAmount += lineTax
		}
	}

	invoiceTotal := total - discountAmount + taxAmount

	inv := store.Invoice{
		ID:               id,
		Object:           "invoice",
		Customer:         sub.Customer,
		Subscription:     sub.ID,
		Status:           "paid", // auto-paid for charge_automatically
		AmountDue:        invoiceTotal,
		AmountPaid:       invoiceTotal,
		AmountRemaining:  0,
		Total:            invoiceTotal,
		Subtotal:         total,
		Currency:         currency,
		CollectionMethod: sub.CollectionMethod,
		Paid:             true,
		Discount:         discount,
		DefaultTaxRates:  sub.DefaultTaxRates,
		TotalTaxAmounts:  taxAmounts,
		Lines: &store.InvoiceLines{
			Object:  "list",
			Data:    lines,
			HasMore: false,
			URL:     "/v1/invoices/" + id + "/lines",
		},
		Number:      "INV-" + id,
		PeriodStart: sub.CurrentPeriodStart,
		PeriodEnd:   sub.CurrentPeriodEnd,
		Livemode:    false,
		Created:     store.Now(),
	}

	if discountAmount > 0 {
		inv.TotalDiscountAmounts = []store.DiscountAmount{{Amount: discountAmount, Discount: discount.ID}}
	}

	if sub.CollectionMethod == "send_invoice" {
		inv.Status = "open"
		inv.Paid = false
		inv.AmountPaid = 0
		inv.AmountRemaining = invoiceTotal
	}

	h.store.Invoices.Set(id, inv)
	h.dispatcher.Enqueue("invoice.created", mapFromJSON(inv))
	if inv.Paid {
		h.dispatcher.Enqueue("invoice.paid", mapFromJSON(inv))
	}
	return id
}
