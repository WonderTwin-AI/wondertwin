// Package token provides JIT token generation and verification for wt-auth.
// Tokens are HMAC-SHA256 signed, not JWTs. Format: wtat_{base64url(payload)}.{base64url(signature)}
// The dot separator avoids collision with base64url's underscore character.
package token

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Claims are the payload of a JIT token.
type Claims struct {
	ID        string `json:"id"`
	Org       string `json:"org"`
	Twin      string `json:"twin"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	Telemetry bool   `json:"tel"`
}

// Signer creates and verifies tokens.
type Signer struct {
	key []byte
}

// NewSigner creates a token signer with the given HMAC key.
func NewSigner(key string) *Signer {
	return &Signer{key: []byte(key)}
}

// Issue creates a new signed token.
func (s *Signer) Issue(orgID, twinName string, ttl time.Duration, telemetryRequired bool) (string, *Claims, error) {
	id, err := randomID()
	if err != nil {
		return "", nil, err
	}

	now := time.Now()
	claims := &Claims{
		ID:        id,
		Org:       orgID,
		Twin:      twinName,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
		Telemetry: telemetryRequired,
	}

	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", nil, fmt.Errorf("marshaling claims: %w", err)
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := s.sign(payloadB64)
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	token := "wtat_" + payloadB64 + "." + sigB64
	return token, claims, nil
}

// Verify parses and verifies a token string. Returns the claims if valid.
func (s *Signer) Verify(tokenStr string) (*Claims, error) {
	if !strings.HasPrefix(tokenStr, "wtat_") {
		return nil, fmt.Errorf("invalid token format")
	}

	rest := tokenStr[5:] // strip "wtat_"
	dotIdx := strings.LastIndex(rest, ".")
	if dotIdx < 0 {
		return nil, fmt.Errorf("invalid token format")
	}

	payloadB64 := rest[:dotIdx]
	sigB64 := rest[dotIdx+1:]

	// Verify signature.
	expectedSig := s.sign(payloadB64)
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding")
	}
	if !hmac.Equal(sig, expectedSig) {
		return nil, fmt.Errorf("invalid signature")
	}

	// Decode payload.
	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("invalid payload encoding")
	}

	var claims Claims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	// Check expiry.
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

func (s *Signer) sign(data string) []byte {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func randomID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "tok_" + hex.EncodeToString(b), nil
}
