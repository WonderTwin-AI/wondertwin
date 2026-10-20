// Package webhook holds Xero's webhook signature and delivery payload.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

// SignatureHeader carries the signature of a delivery.
const SignatureHeader = "x-xero-signature"

// Signature is the base64 HMAC-SHA256 of the raw body under the app's webhook
// key. A receiver recomputes it and answers 401 on a mismatch.
func Signature(body []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// Signer implements the kit's webhook.Signer for Xero. Unlike Stripe's it signs
// the body alone, so it needs no clock.
type Signer struct{}

// Sign returns the x-xero-signature header for a body.
func (Signer) Sign(body []byte, key string) map[string]string {
	return map[string]string{SignatureHeader: Signature(body, key)}
}
