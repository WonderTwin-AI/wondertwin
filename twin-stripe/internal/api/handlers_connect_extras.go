package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// ── Transfer Reversals ───────────────────────────────────────────────

// CreateTransferReversal handles POST /v1/transfers/{transfer_id}/reversals.
func (h *Handler) CreateTransferReversal(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	transferID := chi.URLParam(r, "transfer_id")
	transfer, ok := h.store.Transfers.Get(transferID)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such transfer: '"+transferID+"'")
		return
	}

	// Determine reversal amount
	remaining := transfer.Amount - transfer.AmountReversed
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
				"Reversal amount exceeds remaining reversible amount.")
			return
		}
		amount = parsed
	}

	if amount <= 0 {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_invalid",
			"Transfer has already been fully reversed.")
		return
	}

	// Credit platform balance (reverse the original debit)
	h.store.CreditBalance("", transfer.Currency, amount)

	// Debit destination account balance (reverse the original credit)
	if err := h.store.DebitBalance(transfer.Destination, transfer.Currency, amount); err != nil {
		slog.Warn("destination balance insufficient for transfer reversal", "amount", amount, "error", err)
	}

	// Record balance transaction
	btID := h.store.RecordBalanceTransaction("transfer_reversal", transferID, transfer.Currency, amount, 0)

	id := h.store.TransferReversals.NextID()
	reversal := store.TransferReversal{
		ID:                 id,
		Object:             "transfer_reversal",
		Amount:             amount,
		Currency:           transfer.Currency,
		Transfer:           transferID,
		BalanceTransaction: btID,
		Metadata:           parseMetadata(r),
		Created:            h.store.Now(),
	}
	h.store.TransferReversals.Set(id, reversal)

	// Update parent transfer
	transfer.AmountReversed += amount
	if transfer.AmountReversed >= transfer.Amount {
		transfer.Reversed = true
	}
	h.store.Transfers.Set(transferID, transfer)

	h.emitEvent("transfer.reversed", mapFromJSON(reversal))

	twincore.JSON(w, http.StatusOK, reversal)
}

// GetTransferReversal handles GET /v1/transfers/{transfer_id}/reversals/{id}.
func (h *Handler) GetTransferReversal(w http.ResponseWriter, r *http.Request) {
	transferID := chi.URLParam(r, "transfer_id")
	if _, ok := h.store.Transfers.Get(transferID); !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such transfer: '"+transferID+"'")
		return
	}

	id := chi.URLParam(r, "id")
	reversal, ok := h.store.TransferReversals.Get(id)
	if !ok || reversal.Transfer != transferID {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such transfer reversal: '"+id+"'")
		return
	}
	twincore.JSON(w, http.StatusOK, reversal)
}

// ListTransferReversals handles GET /v1/transfers/{transfer_id}/reversals.
func (h *Handler) ListTransferReversals(w http.ResponseWriter, r *http.Request) {
	transferID := chi.URLParam(r, "transfer_id")
	if _, ok := h.store.Transfers.Get(transferID); !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such transfer: '"+transferID+"'")
		return
	}

	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)

	page := h.store.TransferReversals.Paginate(cursor, limit)
	filtered := make([]store.TransferReversal, 0)
	for _, rev := range page.Data {
		if rev.Transfer == transferID {
			filtered = append(filtered, rev)
		}
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/transfers/" + transferID + "/reversals",
		"data":     filtered,
		"has_more": false,
	})
}

// ── Account Links ────────────────────────────────────────────────────

// CreateAccountLink handles POST /v1/account_links.
func (h *Handler) CreateAccountLink(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	account := r.FormValue("account")
	if account == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing",
			"Missing required param: account.")
		return
	}

	linkType := r.FormValue("type")
	if linkType == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing",
			"Missing required param: type.")
		return
	}
	if linkType != "account_onboarding" && linkType != "account_update" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_invalid",
			"Invalid type: must be 'account_onboarding' or 'account_update'.")
		return
	}

	// Verify account exists
	if _, ok := h.store.Accounts.Get(account); !ok {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "resource_missing",
			"No such account: '"+account+"'")
		return
	}

	now := h.store.Now()
	link := store.AccountLink{
		Object:    "account_link",
		URL:       fmt.Sprintf("https://connect.stripe.com/setup/e/%s/%s", account, h.randomHex(16)),
		ExpiresAt: now + 300, // 5 minutes
		Created:   now,
	}

	// Ephemeral - not stored
	twincore.JSON(w, http.StatusOK, link)
}

// ── Persons ──────────────────────────────────────────────────────────

// CreatePerson handles POST /v1/accounts/{account_id}/persons.
func (h *Handler) CreatePerson(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	accountID := chi.URLParam(r, "account_id")
	if _, ok := h.store.Accounts.Get(accountID); !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such account: '"+accountID+"'")
		return
	}

	id := h.store.Persons.NextID()
	person := store.Person{
		ID:        id,
		Object:    "person",
		Account:   accountID,
		FirstName: r.FormValue("first_name"),
		LastName:  r.FormValue("last_name"),
		Email:     r.FormValue("email"),
		Phone:     r.FormValue("phone"),
		Livemode:  false,
		Metadata:  parseMetadata(r),
		Created:   h.store.Now(),
	}

	person.Relationship = parsePersonRelationship(r)

	h.store.Persons.Set(id, person)
	h.emitEvent("person.created", mapFromJSON(person))
	twincore.JSON(w, http.StatusOK, person)
}

