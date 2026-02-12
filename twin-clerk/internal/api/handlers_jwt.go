package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
)

// JWTManager manages RSA key pairs and JWT generation for the Clerk twin.
// It generates an RSA keypair at startup and exposes the public key via JWKS.
type JWTManager struct {
	mu         sync.RWMutex
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	keyID      string
}

// NewJWTManager creates a new JWTManager with a fresh RSA-2048 keypair.
func NewJWTManager() (*JWTManager, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Generate a stable key ID from the public key modulus
	hash := sha256.Sum256(key.PublicKey.N.Bytes())
	kid := base64.RawURLEncoding.EncodeToString(hash[:8])

	return &JWTManager{
		privateKey: key,
		publicKey:  &key.PublicKey,
		keyID:      kid,
	}, nil
}

// GenerateToken creates a signed JWT for the given user/session.
// Claims match the Clerk JWT format used by clerk-sdk-go/v2's jwt.Verify().
func (m *JWTManager) GenerateToken(userID, sessionID string, extraClaims map[string]any) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	claims := jwt.MapClaims{
		"iss": "https://clerk.twin.wondertwin.dev",
		"sub": userID,
		"aud": "wondertwin",
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
		"sid": sessionID,
		"azp": "wondertwin",
	}

	// Add extra claims if provided
	for k, v := range extraClaims {
		claims[k] = v
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = m.keyID

	signed, err := token.SignedString(m.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	return signed, nil
}

// JWKS represents a JSON Web Key Set.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a single JSON Web Key.
type JWK struct {
	KTY string `json:"kty"`
	Use string `json:"use"`
	KID string `json:"kid"`
	ALG string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// GetJWKS returns the JWKS endpoint: the public key in JWK format.
func (m *JWTManager) GetJWKS() JWKS {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return JWKS{
		Keys: []JWK{
			{
				KTY: "RSA",
				Use: "sig",
				KID: m.keyID,
				ALG: "RS256",
				N:   base64.RawURLEncoding.EncodeToString(m.publicKey.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(m.publicKey.E)).Bytes()),
			},
		},
	}
}

// GetJWKS handles GET /.well-known/jwks.json.
// This is the endpoint that Clerk SDKs use to fetch the public key for JWT verification.
func (h *Handler) GetJWKS(w http.ResponseWriter, r *http.Request) {
	jwks := h.jwtMgr.GetJWKS()
	twincore.JSON(w, http.StatusOK, jwks)
}

// generateJWTRequest is the JSON body for POST /admin/jwt/generate.
type generateJWTRequest struct {
	UserID      string         `json:"user_id"`
	SessionID   string         `json:"session_id,omitempty"`
	ExpiresIn   string         `json:"expires_in,omitempty"` // Go duration string, e.g., "1h"
	ExtraClaims map[string]any `json:"extra_claims,omitempty"`
}

// GenerateJWT handles POST /admin/jwt/generate.
// This is a test-only endpoint for generating JWTs for integration tests.
func (h *Handler) GenerateJWT(w http.ResponseWriter, r *http.Request) {
	var req generateJWTRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		clerkError(w, http.StatusBadRequest, "form_param_invalid",
			"Invalid request body.", err.Error())
		return
	}

	if req.UserID == "" {
		clerkError(w, http.StatusBadRequest, "form_param_missing",
			"user_id is required.", "You must provide a user_id to generate a JWT.")
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = "sess_sim_" + req.UserID
	}

	// Handle custom expiration
	extraClaims := req.ExtraClaims
	if req.ExpiresIn != "" {
		d, err := time.ParseDuration(req.ExpiresIn)
		if err != nil {
			clerkError(w, http.StatusBadRequest, "form_param_invalid",
				"Invalid expires_in duration.", err.Error())
			return
		}
		if extraClaims == nil {
			extraClaims = make(map[string]any)
		}
		extraClaims["exp"] = time.Now().Add(d).Unix()
	}

	token, err := h.jwtMgr.GenerateToken(req.UserID, sessionID, extraClaims)
	if err != nil {
		clerkError(w, http.StatusInternalServerError, "internal_error",
			"Failed to generate JWT.", err.Error())
		return
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"user_id":    req.UserID,
		"session_id": sessionID,
	})
}
