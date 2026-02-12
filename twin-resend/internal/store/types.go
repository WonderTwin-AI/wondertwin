// Package store defines the Resend twin's state types and in-memory store.
package store

// Email represents a Resend email.
type Email struct {
	ID        string   `json:"id"`
	Object    string   `json:"object"`
	From      string   `json:"from"`
	To        []string `json:"to"`
	Subject   string   `json:"subject"`
	HTML      string   `json:"html,omitempty"`
	Text      string   `json:"text,omitempty"`
	CC        []string `json:"cc,omitempty"`
	BCC       []string `json:"bcc,omitempty"`
	ReplyTo   []string `json:"reply_to,omitempty"`
	Status    string   `json:"status"`
	CreatedAt string   `json:"created_at"`
	LastEvent string   `json:"last_event"`
}

// Email status constants matching Resend's lifecycle.
const (
	EmailStatusSent      = "sent"
	EmailStatusDelivered = "delivered"
	EmailStatusBounced   = "bounced"
)
