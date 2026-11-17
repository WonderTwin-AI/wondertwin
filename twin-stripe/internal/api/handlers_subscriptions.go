package api

import (
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func (h *Handler) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	customer := r.FormValue("customer")
	if customer == "" {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required param: customer.")
		return
	}
	cus, exists := h.store.Customers.Get(customer)
	if !exists {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "resource_missing", "No such customer: "+customer)
		return
	}
	if !h.requireTaxSetUp(w, r, false) {
		return
	}
	if automaticTaxRequested(r) && customerTaxCountry(cus) == "" {
		writeError(w, http.StatusBadRequest, apiError{Type: "invalid_request_error", Code: "customer_tax_location_invalid", Message: subscriptionLocationMessage})
		return
	}

	now := h.store.Clock.Now()
	id := h.store.StripeID(h.store.Subscriptions.NextID())

	sub := store.Subscription{
		ID:                   id,
		Object:               "subscription",
		Customer:             customer,
		Status:               "active",
		CurrentPeriodStart:   now.Unix(),
		CurrentPeriodEnd:     now.AddDate(0, 1, 0).Unix(), // default monthly
		CollectionMethod:     "charge_automatically",
		DefaultPaymentMethod: r.FormValue("default_payment_method"),
		Livemode:             false,
		Metadata:             parseMetadata(r),
		AutomaticTax:         subscriptionTax(automaticTaxRequested(r)),
		Created:              now.Unix(),
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
			stripeError(w, http.StatusBadRequest, "invalid_request_error", "resource_missing", "No such price: "+priceID)
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

	// Apply coupons: dahlia takes discounts[N][coupon]. Every coupon is
	// looked up before any is redeemed, so a bad entry changes nothing.
	sub.Discounts = []string{}
	couponIDs := indexedFormValues(r, "discounts", "coupon")
	coupons := make([]store.Coupon, 0, len(couponIDs))
	for _, couponID := range couponIDs {
		coup, ok := h.store.Coupons.Get(couponID)
		if !ok {
			stripeError(w, http.StatusBadRequest, "invalid_request_error", "resource_missing", "No such coupon: "+couponID)
			return
		}
		coupons = append(coupons, coup)
	}
	for _, coup := range coupons {
		coup.TimesRedeemed++
		h.store.Coupons.Set(coup.ID, coup)
		d := h.newDiscount(coup, customer, id, "")
		sub.Discounts = append(sub.Discounts, d.ID)
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
	h.emitEvent("customer.subscription.created", mapFromJSON(sub))

	twincore.JSON(w, http.StatusOK, sub)
}

func (h *Handler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sub, ok := h.store.Subscriptions.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such subscription: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, sub)
}

