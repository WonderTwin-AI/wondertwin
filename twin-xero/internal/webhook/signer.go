// Package webhook provides Xero-specific webhook signing.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

// XeroSigner signs webhook payloads using HMAC-SHA256, base64-encoded.
type XeroSigner struct{}

// NewXeroSigner creates a new Xero webhook signer.
func NewXeroSigner() *XeroSigner {
	return &XeroSigner{}
}

// Sign returns the x-xero-signature header for the given payload.
func (s *XeroSigner) Sign(payload []byte, secret string) map[string]string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return map[string]string{
		"x-xero-signature": sig,
	}
}
