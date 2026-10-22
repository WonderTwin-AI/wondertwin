package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func jwkServer(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.With(h.checkRequest).Post("/webhook_verification_key/get", h.webhookVerificationKeyGet)
	return r
}

// signed signs a body and returns the JWT's header, claims, and the parts to verify.
func signed(t *testing.T, h *Handler, body string) (header, claims map[string]any, signingInput string, sig []byte) {
	t.Helper()
	jwt := h.WebhookSigner().Sign([]byte(body), "")["Plaid-Verification"]
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("Plaid-Verification = %q, want a three part JWT", jwt)
	}
	for i, into := range []*map[string]any{&header, &claims} {
		raw, err := b64.DecodeString(parts[i])
		if err != nil || json.Unmarshal(raw, into) != nil {
			t.Fatalf("part %d is not base64url JSON: %v", i, err)
		}
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		t.Fatalf("signature: %d bytes, err %v; want 64 bytes of r||s", len(sig), err)
	}
	return header, claims, parts[0] + "." + parts[1], sig
}

func fetchKey(t *testing.T, srv http.Handler, kid string) map[string]any {
	t.Helper()
	_, out := probe{path: "/webhook_verification_key/get", body: `{"client_id":"x","secret":"y","key_id":"` + kid + `"}`}.do(t, srv)
	return out
}

func TestWebhookSignerProducesAJWTTheServedKeyVerifies(t *testing.T) {
	h := newTestHandler(1)
	header, claims, input, sig := signed(t, h, `{"webhook_type":"ITEM","webhook_code":"ERROR"}`)
	if header["alg"] != "ES256" || header["typ"] != "JWT" || header["kid"] == nil {
		t.Fatalf("header = %v", header)
	}
	sum := sha256.Sum256([]byte(`{"webhook_type":"ITEM","webhook_code":"ERROR"}`))
	if claims["request_body_sha256"] != hex.EncodeToString(sum[:]) || claims["iat"] != float64(h.store.Clock.Now().Unix()) {
		t.Fatalf("claims = %v", claims)
	}

	jwk := fetchKey(t, jwkServer(h), header["kid"].(string))["key"].(map[string]any)
	x, _ := b64.DecodeString(jwk["x"].(string))
	y, _ := b64.DecodeString(jwk["y"].(string))
	pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
	digest := sha256.Sum256([]byte(input))
	if !ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("the served key does not verify the signature")
	}
	tampered := sha256.Sum256([]byte(input + "x"))
	if ecdsa.Verify(pub, tampered[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("the signature verified over different content")
	}
}

func TestWebhookVerificationKeyGetServesTheDocumentedJWKShape(t *testing.T) {
	h := newTestHandler(1)
	header, _, _, _ := signed(t, h, "{}")
	out := fetchKey(t, jwkServer(h), header["kid"].(string))
	jwk, _ := out["key"].(map[string]any)
	for k, want := range map[string]any{"alg": "ES256", "crv": "P-256", "kty": "EC", "use": "sig", "kid": header["kid"]} {
		if jwk[k] != want {
			t.Fatalf("key.%s = %v, want %v", k, jwk[k], want)
		}
	}
	if v, ok := jwk["expired_at"]; !ok || v != nil {
		t.Fatalf("expired_at must be present as null: %v", jwk)
	}
	if _, ok := jwk["created_at"].(float64); !ok || out["request_id"] == nil {
		t.Fatalf("created_at or request_id missing: %v", out)
	}
	x, _ := b64.DecodeString(jwk["x"].(string))
	y, _ := b64.DecodeString(jwk["y"].(string))
	if len(x) != 32 || len(y) != 32 || !elliptic.P256().IsOnCurve(new(big.Int).SetBytes(x), new(big.Int).SetBytes(y)) {
		t.Fatalf("x and y are not a P-256 point: %d and %d bytes", len(x), len(y))
	}
}

func TestWebhookVerificationKeyGetRefusesBadKeyIDs(t *testing.T) {
	srv := jwkServer(newTestHandler(1))
	for body, code := range map[string]string{
		`{"client_id":"x","secret":"y","key_id":"not-the-key"}`: "INVALID_WEBHOOK_VERIFICATION_KEY_ID",
		`{"client_id":"x","secret":"y"}`:                        "MISSING_FIELDS",
		`{"client_id":"x","secret":"y","key_id":5}`:             "INVALID_BODY",
	} {
		status, out := probe{path: "/webhook_verification_key/get", body: body}.do(t, srv)
		if status != 400 || out["error_code"] != code {
			t.Fatalf("%s: %d %v", body, status, out["error_code"])
		}
	}
}

func TestWebhookSignerUsesOneKeyForTheWholeRun(t *testing.T) {
	h := newTestHandler(1)
	a, _, _, _ := signed(t, h, "one")
	h.store.Reset()
	b, _, _, _ := signed(t, h, "two")
	if a["kid"] != b["kid"] {
		t.Fatalf("kid changed across a reset: %v then %v", a["kid"], b["kid"])
	}
}
