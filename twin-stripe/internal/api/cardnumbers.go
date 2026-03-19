package api

// cardBehavior describes the outcome when a test card number is used.
type cardBehavior struct {
	Succeed        bool
	DeclineCode    string // e.g. "card_declined", "insufficient_funds"
	Message        string
	RequiresAction bool // 3D Secure authentication required
}

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
	"4000000000000002": {DeclineCode: "card_declined", Message: "Your card was declined."},
	"4000000000009995": {DeclineCode: "insufficient_funds", Message: "Your card has insufficient funds."},
	"4000000000009987": {DeclineCode: "lost_card", Message: "Your card was declined."},
	"4000000000009979": {DeclineCode: "stolen_card", Message: "Your card was declined."},
	"4000000000000069": {DeclineCode: "expired_card", Message: "Your card has expired."},
	"4000000000000127": {DeclineCode: "incorrect_cvc", Message: "Your card's security code is incorrect."},
	"4000000000000119": {DeclineCode: "processing_error", Message: "An error occurred while processing your card."},
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
	pm, ok := h.store.PaymentMethods.Get(paymentMethodID)
	if !ok || pm.Card == nil || pm.Card.Number == "" {
		return cardBehavior{Succeed: true}
	}
	return lookupCardBehavior(pm.Card.Number)
}
