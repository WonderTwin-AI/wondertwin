// Package store defines the Twilio twin's state types and in-memory store.
package store

// Message represents a Twilio SMS message.
type Message struct {
	SID         string `json:"sid"`
	AccountSID  string `json:"account_sid"`
	To          string `json:"to"`
	From        string `json:"from"`
	Body        string `json:"body"`
	Status      string `json:"status"`
	Direction   string `json:"direction"`
	NumSegments string `json:"num_segments"`
	NumMedia    string `json:"num_media"`
	Price       string `json:"price,omitempty"`
	PriceUnit   string `json:"price_unit"`
	ErrorCode   *int   `json:"error_code"`
	ErrorMessage string `json:"error_message,omitempty"`
	DateCreated string `json:"date_created"`
	DateUpdated string `json:"date_updated"`
	DateSent    string `json:"date_sent,omitempty"`
	URI         string `json:"uri"`
}

// Message status constants matching Twilio's lifecycle.
const (
	MessageStatusQueued    = "queued"
	MessageStatusSending   = "sending"
	MessageStatusSent      = "sent"
	MessageStatusDelivered = "delivered"
	MessageStatusFailed    = "failed"
)

// Verification represents a Twilio Verify verification attempt.
type Verification struct {
	SID        string `json:"sid"`
	ServiceSID string `json:"service_sid"`
	To         string `json:"to"`
	Channel    string `json:"channel"`
	Status     string `json:"status"`
	Code       string `json:"-"` // hidden from API responses
	Valid      bool   `json:"valid"`
	CreatedAt  string `json:"date_created"`
	UpdatedAt  string `json:"date_updated"`
	ExpiresAt  string `json:"date_expires"`
	URL        string `json:"url"`
}

// Verification status constants matching Twilio Verify lifecycle.
const (
	VerificationStatusPending  = "pending"
	VerificationStatusApproved = "approved"
	VerificationStatusCanceled = "canceled"
	VerificationStatusExpired  = "expired"
)
