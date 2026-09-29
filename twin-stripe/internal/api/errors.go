package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// apiError is Stripe's error object (api/errors). Empty fields are omitted,
// as Stripe omits them.
type apiError struct {
	Type          string `json:"type"`
	Code          string `json:"code,omitempty"`
	DeclineCode   string `json:"decline_code,omitempty"`
	Message       string `json:"message,omitempty"`
	Param         string `json:"param,omitempty"`
	DocURL        string `json:"doc_url,omitempty"`
	Charge        string `json:"charge,omitempty"`
	AdviceCode    string `json:"advice_code,omitempty"`
	PaymentIntent any    `json:"payment_intent,omitempty"`
	PaymentMethod any    `json:"payment_method,omitempty"`
}

// writeError writes e as Stripe's error envelope. It fills doc_url for codes
// Stripe documents, since Stripe links every documented code to its entry.
func writeError(w http.ResponseWriter, status int, e apiError) {
	if e.DocURL == "" && e.Code != "" {
		if _, ok := documentedErrorCodes[e.Code]; ok {
			e.DocURL = errorDocURL(e.Code)
		}
	}
	twincore.JSON(w, status, map[string]any{"error": e})
}

// errorDocURL is the doc_url Stripe returns for an error code.
func errorDocURL(code string) string {
	return "https://stripe.com/docs/error-codes/" + strings.ReplaceAll(code, "_", "-")
}

var (
	noSuchPattern       = regexp.MustCompile(`^(No such [a-z_ ]+): '?([^' ]+)'?$`)
	missingParamPattern = regexp.MustCompile(`^Missing required param: ([A-Za-z0-9_\[\]]+)\.$`)
)

// stripeError writes a Stripe error from its type, code and message, and
// derives what Stripe adds to such errors: the quoted ID and param "id" on a
// missing resource, the param on a missing parameter, and doc_url.
func stripeError(w http.ResponseWriter, status int, errType, code, message string) {
	e := apiError{Type: errType, Code: code, Message: message}
	switch code {
	case "resource_missing":
		if m := noSuchPattern.FindStringSubmatch(message); m != nil {
			e.Message = m[1] + ": '" + m[2] + "'"
			e.Param = "id"
		}
	case "parameter_missing":
		if m := missingParamPattern.FindStringSubmatch(message); m != nil {
			e.Param = m[1]
		}
	}
	writeError(w, status, e)
}

// missingParamMessage names the first of params absent from the request, as
// Stripe reports one missing parameter at a time.
func missingParamMessage(r *http.Request, params ...string) string {
	for _, p := range params {
		if r.FormValue(p) == "" {
			return "Missing required param: " + p + "."
		}
	}
	return "Missing required param: " + params[0] + "."
}

