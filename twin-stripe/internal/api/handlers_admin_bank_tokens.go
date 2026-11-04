package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// bankTokenRequest is the JSON body for POST /admin/bank_account_tokens. Only
// token is required. The other fields default to what a Plaid Sandbox token
// resolves to in Stripe test mode (quirk crud-011), so a bare registration
// behaves like the real integration.
type bankTokenRequest struct {
	Token             string `json:"token"`
	BankName          string `json:"bank_name"`
	Last4             string `json:"last4"`
	RoutingNumber     string `json:"routing_number"`
	Country           string `json:"country"`
	Currency          string `json:"currency"`
	AccountHolderName string `json:"account_holder_name"`
}

var (
	bankTokenID = regexp.MustCompile(`^btok_[A-Za-z0-9_]+$`)
	fourDigits  = regexp.MustCompile(`^[0-9]{4}$`)
	twoLetters  = regexp.MustCompile(`^[A-Z]{2}$`)
	currencyRe  = regexp.MustCompile(`^[a-z]{3}$`)
)

// AdminRegisterBankToken handles POST /admin/bank_account_tokens. It stores a
// bank account token another service issued, a Plaid processor token for
// example, so that attaching it as an external account resolves to these bank
// details. The app emulator shares no state with the Plaid app emulator.
func (h *Handler) AdminRegisterBankToken(w http.ResponseWriter, r *http.Request) {
	var req bankTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if !bankTokenID.MatchString(req.Token) {
		twincore.Error(w, http.StatusBadRequest, "token must be btok_ followed by letters, digits or underscores")
		return
	}
	if _, exists := h.store.Tokens.Get(req.Token); exists || testBankTokens[req.Token] {
		twincore.Error(w, http.StatusBadRequest, "token "+req.Token+" already exists")
		return
	}
	req.Country = strings.ToUpper(req.Country)
	req.Currency = strings.ToLower(req.Currency)
	defaults := []struct {
		field *string
		value string
	}{
		{&req.BankName, testBankName},
		{&req.Last4, testAccountNumber[len(testAccountNumber)-4:]},
		{&req.RoutingNumber, testRoutingNumber},
		{&req.Country, "US"},
		{&req.Currency, "usd"},
	}
	for _, d := range defaults {
		if *d.field == "" {
			*d.field = d.value
		}
	}
	switch {
	case !fourDigits.MatchString(req.Last4):
		twincore.Error(w, http.StatusBadRequest, "last4 must be four digits")
		return
	case !twoLetters.MatchString(req.Country):
		twincore.Error(w, http.StatusBadRequest, "country must be a two-letter country code")
		return
	case !currencyRe.MatchString(req.Currency):
		twincore.Error(w, http.StatusBadRequest, "currency must be a three-letter currency code")
		return
	}

	tok := store.Token{
		ID:     req.Token,
		Object: "token",
		Type:   "bank_account",
		BankAccount: &store.BankAccountDetails{
			ID:                h.store.StripeID(h.store.ExternalAccts.NextID()),
			Object:            "bank_account",
			AccountHolderName: req.AccountHolderName,
			BankName:          req.BankName,
			Country:           req.Country,
			Currency:          req.Currency,
			Fingerprint:       bankFingerprint(req.RoutingNumber, req.Last4),
			Last4:             req.Last4,
			RoutingNumber:     req.RoutingNumber,
			Status:            "new",
		},
		Created: h.store.Now(),
	}
	h.store.Tokens.Set(tok.ID, tok)
	twincore.JSON(w, http.StatusOK, tok)
}
