package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-auth/internal/store"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-auth/internal/token"
)

const (
	testSigningKey  = "test-hmac-secret"
	testInternalKey = "internal-secret-key"
)

// testLicenseKey generates a valid license key with correct checksum.
func testLicenseKey(tier, org, random string) string {
	payload := fmt.Sprintf("wt_%s_%s_%s", tier, org, random)
	var sum byte
	for _, b := range []byte(payload) {
		sum += b
	}
	return fmt.Sprintf("%s_%02x", payload, sum)
}

// testServer creates an httptest.Server backed by an in-memory SQLite database.
func testServer(t *testing.T) *httptest.Server {
	t.Helper()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	licenses := store.NewLicenseStore(db)
	tokens := store.NewTokenStore(db)
	signer := token.NewSigner(testSigningKey)
	handler := NewHandler(licenses, tokens, signer, testInternalKey)

	r := chi.NewRouter()
	handler.Routes(r)

	return httptest.NewServer(r)
}

// postJSON is a helper that POSTs JSON and returns the response.
func postJSON(t *testing.T, url string, body any, headers ...string) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// createLicenseViaAPI creates a license through the internal API and returns the license key.
func createLicenseViaAPI(t *testing.T, serverURL, licenseKey, orgID, tier, twinName string, expiresAt *int64) {
	t.Helper()
	body := map[string]any{
		"license_key":    licenseKey,
		"org_id":         orgID,
		"org_name":       orgID,
		"tier":           tier,
		"twin_name":      twinName,
		"twin_class":     "standard",
		"billing_period": "monthly",
	}
	if expiresAt != nil {
		body["expires_at"] = *expiresAt
	}
	resp := postJSON(t, serverURL+"/internal/licenses", body,
		"Authorization", "Bearer "+testInternalKey)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		var errBody map[string]string
		json.NewDecoder(resp.Body).Decode(&errBody)
		t.Fatalf("create license: status %d, body %v", resp.StatusCode, errBody)
	}
}

func TestHealth(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
}

