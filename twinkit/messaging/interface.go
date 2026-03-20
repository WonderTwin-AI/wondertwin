// Package messaging provides a message lifecycle engine for twins that
// simulate email, SMS, voice, push, and notification platforms. It handles
// message state machines, delivery tracking, suppression lists, and sender
// identity management.
package messaging

import "time"

// MessageStatus represents where a message is in its lifecycle.
type MessageStatus string

const (
	StatusQueued      MessageStatus = "QUEUED"
	StatusSending     MessageStatus = "SENDING"
	StatusSent        MessageStatus = "SENT"
	StatusDelivered   MessageStatus = "DELIVERED"
	StatusFailed      MessageStatus = "FAILED"
	StatusBounced     MessageStatus = "BOUNCED"
	StatusComplained  MessageStatus = "COMPLAINED"
	StatusOpened      MessageStatus = "OPENED"
	StatusClicked     MessageStatus = "CLICKED"
	StatusUndelivered MessageStatus = "UNDELIVERED"
	StatusDropped     MessageStatus = "DROPPED"
)

// ChannelType identifies the communication channel.
type ChannelType string

const (
	ChannelEmail ChannelType = "EMAIL"
	ChannelSMS   ChannelType = "SMS"
	ChannelVoice ChannelType = "VOICE"
	ChannelPush  ChannelType = "PUSH"
	ChannelInApp ChannelType = "IN_APP"
)

// Message is the universal message type across all messaging platforms.
type Message struct {
	ID          string            `json:"id"`
	Channel     ChannelType       `json:"channel"`
	Status      MessageStatus     `json:"status"`
	From        string            `json:"from"`
	To          []string          `json:"to"`
	CC          []string          `json:"cc,omitempty"`
	BCC         []string          `json:"bcc,omitempty"`
	ReplyTo     string            `json:"reply_to,omitempty"`
	Subject     string            `json:"subject,omitempty"`
	Body        string            `json:"body,omitempty"`
	HTMLBody    string            `json:"html_body,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	Attachments []Attachment      `json:"attachments,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	SentAt      time.Time         `json:"sent_at,omitempty"`
	DeliveredAt time.Time         `json:"delivered_at,omitempty"`
}

// Attachment is a file attached to a message.
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"`
}

// DeliveryEvent represents a status change notification for a message.
type DeliveryEvent struct {
	MessageID string        `json:"message_id"`
	Status    MessageStatus `json:"status"`
	Channel   ChannelType   `json:"channel"`
	Recipient string        `json:"recipient,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
	Details   string        `json:"details,omitempty"`
}

// Suppression tracks an address that should not receive messages.
type Suppression struct {
	Address   string      `json:"address"`
	Channel   ChannelType `json:"channel"`
	Reason    string      `json:"reason"`
	CreatedAt time.Time   `json:"created_at"`
}

// Sender represents a verified sender identity.
type Sender struct {
	ID       string      `json:"id"`
	Channel  ChannelType `json:"channel"`
	Address  string      `json:"address"`
	Verified bool        `json:"verified"`
}
