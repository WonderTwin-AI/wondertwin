package store

import "time"

// Webhook statuses. A subscription delivers only while OK; one whose receiver
// failed the intent-to-receive check stays registered but disabled until it is
// verified again.
const (
	WebhookOK     = "OK"
	WebhookFailed = "INTENT_TO_RECEIVE_FAILED"
)

// Webhook is an app's subscription. Xero has no API for it (a developer-portal
// action), so the emulator registers one through an admin route.
type Webhook struct {
	ID             string    `json:"id"`
	URL            string    `json:"url"`
	Key            string    `json:"key"`
	Categories     []string  `json:"categories"`
	Status         string    `json:"status"`
	Reason         string    `json:"reason,omitempty"`
	CreatedDateUTC time.Time `json:"createdDateUtc"`
}
