package api_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// requireVerified checks a delivery the way a receiver does: it reads the key id
// from the Plaid-Verification JWT, fetches that key from the emulator's own
// /webhook_verification_key/get, verifies the ES256 signature over the first two
// parts, and compares request_body_sha256 with the bytes that arrived.
func requireVerified(t *testing.T, tc *testutil.TwinClient, r received) {
	t.Helper()
	parts := strings.Split(r.headers.Get("Plaid-Verification"), ".")
	if len(parts) != 3 {
		t.Fatalf("Plaid-Verification = %q, want a three part JWT", r.headers.Get("Plaid-Verification"))
	}
	var header, claims map[string]any
	for i, into := range []*map[string]any{&header, &claims} {
		raw, err := b64decode(parts[i])
		if err != nil || json.Unmarshal(raw, into) != nil {
			t.Fatalf("JWT part %d is not base64url JSON: %v", i, err)
		}
	}
	if header["alg"] != "ES256" {
		t.Fatalf("alg = %v, want ES256", header["alg"])
	}
	kid, _ := header["kid"].(string)
	jwk, _ := tc.Post("/webhook_verification_key/get", creds(map[string]any{"key_id": kid})).
		AssertStatus(200).JSONMap()["key"].(map[string]any)
	x, _ := b64decode(jwk["x"].(string))
	y, _ := b64decode(jwk["y"].(string))
	pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
	sig, err := b64decode(parts[2])
	if err != nil || len(sig) != 64 {
		t.Fatalf("signature is %d bytes (err %v), want 64 bytes of r||s", len(sig), err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("the served key does not verify the delivery's signature")
	}
	sum := sha256.Sum256([]byte(r.body))
	if claims["request_body_sha256"] != hex.EncodeToString(sum[:]) {
		t.Fatalf("request_body_sha256 does not match the delivered body: %v", claims)
	}
}

func TestFireWebhookDeliveryVerifiesAgainstTheServedKey(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	url, got := receiver(t)
	token := linkedWithHook(tc, []string{"auth"}, url)

	fire(tc, token, map[string]any{"webhook_code": "LOGIN_REPAIRED"}).AssertStatus(200)
	requireVerified(t, tc, next(t, got))
}

func TestSyncWebhookDeliveriesVerifyAgainstTheServedKey(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	post := func(path string, b map[string]any) map[string]any { return tc.Post(path, b).AssertStatus(200).JSONMap() }
	url, got := receiver(t)
	token := linkedWithHook(tc, []string{"transactions"}, url)

	doSync(tc, token, "", 0)
	post("/sandbox/transactions/create", creds(map[string]any{"access_token": token, "transactions": []any{newTx("row", 1, nil)}}))
	refresh(t, post, token)
	for i := 0; i < 2; i++ { // DEFAULT_UPDATE and SYNC_UPDATES_AVAILABLE, in either order
		requireVerified(t, tc, next(t, got))
	}
}

// setupPlaidWithCredentials is setupPlaid with the emulator enforcing a pair.
func setupPlaidWithCredentials(t *testing.T, clientID, secret string) *testutil.TwinClient {
	t.Helper()
	twin := twincore.New(&twincore.Config{Name: "twin-plaid-test"})
	mem := store.New()
	mem.Rand = twin.Rand
	h := api.NewHandler(mem, twin.Middleware())
	h.SetCredentials(clientID, secret)
	h.Routes(twin.Router)
	adminHandler := admin.NewHandler(mem, twin.Middleware(), mem.Clock)
	adminHandler.SetConfigProvider(twin)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	return testutil.NewTwinClient(t, srv)
}

func TestConfiguredCredentialsAreEnforcedOnRealRoutes(t *testing.T) {
	tc := setupPlaidWithCredentials(t, "good-id", "good-secret")
	good := func(m map[string]any) map[string]any {
		m["client_id"], m["secret"] = "good-id", "good-secret"
		return m
	}
	pub := tc.Post("/sandbox/public_token/create", good(map[string]any{
		"institution_id": "ins_109508", "initial_products": []string{"auth", "identity", "transactions"},
	})).AssertStatus(200).JSONMap()["public_token"]
	token := tc.Post("/item/public_token/exchange", good(map[string]any{"public_token": pub})).
		AssertStatus(200).JSONMap()["access_token"]

	routes := map[string]map[string]any{
		"/accounts/get":                {"access_token": token},
		"/item/get":                    {"access_token": token},
		"/auth/get":                    {"access_token": token},
		"/transactions/sync":           {"access_token": token},
		"/institutions/get_by_id":      {"institution_id": "ins_109508", "country_codes": []string{"US"}},
		"/sandbox/public_token/create": {"institution_id": "ins_109508", "initial_products": []string{"auth"}},
	}
	for path, body := range routes {
		for name, pair := range map[string][2]string{
			"wrong secret": {"good-id", "nope"}, "wrong client_id": {"nope", "good-secret"},
		} {
			b := map[string]any{"client_id": pair[0], "secret": pair[1]}
			for k, v := range body {
				b[k] = v
			}
			m := tc.Post(path, b).AssertStatus(400).JSONMap()
			if m["error_code"] != "INVALID_API_KEYS" || m["error_type"] != "INVALID_INPUT" {
				t.Errorf("%s with a %s: %v", path, name, m)
			}
		}
		tc.Post(path, good(cloneBody(body))).AssertStatus(200)
	}
}

func cloneBody(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func TestAdminStateRoundTripRestoresTheLinkedItemOverHTTP(t *testing.T) {
	tc, ac, _ := setupPlaid(t)
	token := linkedItem(tc)
	before := accountsOf(tc, token)

	state := ac.GetState().AssertStatus(200).JSONMap()
	for _, k := range []string{"items", "accounts", "access_tokens", "transactions"} {
		if m, _ := state[k].(map[string]any); len(m) == 0 {
			t.Fatalf("the snapshot has no %s: %v", k, state)
		}
	}
	ac.Reset().AssertStatus(200)
	tc.Post("/accounts/get", creds(map[string]any{"access_token": token})).AssertStatus(400)

	ac.LoadState(state).AssertStatus(200)
	after := accountsOf(tc, token)
	if len(after) != len(before) || after[0]["account_id"] != before[0]["account_id"] {
		t.Fatalf("accounts after the round trip = %v, want %v", after, before)
	}
}
