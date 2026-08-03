package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// ---------- Tokens ----------

func (h *Handler) CreateToken(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	id := h.store.Tokens.NextID()

	number := r.FormValue("card[number]")
	expMonthStr := r.FormValue("card[exp_month]")
	expYearStr := r.FormValue("card[exp_year]")
	// cvc is accepted but not stored

	var card *store.CardDetails
	if number != "" {
		expMonth, _ := strconv.Atoi(expMonthStr)
		expYear, _ := strconv.Atoi(expYearStr)

		last4 := number
		if len(number) >= 4 {
			last4 = number[len(number)-4:]
		}

		brand := "unknown"
		if strings.HasPrefix(number, "4") {
			brand = "visa"
		} else if strings.HasPrefix(number, "5") {
			brand = "mastercard"
		} else if strings.HasPrefix(number, "3") {
			brand = "amex"
		}

		card = &store.CardDetails{
			Brand:    brand,
			Last4:    last4,
			ExpMonth: expMonth,
			ExpYear:  expYear,
			Funding:  "credit",
		}
	}

	tok := store.Token{
		ID:       id,
		Object:   "token",
		Type:     "card",
		Card:     card,
		Used:     false,
		Livemode: false,
		Created:  h.store.Now(),
	}

	h.store.Tokens.Set(id, tok)
	twincore.JSON(w, http.StatusOK, tok)
}

func (h *Handler) GetToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tok, ok := h.store.Tokens.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing",
			"No such token: '"+id+"'")
		return
	}
	twincore.JSON(w, http.StatusOK, tok)
}

// ---------- Sources ----------

func (h *Handler) CreateSource(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	srcType := r.FormValue("type")
	if srcType == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing",
			"Missing required param: type.")
		return
	}

	id := h.store.Sources.NextID()

	amountStr := r.FormValue("amount")
	var amount int64
	if amountStr != "" {
		amount, _ = strconv.ParseInt(amountStr, 10, 64)
	}

	flow := r.FormValue("flow")
	if flow == "" {
		flow = "none"
	}

	src := store.Source{
		ID:       id,
		Object:   "source",
		Type:     srcType,
		Status:   "chargeable",
		Amount:   amount,
		Currency: r.FormValue("currency"),
		Customer: r.FormValue("customer"),
		Flow:     flow,
		Livemode: false,
		Metadata: parseMetadata(r),
		Created:  h.store.Now(),
	}

	h.store.Sources.Set(id, src)
	twincore.JSON(w, http.StatusOK, src)
}

func (h *Handler) GetSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	src, ok := h.store.Sources.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing",
			"No such source: '"+id+"'")
		return
	}
	twincore.JSON(w, http.StatusOK, src)
}

func (h *Handler) UpdateSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	src, ok := h.store.Sources.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing",
			"No such source: '"+id+"'")
		return
	}

	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if md := parseMetadata(r); md != nil {
		src.Metadata = md
	}

	h.store.Sources.Set(id, src)
	twincore.JSON(w, http.StatusOK, src)
}

// ---------- Mandates ----------

func (h *Handler) GetMandate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	mandate, ok := h.store.Mandates.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing",
			"No such mandate: '"+id+"'")
		return
	}
	twincore.JSON(w, http.StatusOK, mandate)
}

// ---------- Confirmation Tokens ----------

func (h *Handler) GetConfirmationToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ct, ok := h.store.ConfirmationTokens.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing",
			"No such confirmation token: '"+id+"'")
		return
	}
	twincore.JSON(w, http.StatusOK, ct)
}