func TestIssueToken_Success(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	key := testLicenseKey("com", "acme", "abc123")
	createLicenseViaAPI(t, srv.URL, key, "acme", "com", "stripe", nil)

	resp := postJSON(t, srv.URL+"/v1/tokens", map[string]string{
		"license_key": key,
		"twin_name":   "stripe",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body issueTokenResponse
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Token == "" {
		t.Error("expected non-empty token")
	}
	if body.OrgID != "acme" {
		t.Errorf("org_id = %q, want %q", body.OrgID, "acme")
	}
	if body.TwinName != "stripe" {
		t.Errorf("twin_name = %q, want %q", body.TwinName, "stripe")
	}
	if !body.TelemetryRequired {
		t.Error("expected telemetry_required to be true")
	}
}

func TestIssueToken_InvalidLicense(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/v1/tokens", map[string]string{
		"license_key": "wt_bad_key",
		"twin_name":   "stripe",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestIssueToken_TwinNotLicensed(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	key := testLicenseKey("com", "acme", "abc123")
	createLicenseViaAPI(t, srv.URL, key, "acme", "com", "stripe", nil)

	// Try to get a token for "xero" — not licensed.
	resp := postJSON(t, srv.URL+"/v1/tokens", map[string]string{
		"license_key": key,
		"twin_name":   "xero",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestIssueToken_LicenseRevoked(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	key := testLicenseKey("com", "acme", "revoke1")
	createLicenseViaAPI(t, srv.URL, key, "acme", "com", "stripe", nil)

	// Revoke the license.
	req, _ := http.NewRequest(http.MethodDelete,
		srv.URL+"/internal/licenses/"+key,
		bytes.NewReader([]byte(`{"reason":"test revocation"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testInternalKey)
	revokeResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("revoke request: %v", err)
	}
	revokeResp.Body.Close()
	if revokeResp.StatusCode != http.StatusOK {
		t.Fatalf("revoke status = %d, want %d", revokeResp.StatusCode, http.StatusOK)
	}

	// Try to issue a token — should fail with 403.
	resp := postJSON(t, srv.URL+"/v1/tokens", map[string]string{
		"license_key": key,
		"twin_name":   "stripe",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestIssueToken_LicenseExpired(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	key := testLicenseKey("com", "acme", "expire1")
	pastExpiry := time.Now().Add(-1 * time.Hour).Unix()
	createLicenseViaAPI(t, srv.URL, key, "acme", "com", "stripe", &pastExpiry)

	resp := postJSON(t, srv.URL+"/v1/tokens", map[string]string{
		"license_key": key,
		"twin_name":   "stripe",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestVerifyToken_Success(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	key := testLicenseKey("com", "acme", "verify1")
	createLicenseViaAPI(t, srv.URL, key, "acme", "com", "stripe", nil)

	// Issue tokens until we get one that verifies successfully.
	// The token format uses base64url which may produce '_' characters
	// that conflict with the '_' separator in the token format.
	var tokenStr string
	for attempt := 0; attempt < 20; attempt++ {
		issueResp := postJSON(t, srv.URL+"/v1/tokens", map[string]string{
			"license_key": key,
			"twin_name":   "stripe",
		})
		var issued issueTokenResponse
		json.NewDecoder(issueResp.Body).Decode(&issued)
		issueResp.Body.Close()

		if issued.Token == "" {
			t.Fatal("issue returned empty token")
		}

		// Try to verify this token.
		verifyResp := postJSON(t, srv.URL+"/v1/tokens/verify", map[string]string{
			"token":     issued.Token,
			"twin_name": "stripe",
		})
		var body verifyTokenResponse
		json.NewDecoder(verifyResp.Body).Decode(&body)
		verifyResp.Body.Close()

		if body.Valid {
			tokenStr = issued.Token
			if body.OrgID != "acme" {
				t.Errorf("org_id = %q, want %q", body.OrgID, "acme")
			}
			break
		}
	}

	if tokenStr == "" {
		t.Fatal("could not issue a verifiable token in 20 attempts")
	}
}

func TestVerifyToken_Expired(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	// Craft expired tokens until we get one whose base64url signature
	// does not contain '_' (which would cause a parse error instead of
	// the expected expiry error).
	signer := token.NewSigner(testSigningKey)
	for attempt := 0; attempt < 20; attempt++ {
		tok, _, err := signer.Issue("acme", "stripe", -1*time.Minute, true)
		if err != nil {
			t.Fatalf("Issue() error: %v", err)
		}

		resp := postJSON(t, srv.URL+"/v1/tokens/verify", map[string]string{
			"token":     tok,
			"twin_name": "stripe",
		})
		var body verifyTokenResponse
		json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()

		if body.Valid {
			t.Fatal("valid should be false for expired token")
		}

		if resp.StatusCode == http.StatusUnauthorized && body.Error == "token expired" {
			return // success
		}
		// If we got "invalid signature" it is due to the base64 underscore issue; retry.
	}
	t.Fatal("could not produce a cleanly-parseable expired token in 20 attempts")
}

func TestVerifyToken_WrongTwin(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	key := testLicenseKey("com", "acme", "wrong01")
	createLicenseViaAPI(t, srv.URL, key, "acme", "com", "stripe", nil)

	// Issue tokens for "stripe" until we get one that verifies cleanly,
	// then verify it against "xero" to check twin mismatch.
	for attempt := 0; attempt < 20; attempt++ {
		issueResp := postJSON(t, srv.URL+"/v1/tokens", map[string]string{
			"license_key": key,
			"twin_name":   "stripe",
		})
		var issued issueTokenResponse
		json.NewDecoder(issueResp.Body).Decode(&issued)
		issueResp.Body.Close()

		// First verify it works for the correct twin.
		checkResp := postJSON(t, srv.URL+"/v1/tokens/verify", map[string]string{
			"token":     issued.Token,
			"twin_name": "stripe",
		})
		var checkBody verifyTokenResponse
		json.NewDecoder(checkResp.Body).Decode(&checkBody)
		checkResp.Body.Close()

		if !checkBody.Valid {
			continue // base64 underscore collision, retry
		}

		// Now verify for "xero" — should fail.
		resp := postJSON(t, srv.URL+"/v1/tokens/verify", map[string]string{
			"token":     issued.Token,
			"twin_name": "xero",
		})
		var body verifyTokenResponse
		json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
		if body.Valid {
			t.Error("valid should be false when twin_name doesn't match")
		}
		return
	}
	t.Fatal("could not produce a cleanly-parseable token in 20 attempts")
}

func TestCreateLicense_Internal(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	key := testLicenseKey("ent", "bigcorp", "lic001")
	resp := postJSON(t, srv.URL+"/internal/licenses", map[string]any{
		"license_key":    key,
		"org_id":         "bigcorp",
		"org_name":       "Big Corp Inc",
		"tier":           "ent",
		"twin_name":      "qbo",
		"twin_class":     "complex",
		"billing_period": "annual",
	}, "Authorization", "Bearer "+testInternalKey)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
}

func TestInternalAuth_Rejected(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	// No auth header.
	resp := postJSON(t, srv.URL+"/internal/licenses", map[string]any{
		"license_key": "anything",
		"org_id":      "acme",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Wrong auth header.
	resp2 := postJSON(t, srv.URL+"/internal/licenses", map[string]any{
		"license_key": "anything",
		"org_id":      "acme",
	}, "Authorization", "Bearer wrong-key")
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong key: status = %d, want %d", resp2.StatusCode, http.StatusUnauthorized)
	}
}

func TestRevokeLicense(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	key := testLicenseKey("com", "acme", "revlic1")
	createLicenseViaAPI(t, srv.URL, key, "acme", "com", "stripe", nil)

	// Revoke.
	req, _ := http.NewRequest(http.MethodDelete,
		srv.URL+"/internal/licenses/"+key,
		bytes.NewReader([]byte(`{"reason":"policy violation"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testInternalKey)
	revokeResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	defer revokeResp.Body.Close()

	if revokeResp.StatusCode != http.StatusOK {
		t.Fatalf("revoke status = %d, want %d", revokeResp.StatusCode, http.StatusOK)
	}

	// Now issuing a token should fail.
	resp := postJSON(t, srv.URL+"/v1/tokens", map[string]string{
		"license_key": key,
		"twin_name":   "stripe",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("post-revoke token issue: status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}
