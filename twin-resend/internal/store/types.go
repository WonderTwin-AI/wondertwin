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

// Domain represents a verified sending domain.
type Domain struct {
	ID        string         `json:"id"`
	Object    string         `json:"object"` // "domain"
	Name      string         `json:"name"`
	Status    string         `json:"status"` // not_started, pending, verified, failed, temporary_failure
	Region    string         `json:"region"` // us-east-1, eu-west-1, sa-east-1
	Records   []DomainRecord `json:"records"`
	CreatedAt string         `json:"created_at"`
}

// DomainRecord is a DNS record needed for domain verification.
type DomainRecord struct {
	Record   string `json:"record"`   // CNAME, TXT, MX
	Name     string `json:"name"`
	Type     string `json:"type"`
	TTL      string `json:"ttl"`
	Status   string `json:"status"`   // not_started, verified
	Value    string `json:"value"`
	Priority *int   `json:"priority,omitempty"`
}

// APIKey represents a Resend API key.
type APIKey struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Token     string `json:"token,omitempty"` // only returned on create
	CreatedAt string `json:"created_at"`
}

// Audience represents a contact list / audience.
type Audience struct {
	ID        string `json:"id"`
	Object    string `json:"object"` // "audience"
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// Contact represents a contact within an audience.
type Contact struct {
	ID           string `json:"id"`
	Object       string `json:"object"` // "contact"
	Email        string `json:"email"`
	FirstName    string `json:"first_name,omitempty"`
	LastName     string `json:"last_name,omitempty"`
	Unsubscribed bool   `json:"unsubscribed"`
	CreatedAt    string `json:"created_at"`
	// Internal
	AudienceID string `json:"-"`
}

// Broadcast represents a broadcast email to an audience.
type Broadcast struct {
	ID         string `json:"id"`
	Object     string `json:"object"` // "broadcast"
	Name       string `json:"name"`
	AudienceID string `json:"audience_id"`
	From       string `json:"from"`
	Subject    string `json:"subject"`
	ReplyTo    []string `json:"reply_to,omitempty"`
	HTML       string `json:"html,omitempty"`
	Text       string `json:"text,omitempty"`
	Status     string `json:"status"` // draft, queued, sending, sent
	CreatedAt  string `json:"created_at"`
	SentAt     string `json:"sent_at,omitempty"`
}
