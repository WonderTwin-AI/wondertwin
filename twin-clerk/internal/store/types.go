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
	// PasswordHash stores the password for FAPI sign-in verification.
	// Not exposed in JSON responses (json:"-").
	PasswordHash        string            `json:"-"`
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

// --- FAPI (Frontend API) types ---

// Client represents a Clerk client (browser session container).
// A client holds multiple sessions and tracks sign-in/sign-up state.
type Client struct {
	ID                         string       `json:"id"`
	Object                     string       `json:"object"`
	Sessions                   []FAPISession `json:"sessions"`
	SignIn                     *SignIn      `json:"sign_in"`
	SignUp                     any          `json:"sign_up"`
	LastActiveSessionID        *string      `json:"last_active_session_id"`
	LastAuthenticationStrategy *string      `json:"last_authentication_strategy"`
	CookieExpiresAt            *int64       `json:"cookie_expires_at"`
	CreatedAt                  int64        `json:"created_at"`
	UpdatedAt                  int64        `json:"updated_at"`
}

// FAPISession is the FAPI representation of a session, which embeds the full User.
type FAPISession struct {
	ID                       string            `json:"id"`
	Object                   string            `json:"object"`
	Status                   string            `json:"status"`
	ExpireAt                 int64             `json:"expire_at"`
	AbandonAt                int64             `json:"abandon_at"`
	LastActiveAt             int64             `json:"last_active_at"`
	LastActiveToken          *TokenResponse    `json:"last_active_token"`
	LastActiveOrganizationID *string           `json:"last_active_organization_id"`
	Actor                    any               `json:"actor"`
	User                     User              `json:"user"`
	PublicUserData           PublicUserData     `json:"public_user_data"`
	FactorVerificationAge    any               `json:"factor_verification_age"`
	CreatedAt                int64             `json:"created_at"`
	UpdatedAt                int64             `json:"updated_at"`
}

// PublicUserData is the public subset of user data in a session.
type PublicUserData struct {
	FirstName  string `json:"first_name,omitempty"`
	LastName   string `json:"last_name,omitempty"`
	ImageURL   string `json:"image_url,omitempty"`
	Identifier string `json:"identifier,omitempty"`
}

// SignIn represents an in-progress or completed sign-in attempt.
type SignIn struct {
	ID                    string         `json:"id"`
	Object                string         `json:"object"`
	Status                string         `json:"status"`
	SupportedIdentifiers  []string       `json:"supported_identifiers"`
	Identifier            string         `json:"identifier,omitempty"`
	SupportedFirstFactors []SignInFactor `json:"supported_first_factors"`
	FirstFactorVerification *FactorVerification `json:"first_factor_verification"`
	SecondFactorVerification any         `json:"second_factor_verification"`
	CreatedSessionID      *string        `json:"created_session_id"`
	UserID                *string        `json:"user_id,omitempty"`
	CreatedAt             int64          `json:"created_at"`
	UpdatedAt             int64          `json:"updated_at"`
	AbandonAt             int64          `json:"abandon_at"`
}

// SignInFactor describes a supported authentication factor.
type SignInFactor struct {
	Strategy string `json:"strategy"`
	SafeIdentifier string `json:"safe_identifier,omitempty"`
	EmailAddressID string `json:"email_address_id,omitempty"`
	PhoneNumberID  string `json:"phone_number_id,omitempty"`
}

// FactorVerification tracks the state of a factor verification attempt.
type FactorVerification struct {
	Status   string `json:"status"`
	Strategy string `json:"strategy"`
	Attempts *int   `json:"attempts"`
	ExpireAt *int64 `json:"expire_at"`
}