// documentedErrorCodes are the codes listed on docs.stripe.com/error-codes
// (captured 2026-09-28). Only these get a doc_url.
var documentedErrorCodes = map[string]struct{}{
	"account_closed": {}, "account_country_invalid_address": {},
	"account_error_country_change_requires_additional_steps": {},
	"account_holder_name_verification_failed":                {}, "account_invalid": {},
	"account_number_invalid": {}, "acss_debit_session_incomplete": {}, "action_blocked": {},
	"alipay_upgrade_required": {}, "amount_too_large": {}, "amount_too_small": {},
	"anomalous_money_movement_request": {}, "api_key_expired": {}, "authentication_failure": {},
	"authentication_required": {}, "balance_insufficient": {}, "balance_invalid_parameter": {},
	"bank_account_bad_routing_numbers": {}, "bank_account_declined": {}, "bank_account_exists": {},
	"bank_account_restricted": {}, "bank_account_unusable": {}, "bank_account_unverified": {},
	"bank_account_verification_failed": {}, "billing_invalid_mandate": {},
	"bitcoin_upgrade_required": {}, "capability_not_active": {},
	"capture_charge_authorization_expired": {}, "capture_unauthorized_payment": {},
	"card_decline_rate_limit_exceeded": {}, "card_declined": {},
	"cardholder_phone_number_required": {}, "charge_already_captured": {},
	"charge_already_refunded": {}, "charge_disputed": {}, "charge_exceeds_source_limit": {},
	"charge_exceeds_transaction_limit": {}, "charge_expired_for_capture": {},
	"charge_invalid_parameter": {}, "clearing_code_unsupported": {}, "country_code_invalid": {},
	"country_unsupported": {}, "coupon_expired": {}, "customer_max_payment_methods": {},
	"customer_max_subscriptions": {}, "customer_tax_location_invalid": {}, "debit_not_authorized": {},
	"dispute_evidence_page_limit_exceeded": {}, "email_invalid": {}, "expired_card": {},
	"expired_payment_method": {}, "financial_connections_account_inactive": {},
	"financial_connections_no_successful_transaction_refresh": {}, "forwarding_api_inactive": {},
	"forwarding_api_invalid_parameter": {}, "forwarding_api_retryable_upstream_error": {},
	"forwarding_api_upstream_connection_error": {}, "forwarding_api_upstream_connection_timeout": {},
	"forwarding_api_upstream_error": {}, "idempotency_key_in_use": {}, "incorrect_address": {},
	"incorrect_cvc": {}, "incorrect_number": {}, "incorrect_postal_code": {}, "incorrect_zip": {},
	"instant_payouts_config_disabled": {}, "instant_payouts_limit_exceeded": {},
	"instant_payouts_unsupported": {}, "insufficient_funds": {}, "intent_invalid_state": {},
	"intent_verification_method_missing": {}, "invalid_card_type": {}, "invalid_characters": {},
	"invalid_charge_amount": {}, "invalid_cvc": {}, "invalid_expiry_month": {},
	"invalid_expiry_year": {}, "invalid_mandate_reference_prefix_format": {}, "invalid_number": {},
	"invalid_source_usage": {}, "invalid_tax_location": {}, "invoice_no_customer_line_items": {},
	"invoice_no_payment_method_types": {}, "invoice_no_subscription_line_items": {},
	"invoice_not_editable": {}, "invoice_on_behalf_of_not_editable": {},
	"invoice_payment_intent_requires_action": {}, "invoice_upcoming_none": {},
	"livemode_mismatch": {}, "lock_timeout": {}, "missing": {}, "no_account": {},
	"not_allowed_on_standard_account": {}, "out_of_inventory": {}, "parameter_invalid_empty": {},
	"parameter_invalid_integer": {}, "parameter_invalid_string_blank": {},
	"parameter_invalid_string_empty": {}, "parameter_missing": {}, "parameter_unknown": {},
	"parameters_exclusive": {}, "payment_intent_action_required": {},
	"payment_intent_amount_reconfirmation_required": {}, "payment_intent_authentication_failure": {},
	"payment_intent_automatic_tax_incomplete": {}, "payment_intent_incompatible_payment_method": {},
	"payment_intent_invalid_parameter": {}, "payment_intent_konbini_rejected_confirmation_number": {},
	"payment_intent_mandate_invalid": {}, "payment_intent_payment_attempt_expired": {},
	"payment_intent_payment_attempt_failed": {}, "payment_intent_unexpected_state": {},
	"payment_method_bank_account_already_verified": {}, "payment_method_bank_account_blocked": {},
	"payment_method_billing_details_address_missing": {}, "payment_method_currency_mismatch": {},
	"payment_method_customer_decline": {}, "payment_method_invalid_parameter": {},
	"payment_method_invalid_parameter_testmode": {}, "payment_method_microdeposit_failed": {},
	"payment_method_microdeposit_processing_error":                      {},
	"payment_method_microdeposit_verification_amounts_invalid":          {},
	"payment_method_microdeposit_verification_amounts_mismatch":         {},
	"payment_method_microdeposit_verification_attempts_exceeded":        {},
	"payment_method_microdeposit_verification_descriptor_code_mismatch": {},
	"payment_method_microdeposit_verification_timeout":                  {}, "payment_method_not_available": {},
	"payment_method_provider_decline": {}, "payment_method_provider_timeout": {},
	"payment_method_restricted": {}, "payment_method_unactivated": {},
	"payment_method_unexpected_state": {}, "payment_method_unsupported_type": {},
	"payouts_limit_exceeded": {}, "payouts_not_allowed": {}, "platform_account_required": {},
	"platform_api_key_expired": {}, "postal_code_invalid": {}, "processing_error": {},
	"product_inactive": {}, "progressive_onboarding_limit_exceeded": {},
	"promotion_code_customer_missing_first_time": {}, "promotion_code_customer_not_first_time": {},
	"rate_limit": {}, "refer_to_customer": {}, "refund_disputed_payment": {},
	"resource_already_exists": {}, "resource_missing": {}, "return_intent_already_processed": {},
	"routing_number_invalid": {}, "secret_key_required": {}, "sepa_unsupported_account": {},
	"setup_attempt_failed": {}, "setup_intent_authentication_failure": {},
	"setup_intent_invalid_parameter": {}, "setup_intent_mandate_invalid": {},
	"setup_intent_setup_attempt_expired": {}, "setup_intent_unexpected_state": {},
	"shipping_address_invalid": {}, "shipping_calculation_failed": {}, "sku_inactive": {},
	"state_unsupported": {}, "status_transition_invalid": {}, "stripe_tax_inactive": {},
	"tax_id_invalid": {}, "tax_id_prohibited": {}, "taxes_calculation_failed": {},
	"terminal_location_country_unsupported": {}, "terminal_reader_busy": {},
	"terminal_reader_hardware_fault": {}, "terminal_reader_invalid_location_for_activation": {},
	"terminal_reader_invalid_location_for_payment": {}, "terminal_reader_offline": {},
	"terminal_reader_timeout": {}, "testmode_charges_only": {}, "tls_version_unsupported": {},
	"token_already_used": {}, "token_in_use": {}, "transfer_source_balance_parameters_mismatch": {},
	"transfers_not_allowed": {}, "url_invalid": {},
}
