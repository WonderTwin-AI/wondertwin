// Package webhook provides QBO-specific webhook signing.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

// QBOSigner signs webhook payloads using HMAC-SHA256 with the verifier token.
type QBOSigner struct{}

func NewQBOSigner() *QBOSigner { return &QBOSigner{} }

// Sign returns the intuit-signature header for the given payload.
func (s *QBOSigner) Sign(payload []byte, secret string) map[string]string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return map[string]string{
		"intuit-signature": sig,
	}
}