// GetPerson handles GET /v1/accounts/{account_id}/persons/{id}.
func (h *Handler) GetPerson(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "account_id")
	if _, ok := h.store.Accounts.Get(accountID); !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such account: '"+accountID+"'")
		return
	}

	id := chi.URLParam(r, "id")
	person, ok := h.store.Persons.Get(id)
	if !ok || person.Account != accountID {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such person: '"+id+"'")
		return
	}
	twincore.JSON(w, http.StatusOK, person)
}

// UpdatePerson handles POST /v1/accounts/{account_id}/persons/{id}.
func (h *Handler) UpdatePerson(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	accountID := chi.URLParam(r, "account_id")
	if _, ok := h.store.Accounts.Get(accountID); !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such account: '"+accountID+"'")
		return
	}

	id := chi.URLParam(r, "id")
	person, ok := h.store.Persons.Get(id)
	if !ok || person.Account != accountID {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such person: '"+id+"'")
		return
	}

	if v := r.FormValue("first_name"); v != "" {
		person.FirstName = v
	}
	if v := r.FormValue("last_name"); v != "" {
		person.LastName = v
	}
	if v := r.FormValue("email"); v != "" {
		person.Email = v
	}
	if v := r.FormValue("phone"); v != "" {
		person.Phone = v
	}
	if meta := parseMetadata(r); meta != nil {
		person.Metadata = meta
	}

	h.store.Persons.Set(id, person)
	h.emitEvent("person.updated", mapFromJSON(person))
	twincore.JSON(w, http.StatusOK, person)
}

// DeletePerson handles DELETE /v1/accounts/{account_id}/persons/{id}.
func (h *Handler) DeletePerson(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "account_id")
	if _, ok := h.store.Accounts.Get(accountID); !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such account: '"+accountID+"'")
		return
	}

	id := chi.URLParam(r, "id")
	person, ok := h.store.Persons.Get(id)
	if !ok || person.Account != accountID {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such person: '"+id+"'")
		return
	}

	h.store.Persons.Delete(id)
	h.emitEvent("person.deleted", map[string]any{"id": id, "account": accountID})

	twincore.JSON(w, http.StatusOK, map[string]any{
		"id":      id,
		"object":  "person",
		"deleted": true,
	})
}

// ListPersons handles GET /v1/accounts/{account_id}/persons.
func (h *Handler) ListPersons(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "account_id")
	if _, ok := h.store.Accounts.Get(accountID); !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such account: '"+accountID+"'")
		return
	}

	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)

	page := h.store.Persons.Paginate(cursor, limit)
	filtered := make([]store.Person, 0)
	for _, p := range page.Data {
		if p.Account == accountID {
			filtered = append(filtered, p)
		}
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/accounts/" + accountID + "/persons",
		"data":     filtered,
		"has_more": false,
	})
}

// parsePersonRelationship extracts relationship fields from form data.
func parsePersonRelationship(r *http.Request) *store.PersonRelationship {
	rel := &store.PersonRelationship{}
	hasData := false

	if v := r.FormValue("relationship[director]"); v == "true" {
		rel.Director = true
		hasData = true
	}
	if v := r.FormValue("relationship[executive]"); v == "true" {
		rel.Executive = true
		hasData = true
	}
	if v := r.FormValue("relationship[owner]"); v == "true" {
		rel.Owner = true
		hasData = true
	}
	if v := r.FormValue("relationship[representative]"); v == "true" {
		rel.Representative = true
		hasData = true
	}
	if v := r.FormValue("relationship[title]"); v != "" {
		rel.Title = v
		hasData = true
	}
	if v := r.FormValue("relationship[percent_ownership]"); v != "" {
		if pct, err := strconv.ParseFloat(v, 64); err == nil {
			rel.PercentOwnership = pct
			hasData = true
		}
	}

	if !hasData {
		return nil
	}
	return rel
}

// ── TopUps ───────────────────────────────────────────────────────────

// CreateTopUp handles POST /v1/topups.
func (h *Handler) CreateTopUp(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	amountStr := r.FormValue("amount")
	if amountStr == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing",
			"Missing required param: amount.")
		return
	}
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_invalid",
			"Invalid integer: "+amountStr)
		return
	}

	currency := r.FormValue("currency")
	if currency == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing",
			"Missing required param: currency.")
		return
	}

	// Credit platform balance
	h.store.CreditBalance("", currency, amount)

	// Record balance transaction
	btID := h.store.RecordBalanceTransaction("topup", "", currency, amount, 0)

	id := h.store.TopUps.NextID()
	topup := store.TopUp{
		ID:                 id,
		Object:             "topup",
		Amount:             amount,
		Currency:           currency,
		Description:        r.FormValue("description"),
		Status:             "succeeded",
		BalanceTransaction: btID,
		Livemode:           false,
		Metadata:           parseMetadata(r),
		Created:            h.store.Now(),
	}
	h.store.TopUps.Set(id, topup)

	h.emitEvent("topup.succeeded", mapFromJSON(topup))

	twincore.JSON(w, http.StatusOK, topup)
}

// GetTopUp handles GET /v1/topups/{id}.
func (h *Handler) GetTopUp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	topup, ok := h.store.TopUps.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound,
			"invalid_request_error", "resource_missing",
			"No such topup: '"+id+"'")
		return
	}
	twincore.JSON(w, http.StatusOK, topup)
}

// ListTopUps handles GET /v1/topups.
func (h *Handler) ListTopUps(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)

	page := h.store.TopUps.Paginate(cursor, limit)

	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/topups",
		"data":     page.Data,
		"has_more": page.HasMore,
	})
}
