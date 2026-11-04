package api

import (
	"crypto/sha256"
	"fmt"
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// testBankTokens are Stripe's documented bank account test tokens
// (docs.stripe.com/connect/testing). They are not stored, so they are never
// marked used and stay reusable. What they resolve to is not documented:
// STRIPE TEST BANK, routing 110000000 and account 000123456789 are inferred
// (quirk crud-010), the test bank account Stripe documents for US payouts.
var testBankTokens = map[string]bool{
	"btok_us_verified":                    true,
	"btok_us_verified_noAccount":          true,
	"btok_us_verified_accountClosed":      true,
	"btok_us_verified_insufficientFunds":  true,
	"btok_us_verified_debitNotAuthorized": true,
	"btok_us_verified_invalidCurrency":    true,
}

const (
	testRoutingNumber = "110000000"
	testAccountNumber = "000123456789"
	testBankName      = "STRIPE TEST BANK"
)

// bankFingerprint is the fingerprint of a bank account, the same whether it
// arrives as a token or as raw numbers.
func bankFingerprint(routingNumber, accountNumber string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(routingNumber+accountNumber)))[:16]
}

// resolveBankToken returns the bank account a token stands for, marking a
// stored token used. On refusal it writes Stripe's error and returns false.
// The 400 status of both refusals is not documented (quirks error-005,
// crud-009) and is declared in divergences.json.
func (h *Handler) resolveBankToken(w http.ResponseWriter, id string) (store.BankAccountDetails, bool) {
	if testBankTokens[id] {
		return store.BankAccountDetails{
			Object:        "bank_account",
			BankName:      testBankName,
			Country:       "US",
			Currency:      "usd",
			Fingerprint:   bankFingerprint(testRoutingNumber, testAccountNumber),
			Last4:         testAccountNumber[len(testAccountNumber)-4:],
			RoutingNumber: testRoutingNumber,
			Status:        "new",
		}, true
	}
	tok, ok := h.store.Tokens.Get(id)
	if !ok || tok.Type != "bank_account" || tok.BankAccount == nil {
		writeError(w, http.StatusBadRequest, apiError{
			Type:    "invalid_request_error",
			Code:    "resource_missing",
			Message: "No such token: '" + id + "'",
			Param:   "external_account",
		})
		return store.BankAccountDetails{}, false
	}
	if tok.Used {
		writeError(w, http.StatusBadRequest, apiError{
			Type:    "invalid_request_error",
			Code:    "token_already_used",
			Message: "You cannot use a Stripe token more than once: " + id + ".",
			Param:   "external_account",
		})
		return store.BankAccountDetails{}, false
	}
	tok.Used = true
	h.store.Tokens.Set(id, tok)
	return *tok.BankAccount, true
}
