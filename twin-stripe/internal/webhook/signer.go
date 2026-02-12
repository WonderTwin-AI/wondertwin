// Package webhook implements Stripe v1 webhook signature signing.
// The signature format must be compatible with stripe.webhook.ConstructEvent().
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// StripeSigner implements the Stripe v1 webhook signature scheme.
// This is HMAC-SHA256 with a timestamp, compatible with webhook.ConstructEvent().
//
// The signature header format is:
//
//	Stripe-Signature: t={timestamp},v1={signature}
//
// Where signature = HMAC-SHA256(secret, "{timestamp}.{payload}")
type StripeSigner struct{}

// NewStripeSigner creates a new Stripe webhook signer.
func NewStripeSigner() *StripeSigner {
	return &StripeSigner{}
}

// Sign produces the Stripe-Signature header value.
// Implements pkg/webhook.Signer interface.
func (s *StripeSigner) Sign(payload []byte, secret string) map[string]string {
	timestamp := time.Now().Unix()
	return s.SignWithTimestamp(payload, secret, timestamp)
}

// SignWithTimestamp produces the Stripe-Signature header with a specific timestamp.
// Useful for testing.
func (s *StripeSigner) SignWithTimestamp(payload []byte, secret string, timestamp int64) map[string]string {
	sig := ComputeSignature(timestamp, payload, secret)
	headerValue := fmt.Sprintf("t=%d,v1=%s", timestamp, sig)

	return map[string]string{
		"Stripe-Signature": headerValue,
	}
}

// ComputeSignature computes the Stripe v1 HMAC-SHA256 signature.
// This matches the algorithm in stripe-go/webhook.computeSignature().
func ComputeSignature(timestamp int64, payload []byte, secret string) string {
	// The signed content is "{timestamp}.{payload}"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	mac.Write([]byte("."))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