func (h *Handler) UpdateSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sub, ok := h.store.Subscriptions.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such subscription: "+id)
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if !h.requireTaxSetUp(w, r, false) {
		return
	}

	meta, ok := updateMetadata(w, r, sub.Metadata)
	if !ok {
		return
	}
	sub.Metadata = meta
	if v := r.FormValue("automatic_tax[enabled]"); v != "" {
		sub.AutomaticTax = subscriptionTax(v == "true")
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

	// Handle item changes (price/quantity updates) with proration.
	prorationBehavior := r.FormValue("proration_behavior")
	if prorationBehavior == "" {
		prorationBehavior = "create_prorations" // Stripe default
	}

	var newItems []store.SubscriptionItem
	hasItemChanges := false
	for i := 0; i < 10; i++ {
		if r.FormValue("items["+strconv.Itoa(i)+"][id]") != "" && sub.Items != nil {
			newItems = append(newItems, sub.Items.Data...)
			break
		}
	}
	for i := 0; i < 10; i++ {
		priceID := r.FormValue("items[" + strconv.Itoa(i) + "][price]")
		itemID := r.FormValue("items[" + strconv.Itoa(i) + "][id]")
		if priceID == "" && itemID == "" {
			break
		}
		hasItemChanges = true
		// items[N][id] changes an existing item, keeping what is not sent.
		var existing *store.SubscriptionItem
		if itemID != "" {
			for j := range newItems {
				if newItems[j].ID == itemID {
					existing = &newItems[j]
				}
			}
			if existing == nil {
				stripeError(w, http.StatusBadRequest, "invalid_request_error", "resource_missing", "No such subscription item: "+itemID)
				return
			}
		}
		var price store.Price
		if priceID == "" {
			price = existing.Price
		} else {
			p, ok := h.store.Prices.Get(priceID)
			if !ok {
				stripeError(w, http.StatusBadRequest, "invalid_request_error", "resource_missing", "No such price: "+priceID)
				return
			}
			price = p
		}
		qty := int64(1)
		if existing != nil {
			qty = existing.Quantity
		}
		if v := r.FormValue("items[" + strconv.Itoa(i) + "][quantity]"); v != "" {
			if q, err := strconv.ParseInt(v, 10, 64); err == nil {
				qty = q
			}
		}
		if existing != nil {
			existing.Price = price
			existing.Quantity = qty
			continue
		}
		siID := id + "_si_" + strconv.Itoa(len(newItems))
		newItems = append(newItems, store.SubscriptionItem{
			ID:       siID,
			Object:   "subscription_item",
			Price:    price,
			Quantity: qty,
			Created:  h.store.Now(),
		})
	}

	// always_invoice invoices the proration at once, as create_prorations
	// already does here.
	prorates := prorationBehavior == "create_prorations" || prorationBehavior == "always_invoice"
	if hasItemChanges && sub.Items != nil && prorates {
		now := h.store.Clock.Now().Unix()
		periodTotal := sub.CurrentPeriodEnd - sub.CurrentPeriodStart
		if periodTotal <= 0 {
			periodTotal = 1
		}
		remaining := sub.CurrentPeriodEnd - now
		if remaining < 0 {
			remaining = 0
		}
		fraction := float64(remaining) / float64(periodTotal)

		// Credit for unused time on old items.
		var oldTotal int64
		for _, item := range sub.Items.Data {
			oldTotal += item.Price.UnitAmount * item.Quantity
		}
		credit := int64(math.Round(float64(oldTotal) * fraction))

		// Charge for remaining time on new items.
		var newTotal int64
		for _, item := range newItems {
			newTotal += item.Price.UnitAmount * item.Quantity
		}
		charge := int64(math.Round(float64(newTotal) * fraction))

		prorationAmount := charge - credit

		// Generate a proration invoice if there's a net difference.
		if prorationAmount != 0 {
			invID := h.store.StripeID(h.store.Invoices.NextID())
			currency := "usd"
			// Proration lines sum the items; each takes the first item's
			// tax_behavior.
			var oldBehavior, newBehavior string
			if len(newItems) > 0 {
				currency = newItems[0].Price.Currency
				newBehavior = newItems[0].Price.TaxBehavior
			}
			if len(sub.Items.Data) > 0 {
				oldBehavior = sub.Items.Data[0].Price.TaxBehavior
			}

			var lines []store.InvoiceLine
			if credit > 0 {
				lines = append(lines, store.InvoiceLine{
					ID:          invID + "_il_credit",
					Object:      "line_item",
					Amount:      -credit,
					Currency:    currency,
					Description: "Unused time on previous plan",
					Type:        "invoiceitem",

					PriceTaxBehavior: oldBehavior,
				})
			}
			if charge > 0 {
				lines = append(lines, store.InvoiceLine{
					ID:          invID + "_il_charge",
					Object:      "line_item",
					Amount:      charge,
					Currency:    currency,
					Description: "Remaining time on new plan",
					Type:        "invoiceitem",

					PriceTaxBehavior: newBehavior,
				})
			}

			inv := store.Invoice{
				ID:                invID,
				Object:            "invoice",
				Customer:          sub.Customer,
				Subscription:      sub.ID,
				Status:            "paid",
				Total:             prorationAmount,
				TotalExcludingTax: &prorationAmount,
				Subtotal:          prorationAmount,
				AmountDue:         prorationAmount,
				AmountPaid:        prorationAmount,
				Currency:          currency,
				CollectionMethod:  sub.CollectionMethod,
				Paid:              true,
				Lines:             &store.InvoiceLines{Object: "list", Data: lines, URL: "/v1/invoices/" + invID + "/lines"},
				Number:            "INV-" + invID,
				Description:       "Proration",
				Livemode:          false,
				Created:           h.store.Now(),
			}
			if sub.AutomaticTax.Enabled {
				inv.AutomaticTax.Enabled = true
				h.applyInvoiceTax(&inv)
				inv.AmountDue, inv.AmountPaid, inv.AmountRemaining = inv.Total, inv.Total, 0
			}
			h.store.Invoices.Set(invID, inv)
			sub.LatestInvoice = invID
			h.emitEvent("invoice.created", mapFromJSON(inv))
			if inv.Paid {
				h.emitEvent("invoice.paid", mapFromJSON(inv))
			}
		}

		// Update subscription items.
		sub.Items = &store.SubscriptionItems{
			Object:  "list",
			Data:    newItems,
			HasMore: false,
			URL:     "/v1/subscription_items?subscription=" + id,
		}
	} else if hasItemChanges {
		// proration_behavior=none: just swap items, no invoice.
		sub.Items = &store.SubscriptionItems{
			Object:  "list",
			Data:    newItems,
			HasMore: false,
			URL:     "/v1/subscription_items?subscription=" + id,
		}
	}

	h.store.Subscriptions.Set(id, sub)
	h.emitEvent("customer.subscription.updated", mapFromJSON(sub))
	twincore.JSON(w, http.StatusOK, sub)
}

