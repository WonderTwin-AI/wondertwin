package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

func (h *Handler) CreatePaymentMethod(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	pmType := r.FormValue("type")
	if pmType == "" {
		pmType = "card"
	}

	id := h.store.PaymentMethods.NextID()
	pm := store.PaymentMethod{
		ID:       id,
		Object:   "payment_method",
		Type:     pmType,
		Livemode: false,
		Metadata: parseMetadata(r),
		Created:  store.Now(),
	}

	if pmType == "card" {
		expMonth := 12
		expYear := 2030
		if v := r.FormValue("card[exp_month]"); v != "" {
			expMonth, _ = strconv.Atoi(v)
		}
		if v := r.FormValue("card[exp_year]"); v != "" {
			expYear, _ = strconv.Atoi(v)
		}
		number := r.FormValue("card[number]")
		last4 := "4242"
		brand := "visa"
		if len(number) >= 4 {
			last4 = number[len(number)-4:]
			switch number[0] {
			case '4':
				brand = "visa"
			case '5':
				brand = "mastercard"
			case '3':
				brand = "amex"
			}
		}
		pm.Card = &store.CardDetails{
			Brand:    brand,
			Last4:    last4,
			ExpMonth: expMonth,
			ExpYear:  expYear,
			Funding:  "credit",
			Number:   number,
		}
	}

	if name := r.FormValue("billing_details[name]"); name != "" {
		pm.BillingDetails = &store.BillingDetails{
			Name:  name,
			Email: r.FormValue("billing_details[email]"),
			Phone: r.FormValue("billing_details[phone]"),
		}
	}

	h.store.PaymentMethods.Set(id, pm)
	twincore.JSON(w, http.StatusOK, pm)
}

func (h *Handler) GetPaymentMethod(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pm, ok := h.store.PaymentMethods.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment_method: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, pm)
}

func (h *Handler) AttachPaymentMethod(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pm, ok := h.store.PaymentMethods.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment_method: "+id)
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}
	customer := r.FormValue("customer")
	if customer == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required param: customer.")
		return
	}
	pm.Customer = customer
	h.store.PaymentMethods.Set(id, pm)
	twincore.JSON(w, http.StatusOK, pm)
}

func (h *Handler) DetachPaymentMethod(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pm, ok := h.store.PaymentMethods.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such payment_method: "+id)
		return
	}
	pm.Customer = ""
	h.store.PaymentMethods.Set(id, pm)
	twincore.JSON(w, http.StatusOK, pm)
}

func (h *Handler) ListPaymentMethods(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	customerFilter := r.URL.Query().Get("customer")
	if customerFilter != "" {
		items := h.store.PaymentMethods.Filter(func(_ string, pm store.PaymentMethod) bool {
			return pm.Customer == customerFilter
		})
		twincore.JSON(w, http.StatusOK, map[string]any{
			"object":   "list",
			"url":      "/v1/payment_methods",
			"has_more": false,
			"data":     items,
		})
		return
	}
	page := h.store.PaymentMethods.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/payment_methods",
		"has_more": page.HasMore,
		"data":     page.Data,
	})
}
