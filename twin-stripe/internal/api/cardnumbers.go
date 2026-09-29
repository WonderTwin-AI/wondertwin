package api

import (
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// cardBehavior describes the outcome when a test card is charged.
type cardBehavior struct {
	Succeed        bool
	Code           string // error code, e.g. "card_declined", "expired_card"
	DeclineCode    string // issuer decline code, e.g. "generic_decline"; empty when Stripe gives none
	Message        string
	RequiresAction bool // 3D Secure authentication required
	// AttachFails marks cards that simulate an issuer decline: Stripe refuses
	// to attach them to a customer (docs.stripe.com/testing, declined payments).
	AttachFails bool
}

// Decline outcomes as docs.stripe.com/testing lists them: error code and
// decline code per test card.
var (
	declineGeneric      = cardBehavior{Code: "card_declined", DeclineCode: "generic_decline", Message: "Your card was declined.", AttachFails: true}
	declineInsufficient = cardBehavior{Code: "card_declined", DeclineCode: "insufficient_funds", Message: "Your card has insufficient funds.", AttachFails: true}
	declineLost         = cardBehavior{Code: "card_declined", DeclineCode: "lost_card", Message: "Your card was declined.", AttachFails: true}
	declineStolen       = cardBehavior{Code: "card_declined", DeclineCode: "stolen_card", Message: "Your card was declined.", AttachFails: true}
	declineExpired      = cardBehavior{Code: "expired_card", Message: "Your card has expired.", AttachFails: true}
	declineCVC          = cardBehavior{Code: "incorrect_cvc", Message: "Your card's security code is incorrect.", AttachFails: true}
	declineProcessing   = cardBehavior{Code: "processing_error", Message: "An error occurred while processing your card. Try again in a little bit.", AttachFails: true}
	declineVelocity     = cardBehavior{Code: "card_declined", DeclineCode: "card_velocity_exceeded", Message: "Your card was declined for making repeated attempts too frequently or exceeding its amount limit.", AttachFails: true}
	declineAfterAttach  = cardBehavior{Code: "card_declined", DeclineCode: "generic_decline", Message: "Your card was declined."}
	declineFraudulent   = cardBehavior{Code: "card_declined", DeclineCode: "fraudulent", Message: "Your card was declined.", AttachFails: true}
)

// testCardBehaviors maps Stripe test card numbers to their expected behavior.
// See https://docs.stripe.com/testing#cards
var testCardBehaviors = map[string]cardBehavior{
	// Success cards
	"4242424242424242": {Succeed: true},
	"4000056655665556": {Succeed: true},
	"5555555555554444": {Succeed: true},
	"5200828282828210": {Succeed: true},

	// 3D Secure cards
	"4000000000003220": {RequiresAction: true}, // 3DS2 required
	"4000000000003063": {RequiresAction: true}, // 3DS1 required
	"4000000000003097": {RequiresAction: true}, // 3DS required, will fail auth

	// Decline cards
	"4000000000000002": declineGeneric,
	"4000000000009995": declineInsufficient,
	"4000000000009987": declineLost,
	"4000000000009979": declineStolen,
	"4000000000000069": declineExpired,
	"4000000000000127": declineCVC,
	"4000000000000119": declineProcessing,
	"4000000000006975": declineVelocity,
	"4000000000000341": declineAfterAttach,
}

// testMethod is a documented test PaymentMethod or token ID.
type testMethod struct {
	brand, number, funding string
}

// testPaymentMethods are the test IDs docs.stripe.com/testing documents for
// server-side code (pm_card_*) and legacy tokens (tok_*). Using one creates a
// PaymentMethod from that test card, as Stripe does.
var testPaymentMethods = map[string]testMethod{
	"pm_card_visa":                                     {"visa", "4242424242424242", "credit"},
	"pm_card_visa_debit":                               {"visa", "4000056655665556", "debit"},
	"pm_card_mastercard":                               {"mastercard", "5555555555554444", "credit"},
	"pm_card_mastercard_debit":                         {"mastercard", "5200828282828210", "debit"},
	"pm_card_mastercard_prepaid":                       {"mastercard", "5105105105105100", "prepaid"},
	"pm_card_amex":                                     {"amex", "378282246310005", "credit"},
	"pm_card_discover":                                 {"discover", "6011111111111117", "credit"},
	"pm_card_diners":                                   {"diners", "3056930009020004", "credit"},
	"pm_card_jcb":                                      {"jcb", "3566002020360505", "credit"},
	"pm_card_unionpay":                                 {"unionpay", "6200000000000005", "credit"},
	"pm_card_bypassPending":                            {"visa", "4000000000000077", "credit"},
	"pm_card_bypassPendingInternational":               {"visa", "4000003720000278", "credit"},
	"pm_card_visa_chargeDeclined":                      {"visa", "4000000000000002", "credit"},
	"pm_card_visa_chargeDeclinedInsufficientFunds":     {"visa", "4000000000009995", "credit"},
	"pm_card_visa_chargeDeclinedLostCard":              {"visa", "4000000000009987", "credit"},
	"pm_card_visa_chargeDeclinedStolenCard":            {"visa", "4000000000009979", "credit"},
	"pm_card_chargeDeclinedExpiredCard":                {"visa", "4000000000000069", "credit"},
	"pm_card_chargeDeclinedIncorrectCvc":               {"visa", "4000000000000127", "credit"},
	"pm_card_chargeDeclinedProcessingError":            {"visa", "4000000000000119", "credit"},
	"pm_card_visa_chargeDeclinedVelocityLimitExceeded": {"visa", "4000000000006975", "credit"},
	"pm_card_chargeCustomerFail":                       {"visa", "4000000000000341", "credit"},
	"tok_visa":                                         {"visa", "4242424242424242", "credit"},
	"tok_visa_debit":                                   {"visa", "4000056655665556", "debit"},
	"tok_mastercard":                                   {"mastercard", "5555555555554444", "credit"},
	"tok_amex":                                         {"amex", "378282246310005", "credit"},
	"tok_bypassPending":                                {"visa", "4000000000000077", "credit"},
	"tok_bypassPendingInternational":                   {"visa", "4000003720000278", "credit"},
	"tok_visa_chargeDeclined":                          {"visa", "4000000000000002", "credit"},
	"tok_visa_chargeDeclinedInsufficientFunds":         {"visa", "4000000000009995", "credit"},
	"tok_visa_chargeDeclinedLostCard":                  {"visa", "4000000000009987", "credit"},
	"tok_visa_chargeDeclinedStolenCard":                {"visa", "4000000000009979", "credit"},
	"tok_chargeDeclinedExpiredCard":                    {"visa", "4000000000000069", "credit"},
	"tok_visa_chargeDeclinedExpiredCard":               {"visa", "4000000000000069", "credit"},
	"tok_visa_chargeDeclinedFraudulent":                {"visa", "4100000000000019", "credit"},
	"tok_chargeDeclinedIncorrectCvc":                   {"visa", "4000000000000127", "credit"},
	"tok_visa_chargeDeclinedIncorrectCvc":              {"visa", "4000000000000127", "credit"},
	"tok_chargeDeclinedProcessingError":                {"visa", "4000000000000119", "credit"},
	"tok_visa_chargeDeclinedProcessingError":           {"visa", "4000000000000119", "credit"},
}

func init() {
	// The fraudulent token has no card-number twin in the tables above.
	testCardBehaviors["4100000000000019"] = declineFraudulent
}

// lookupCardBehavior returns the behavior for a card number.
// Unknown numbers are treated as successful.
func lookupCardBehavior(number string) cardBehavior {
	if b, ok := testCardBehaviors[number]; ok {
		return b
	}
	return cardBehavior{Succeed: true}
}

// checkCardBehavior looks up a payment method's card number and returns its behavior.
func (h *Handler) checkCardBehavior(paymentMethodID string) cardBehavior {
	if paymentMethodID == "" {
		return cardBehavior{Succeed: true}
	}
	if tm, ok := testPaymentMethods[paymentMethodID]; ok {
		return lookupCardBehavior(tm.number)
	}
	pm, ok := h.store.PaymentMethods.Get(paymentMethodID)
	if !ok || pm.Card == nil || pm.Card.Number == "" {
		return cardBehavior{Succeed: true}
	}
	return lookupCardBehavior(pm.Card.Number)
}

// resolvePaymentMethod turns a documented test ID (pm_card_visa, tok_visa)
// into a new PaymentMethod built from that test card, the way Stripe does,
// and returns its ID. Any other ID is returned unchanged.
func (h *Handler) resolvePaymentMethod(id string) string {
	tm, ok := testPaymentMethods[id]
	if !ok {
		return id
	}
	newID := h.store.StripeID(h.store.PaymentMethods.NextID())
	h.store.PaymentMethods.Set(newID, store.PaymentMethod{
		ID:     newID,
		Object: "payment_method",
		Type:   "card",
		Card: &store.CardDetails{
			Brand:    tm.brand,
			Last4:    tm.number[len(tm.number)-4:],
			ExpMonth: 12,
			ExpYear:  2034,
			Funding:  tm.funding,
			Country:  "US",
			Number:   tm.number,
		},
		Created: h.store.Now(),
	})
	return newID
}