func (h *Handler) CancelSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sub, ok := h.store.Subscriptions.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such subscription: "+id)
		return
	}

	sub.Status = "canceled"
	sub.CanceledAt = h.store.Now()
	h.store.Subscriptions.Set(id, sub)
	h.emitEvent("customer.subscription.deleted", mapFromJSON(sub))
	twincore.JSON(w, http.StatusOK, sub)
}

func (h *Handler) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r, 10)
	customerFilter := r.URL.Query().Get("customer")
	priceFilter := r.URL.Query().Get("price")
	statusFilter := r.URL.Query().Get("status")
	page, ok := paginate(w, r, h.store.Subscriptions, "subscription", limit, func(s store.Subscription) bool {
		if customerFilter != "" && s.Customer != customerFilter {
			return false
		}
		if statusFilter != "" && statusFilter != "all" && s.Status != statusFilter {
			return false
		}
		if statusFilter == "" && s.Status == "canceled" {
			// Stripe omits canceled subscriptions unless status is given.
			return false
		}
		if priceFilter != "" {
			found := false
			if s.Items != nil {
				for _, it := range s.Items.Data {
					if it.Price.ID == priceFilter {
						found = true
					}
				}
			}
			return found
		}
		return true
	})
	if !ok {
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/subscriptions", "has_more": page.HasMore, "data": page.Data,
	})
}

func (h *Handler) createSubscriptionInvoice(sub *store.Subscription, items []store.SubscriptionItem) string {
	id := h.store.StripeID(h.store.Invoices.NextID())
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

			PriceTaxBehavior: item.Price.TaxBehavior,
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
	var discountIDs []string
	var totalDiscounts []store.DiscountAmount
	if coup, discountID := h.firstCoupon(sub.Discounts); coup != nil {
		if coup.PercentOff > 0 {
			discountAmount = int64(float64(total) * coup.PercentOff / 100)
		} else if coup.AmountOff > 0 {
			discountAmount = coup.AmountOff
			if discountAmount > total {
				discountAmount = total
			}
		}
		discountIDs = []string{discountID}
		totalDiscounts = []store.DiscountAmount{{Amount: discountAmount, Discount: discountID}}
	}

	inv := store.Invoice{
		ID:                   id,
		Object:               "invoice",
		Customer:             sub.Customer,
		Subscription:         sub.ID,
		Status:               "paid", // auto-paid for charge_automatically
		Subtotal:             total,
		Currency:             currency,
		CollectionMethod:     sub.CollectionMethod,
		Paid:                 true,
		Discounts:            discountIDs,
		TotalDiscountAmounts: totalDiscounts,
		DefaultTaxRates:      sub.DefaultTaxRates,
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
		Created:     h.store.Now(),
	}
	inv.AutomaticTax.Enabled = sub.AutomaticTax.Enabled
	h.applyInvoiceTax(&inv)
	invoiceTotal := inv.Total
	inv.AmountDue, inv.AmountPaid, inv.AmountRemaining = invoiceTotal, invoiceTotal, 0

	if sub.CollectionMethod == "send_invoice" {
		inv.Status = "open"
		inv.Paid = false
		inv.AmountPaid = 0
		inv.AmountRemaining = invoiceTotal
	}

	h.store.Invoices.Set(id, inv)
	h.emitEvent("invoice.created", mapFromJSON(inv))
	if inv.Paid {
		h.emitEvent("invoice.paid", mapFromJSON(inv))
	}
	return id
}
