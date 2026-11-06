// Package store defines the Resend twin's state types and in-memory store.
package store

import (
	"encoding/json"
	"time"
)

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
	// MessageID is the Amazon SES message id Resend reports for the send.
	MessageID string `json:"message_id"`
	// ScheduledAt is the ISO 8601 time a scheduled email goes out; nil when
	// it was sent immediately.
	ScheduledAt *string    `json:"scheduled_at,omitempty"`
	Tags        []EmailTag `json:"tags,omitempty"`
	// Headers and Attachments are kept as sent (attachment metadata only,
	// never content); Resend does not return them on a read.
	Headers     map[string]string `json:"-"`
	Attachments []AttachmentMeta  `json:"-"`
}

// EmailTag is a name/value tag on an email.
type EmailTag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// AttachmentMeta is what the emulator keeps of an attachment: its name and
// type, not its content.
type AttachmentMeta struct {
	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// Email status constants matching Resend's lifecycle.
const (
	EmailStatusSent       = "sent"
	EmailStatusDelivered  = "delivered"
	EmailStatusBounced    = "bounced"
	EmailStatusComplained = "complained"
	EmailStatusSuppressed = "suppressed"
	EmailStatusScheduled  = "scheduled"
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

// DomainStatusVerified is the status of a domain that passed verification.
const DomainStatusVerified = "verified"

// DomainRecord is a DNS record needed for domain verification.
type DomainRecord struct {
	Record   string `json:"record"` // CNAME, TXT, MX
	Name     string `json:"name"`
	Type     string `json:"type"`
	TTL      string `json:"ttl"`
	Status   string `json:"status"` // not_started, verified
	Value    string `json:"value"`
	Priority *int   `json:"priority,omitempty"`
}

// APIKey represents a Resend API key.
type APIKey struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Token     string `json:"token,omitempty"` // only returned on create
	CreatedAt string `json:"created_at"`
	// Permission is full_access (the default) or sending_access; a
	// sending_access key may only send email (quirk auth-004).
	Permission string `json:"permission,omitempty"`
	// DomainID is the domain a sending_access key is bound to, if any.
	DomainID string `json:"domain_id,omitempty"`
	// LastUsedAt is when the key last authenticated a request; nil if never.
	LastUsedAt *string `json:"last_used_at"`
}

// API key permissions.
const (
	PermissionFullAccess    = "full_access"
	PermissionSendingAccess = "sending_access"
)

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
	ID         string   `json:"id"`
	Object     string   `json:"object"` // "broadcast"
	Name       string   `json:"name"`
	AudienceID string   `json:"audience_id"`
	From       string   `json:"from"`
	Subject    string   `json:"subject"`
	ReplyTo    []string `json:"reply_to,omitempty"`
	HTML       string   `json:"html,omitempty"`
	Text       string   `json:"text,omitempty"`
	Status     string   `json:"status"` // draft, queued, sending, sent
	CreatedAt  string   `json:"created_at"`
	SentAt     string   `json:"sent_at,omitempty"`
}

// IdempotencyRecord is what the emulator remembers about one Idempotency-Key:
// the fingerprint of the first request's body and, once it succeeded, its
// answer to replay. InFlight marks a request still being served.
type IdempotencyRecord struct {
	RequestHash string          `json:"request_hash"`
	CreatedAt   time.Time       `json:"created_at"`
	InFlight    bool            `json:"in_flight,omitempty"`
	Status      int             `json:"status,omitempty"`
	Body        json.RawMessage `json:"body,omitempty"`
}
