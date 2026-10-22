package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
)

// signingKey is the ES256 key that signs every webhook (quirk DELIVERY-003).
// One key serves the whole run; it is created on first use and survives
// /admin/reset, as a vendor's key outlives any one test.
type signingKey struct {
	key       *ecdsa.PrivateKey
	kid       string
	createdAt time.Time
}

var b64 = base64.RawURLEncoding

// signing returns the key, creating it on first use.
func (h *Handler) signing() *signingKey {
	h.keyOnce.Do(func() {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			panic("twin-plaid: generate signing key: " + err.Error())
		}
		h.sk = &signingKey{key: k, kid: sim.NewIDGenerator(h.store.Rand).UUID(), createdAt: h.store.Clock.Now()}
	})
	return h.sk
}

// jwk is the public half in Plaid's JWK shape. expired_at is required and null.
func (k *signingKey) jwk() map[string]any {
	pad := func(n *big.Int) string { return b64.EncodeToString(n.FillBytes(make([]byte, 32))) }
	return map[string]any{
		"alg": "ES256", "crv": "P-256", "kty": "EC", "use": "sig", "kid": k.kid,
		"x": pad(k.key.X), "y": pad(k.key.Y), "created_at": k.createdAt.Unix(), "expired_at": nil,
	}
}

// webhookVerificationKeyGet is POST /webhook_verification_key/get.
func (h *Handler) webhookVerificationKeyGet(w http.ResponseWriter, r *http.Request) {
	call, _ := CallFrom(r)
	id, ok := asString(call.Body["key_id"])
	if !ok {
		h.invalidBody(w)
		return
	}
	if id == "" {
		h.writeMissing(w, []string{"key_id"})
		return
	}
	k := h.signing()
	if id != k.kid {
		h.writeError(w, http.StatusBadRequest, "INVALID_INPUT", "INVALID_WEBHOOK_VERIFICATION_KEY_ID",
			"invalid key_id provided. note that key_ids are specific to Plaid environments, and verification requests must be made to the same environment that the webhook was sent from")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"key": k.jwk()})
}

// webhookSigner signs a webhook body the way Plaid does: an ES256 JWT in the
// Plaid-Verification header whose claims are iat and the SHA-256 of the exact
// body bytes.
type webhookSigner struct{ h *Handler }

// WebhookSigner is the signer for the webhook dispatcher. Configure the
// dispatcher with SignWithoutSecret: the key is the signer's, not a secret.
func (h *Handler) WebhookSigner() webhook.Signer { return webhookSigner{h} }

func (s webhookSigner) Sign(payload []byte, _ string) map[string]string {
	k := s.h.signing()
	sum := sha256.Sum256(payload)
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": k.kid, "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"iat": s.h.store.Clock.Now().Unix(), "request_body_sha256": hex.EncodeToString(sum[:])})
	signed := b64.EncodeToString(header) + "." + b64.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signed))
	r, sv, err := ecdsa.Sign(rand.Reader, k.key, digest[:])
	if err != nil {
		return nil
	}
	sig := append(r.FillBytes(make([]byte, 32)), sv.FillBytes(make([]byte, 32))...)
	return map[string]string{"Plaid-Verification": signed + "." + b64.EncodeToString(sig)}
}
