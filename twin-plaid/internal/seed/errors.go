package seed

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed item_errors.json
var itemErrorsJSON []byte

// ItemError is an error a Sandbox Item can be in or fail with: Plaid's
// documented sample body for the code. Provenance is in item_errors.json.
type ItemError struct {
	HTTP           int     `json:"http"`
	Type           string  `json:"error_type"`
	Code           string  `json:"-"`
	Message        string  `json:"error_message"`
	DisplayMessage *string `json:"display_message"`
}

// passwordCodes are the codes plaid.com/docs/sandbox/test-credentials lists for
// the error_{CODE} Sandbox password. ITEM_LOGIN_REQUIRED is not among them: it
// comes from /sandbox/item/reset_login.
var passwordCodes = map[string]bool{
	"ACCESS_NOT_GRANTED": true, "COUNTRY_NOT_SUPPORTED": true, "INSTITUTION_NOT_RESPONDING": true,
	"INSTITUTION_NO_LONGER_SUPPORTED": true, "INSTITUTION_REGISTRATION_REQUIRED": true,
	"INSUFFICIENT_CREDENTIALS": true, "INTERNAL_SERVER_ERROR": true, "INVALID_CREDENTIALS": true,
	"INVALID_MFA": true, "INVALID_SEND_METHOD": true, "ITEM_LOCKED": true, "ITEM_NOT_SUPPORTED": true,
	"MFA_NOT_SUPPORTED": true, "NO_ACCOUNTS": true, "PAYMENT_INSUFFICIENT_FUNDS": true,
	"PAYMENT_INVALID_RECIPIENT": true, "PAYMENT_INVALID_REFERENCE": true, "PAYMENT_INVALID_SCHEDULE": true,
	"PAYMENT_REJECTED": true, "PAYMENT_SCHEME_NOT_SUPPORTED": true, "UNAUTHORIZED_INSTITUTION": true,
	"USER_INPUT_TIMEOUT": true, "USER_SETUP_REQUIRED": true, "USER_SHOULD_VERIFY_DEPOSIT_ALLOCATIONS": true,
}

var (
	loadOnce sync.Once
	byCode   map[string]ItemError
)

// LookupItemError returns the documented error for a code, if it has a sample.
func LookupItemError(code string) (ItemError, bool) {
	loadOnce.Do(func() {
		var f struct {
			Errors map[string]ItemError `json:"errors"`
		}
		if err := json.Unmarshal(itemErrorsJSON, &f); err != nil {
			panic("seed: item_errors.json: " + err.Error())
		}
		byCode = f.Errors
		for c, e := range byCode {
			e.Code = c
			byCode[c] = e
		}
	})
	e, ok := byCode[code]
	return e, ok
}

// LookupPasswordError returns the error an error_{CODE} password simulates. A
// listed code with no published sample (COUNTRY_NOT_SUPPORTED and
// USER_SHOULD_VERIFY_DEPOSIT_ALLOCATIONS) is not simulated.
func LookupPasswordError(code string) (ItemError, bool) {
	if !passwordCodes[code] {
		return ItemError{}, false
	}
	return LookupItemError(code)
}
