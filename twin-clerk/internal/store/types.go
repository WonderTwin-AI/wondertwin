// Package store defines the Clerk twin's state types.
package store

import "time"

// User represents a Clerk user.
type User struct {
	ID                  string            `json:"id"`
	Object              string            `json:"object"`
	ExternalID          string            `json:"external_id,omitempty"`
	PrimaryEmailAddress string            `json:"primary_email_address_id,omitempty"`
	PrimaryPhoneNumber  string            `json:"primary_phone_number_id,omitempty"`
	Username            string            `json:"username,omitempty"`
	FirstName           string            `json:"first_name,omitempty"`
	LastName            string            `json:"last_name,omitempty"`
	ImageURL            string            `json:"image_url,omitempty"`
	EmailAddresses      []EmailAddress    `json:"email_addresses"`
	PhoneNumbers        []PhoneNumber     `json:"phone_numbers,omitempty"`
	PublicMetadata      map[string]any    `json:"public_metadata"`
	PrivateMetadata     map[string]any    `json:"private_metadata,omitempty"`
	UnsafeMetadata      map[string]any    `json:"unsafe_metadata,omitempty"`
	OrganizationMemberships []OrgMembership `json:"organization_memberships,omitempty"`
	PasswordEnabled     bool              `json:"password_enabled"`
	TwoFactorEnabled    bool              `json:"two_factor_enabled"`
	Banned              bool              `json:"banned"`
	Locked              bool              `json:"locked"`
	LastSignInAt        *int64            `json:"last_sign_in_at"`
	LastActiveAt        *int64            `json:"last_active_at"`
	CreatedAt           int64             `json:"created_at"`
	UpdatedAt           int64             `json:"updated_at"`
}

// EmailAddress represents a Clerk email address.
type EmailAddress struct {
	ID           string         `json:"id"`
	Object       string         `json:"object"`
	EmailAddress string         `json:"email_address"`
	Verification *Verification  `json:"verification,omitempty"`
	LinkedTo     []LinkedTo     `json:"linked_to,omitempty"`
}

// PhoneNumber represents a Clerk phone number.
type PhoneNumber struct {
	ID          string         `json:"id"`
	Object      string         `json:"object"`
	PhoneNumber string         `json:"phone_number"`
	Verification *Verification `json:"verification,omitempty"`
}

// Verification tracks email/phone verification status.
type Verification struct {
	Status   string `json:"status"`
	Strategy string `json:"strategy"`
}

// LinkedTo links an identifier to a user.
type LinkedTo struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// Session represents a Clerk session.
type Session struct {
	ID              string `json:"id"`
	Object          string `json:"object"`
	UserID          string `json:"user_id"`
	ClientID        string `json:"client_id"`
	Status          string `json:"status"`
	LastActiveAt    int64  `json:"last_active_at"`
	ExpireAt        int64  `json:"expire_at"`
	AbandonAt       int64  `json:"abandon_at"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
	LastActiveToken *TokenResponse `json:"last_active_token,omitempty"`
}

// TokenResponse wraps a JWT token.
type TokenResponse struct {
	Object string `json:"object"`
	JWT    string `json:"jwt"`
}

// Organization represents a Clerk organization.
type Organization struct {
	ID              string         `json:"id"`
	Object          string         `json:"object"`
	Name            string         `json:"name"`
	Slug            string         `json:"slug"`
	ImageURL        string         `json:"image_url,omitempty"`
	MembersCount    int            `json:"members_count"`
	MaxAllowedMemberships int     `json:"max_allowed_memberships"`
	PublicMetadata  map[string]any `json:"public_metadata"`
	PrivateMetadata map[string]any `json:"private_metadata,omitempty"`
	CreatedAt       int64          `json:"created_at"`
	UpdatedAt       int64          `json:"updated_at"`
}

// OrgMembership represents a user's membership in an organization.
type OrgMembership struct {
	ID             string         `json:"id"`
	Object         string         `json:"object"`
	Role           string         `json:"role"`
	PublicMetadata map[string]any `json:"public_metadata,omitempty"`
	Organization   *Organization  `json:"organization,omitempty"`
	CreatedAt      int64          `json:"created_at"`
	UpdatedAt      int64          `json:"updated_at"`
}

// ClerkError is the Clerk API error response format.
type ClerkError struct {
	Errors []ClerkErrorEntry `json:"errors"`
}

// ClerkErrorEntry is a single error in a Clerk error response.
type ClerkErrorEntry struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	LongMessage string `json:"long_message,omitempty"`
}

// ClerkList wraps a paginated list response in Clerk's format.
type ClerkList[T any] struct {
	Data       []T `json:"data"`
	TotalCount int `json:"total_count"`
}

// Now returns the current timestamp in milliseconds (Clerk uses ms).
func Now() int64 {
	return time.Now().UnixMilli()
}
