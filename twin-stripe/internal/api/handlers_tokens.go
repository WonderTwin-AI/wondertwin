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
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if hasBankAccountParams(r) {
		h.createBankAccountToken(w, r)
		return
	}

	id := h.store.StripeID(h.store.Tokens.NextID())

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

// hasBankAccountParams reports whether the request describes a bank account.
func hasBankAccountParams(r *http.Request) bool {
	for k := range r.Form {
		if strings.HasPrefix(k, "bank_account[") {
			return true
		}
	}
	return false
}

// createBankAccountToken creates a btok_ token from bank_account[...], as
// POST /v1/tokens does. account_number and country are required; the bank
// name is Stripe's test bank, as Stripe's reference example shows.
func (h *Handler) createBankAccountToken(w http.ResponseWriter, r *http.Request) {
	accountNumber := r.FormValue("bank_account[account_number]")
	country := strings.ToUpper(r.FormValue("bank_account[country]"))
	if accountNumber == "" || country == "" {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing",
			missingParamMessage(r, "bank_account[account_number]", "bank_account[country]"))
		return
	}
	currency := strings.ToLower(r.FormValue("bank_account[currency]"))
	if currency == "" {
		currency = "usd"
	}
	last4 := accountNumber
	if len(accountNumber) >= 4 {
		last4 = accountNumber[len(accountNumber)-4:]
	}
	routingNumber := r.FormValue("bank_account[routing_number]")
	tok := store.Token{
		ID:     h.store.StripeID("b" + h.store.Tokens.NextID()),
		Object: "token",
		Type:   "bank_account",
		BankAccount: &store.BankAccountDetails{
			ID:                h.store.StripeID(h.store.ExternalAccts.NextID()),
			Object:            "bank_account",
			AccountHolderName: r.FormValue("bank_account[account_holder_name]"),
			AccountHolderType: r.FormValue("bank_account[account_holder_type]"),
			BankName:          testBankName,
			Country:           country,
			Currency:          currency,
			Fingerprint:       bankFingerprint(routingNumber, accountNumber),
			Last4:             last4,
			RoutingNumber:     routingNumber,
			Status:            "new",
		},
		Created: h.store.Now(),
	}
	h.store.Tokens.Set(tok.ID, tok)
	twincore.JSON(w, http.StatusOK, tok)
}

func (h *Handler) GetToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tok, ok := h.store.Tokens.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing",
			"No such token: '"+id+"'")
		return
	}
	twincore.JSON(w, http.StatusOK, tok)
}

// ---------- Sources ----------

func (h *Handler) CreateSource(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	srcType := r.FormValue("type")
	if srcType == "" {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing",
			"Missing required param: type.")
		return
	}

	id := h.store.StripeID(h.store.Sources.NextID())

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
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing",
			"No such source: '"+id+"'")
		return
	}
	twincore.JSON(w, http.StatusOK, src)
}

func (h *Handler) UpdateSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	src, ok := h.store.Sources.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing",
			"No such source: '"+id+"'")
		return
	}

	if err := parseFormOrJSON(r); err != nil {
		stripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	meta, ok := updateMetadata(w, r, src.Metadata)
	if !ok {
		return
	}
	src.Metadata = meta

	h.store.Sources.Set(id, src)
	twincore.JSON(w, http.StatusOK, src)
}

// ---------- Mandates ----------

func (h *Handler) GetMandate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	mandate, ok := h.store.Mandates.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing",
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
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing",
			"No such confirmation token: '"+id+"'")
		return
	}
	twincore.JSON(w, http.StatusOK, ct)
}
