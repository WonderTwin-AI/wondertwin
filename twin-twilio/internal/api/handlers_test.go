package api_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/pkg/admin"
	"github.com/wondertwin-ai/wondertwin/pkg/testutil"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-twilio/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-twilio/internal/store"
)

const testAccountSID = "AC_test_sim"

var basicAuthHeader = "Basic " + base64.StdEncoding.EncodeToString([]byte(testAccountSID+":auth_token_sim"))

func setupTwilio(t *testing.T) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-twilio-test"}
	twin := twincore.New(cfg)
	handler := api.NewHandler(memStore, twin.Middleware())
	handler.Routes(twin.Router)
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	tc := testutil.NewTwinClient(t, srv)
	return srv, tc
}

// twilioPostForm sends a form-encoded POST with Twilio Basic Auth.
// Returns status code and parsed JSON body.
func twilioPostForm(t *testing.T, tc *testutil.TwinClient, path string, form map[string]string) (int, map[string]any) {
	t.Helper()
	values := url.Values{}
	for k, v := range form {
		values.Set(k, v)
	}
	req, err := http.NewRequest("POST", tc.BaseURL+path, strings.NewReader(values.Encode()))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", basicAuthHeader)

	resp, err := tc.HTTPClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var m map[string]any
	json.Unmarshal(body, &m)
	return resp.StatusCode, m
}

// twilioGet sends a GET with Twilio Basic Auth using DoWithHeaders (which works for GETs).
func twilioGet(tc *testutil.TwinClient, path string) *testutil.Response {
	return tc.DoWithHeaders("GET", path, nil, map[string]string{
		"Authorization": basicAuthHeader,
	})
}

func msgPath(path string) string {
	return "/2010-04-01/Accounts/" + testAccountSID + path
}

func verifySvcPath(path string) string {
	return "/v2/Services/VA_test_service" + path
}

// --- Auth Tests ---

func TestTwilioAuthRequired(t *testing.T) {
	_, tc := setupTwilio(t)

	resp := tc.Get(msgPath("/Messages.json"))
	resp.AssertStatus(401)
	resp.AssertBodyContains("Authenticate")
}

// --- Message Tests ---

func TestCreateAndGetMessage(t *testing.T) {
	_, tc := setupTwilio(t)

	status, m := twilioPostForm(t, tc, msgPath("/Messages.json"), map[string]string{
		"To":   "+15551234567",
		"From": "+15559876543",
		"Body": "Hello from test",
	})
	if status != 201 {
		t.Fatalf("expected 201, got %d: %v", status, m)
	}
	sid, ok := m["sid"].(string)
	if !ok || sid == "" {
		t.Fatal("expected message SID")
	}
	if m["status"] != "delivered" {
		t.Errorf("expected status=delivered, got %v", m["status"])
	}
	if m["to"] != "+15551234567" {
		t.Errorf("expected to=+15551234567, got %v", m["to"])
	}

	// Get message
	resp := twilioGet(tc, msgPath("/Messages/"+sid+".json"))
	resp.AssertStatus(200)
	got := resp.JSONMap()
	if got["sid"] != sid {
		t.Errorf("expected sid=%s, got %v", sid, got["sid"])
	}
}

func TestListMessages(t *testing.T) {
	_, tc := setupTwilio(t)

	twilioPostForm(t, tc, msgPath("/Messages.json"), map[string]string{
		"To": "+15551111111", "From": "+15550000000", "Body": "msg1",
	})
	twilioPostForm(t, tc, msgPath("/Messages.json"), map[string]string{
		"To": "+15552222222", "From": "+15550000000", "Body": "msg2",
	})

	resp := twilioGet(tc, msgPath("/Messages.json"))
	resp.AssertStatus(200)
	m := resp.JSONMap()
	msgs, ok := m["messages"].([]any)
	if !ok || len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages, got %v", m["messages"])
	}
}

func TestCreateMessageMissingTo(t *testing.T) {
	_, tc := setupTwilio(t)

	status, m := twilioPostForm(t, tc, msgPath("/Messages.json"), map[string]string{
		"From": "+15550000000",
		"Body": "missing to",
	})
	if status != 400 {
		t.Errorf("expected 400, got %d", status)
	}
	if code, _ := m["code"].(float64); code != 21604 {
		t.Errorf("expected error code 21604, got %v", m["code"])
	}
}

func TestCreateMessageMissingBody(t *testing.T) {
	_, tc := setupTwilio(t)

	status, m := twilioPostForm(t, tc, msgPath("/Messages.json"), map[string]string{
		"To":   "+15551234567",
		"From": "+15550000000",
	})
	if status != 400 {
		t.Errorf("expected 400, got %d", status)
	}
	if code, _ := m["code"].(float64); code != 21602 {
		t.Errorf("expected error code 21602, got %v", m["code"])
	}
}

func TestMessageNotFound(t *testing.T) {
	_, tc := setupTwilio(t)

	resp := twilioGet(tc, msgPath("/Messages/SM_nonexistent.json"))
	resp.AssertStatus(404)
}

// --- Verify Tests ---

func TestCreateAndCheckVerification(t *testing.T) {
	_, tc := setupTwilio(t)

	// Create verification
	status, m := twilioPostForm(t, tc, verifySvcPath("/Verifications"), map[string]string{
		"To":      "+15551234567",
		"Channel": "sms",
	})
	if status != 201 {
		t.Fatalf("expected 201, got %d: %v", status, m)
	}
	if m["status"] != "pending" {
		t.Errorf("expected status=pending, got %v", m["status"])
	}
	if m["to"] != "+15551234567" {
		t.Errorf("expected to=+15551234567, got %v", m["to"])
	}

	// Get the code via admin endpoint (URL-encode + as %2B)
	resp := tc.Get("/admin/verifications?to=%2B15551234567")
	resp.AssertStatus(200)
	adminData := resp.JSONMap()
	verifications := adminData["verifications"].([]any)
	if len(verifications) == 0 {
		t.Fatal("expected at least 1 verification in admin list")
	}
	v := verifications[0].(map[string]any)
	code := v["code"].(string)
	if code == "" {
		t.Fatal("expected non-empty code from admin endpoint")
	}

	// Check with correct code
	status, m = twilioPostForm(t, tc, verifySvcPath("/VerificationCheck"), map[string]string{
		"To":   "+15551234567",
		"Code": code,
	})
	if status != 200 {
		t.Fatalf("expected 200, got %d: %v", status, m)
	}
	if m["status"] != "approved" {
		t.Errorf("expected status=approved, got %v", m["status"])
	}
	if m["valid"] != true {
		t.Errorf("expected valid=true, got %v", m["valid"])
	}
}

func TestCheckVerificationWrongCode(t *testing.T) {
	_, tc := setupTwilio(t)

	twilioPostForm(t, tc, verifySvcPath("/Verifications"), map[string]string{
		"To":      "+15553333333",
		"Channel": "sms",
	})

	status, m := twilioPostForm(t, tc, verifySvcPath("/VerificationCheck"), map[string]string{
		"To":   "+15553333333",
		"Code": "000000",
	})
	if status != 200 {
		t.Fatalf("expected 200, got %d: %v", status, m)
	}
	if m["valid"] != false {
		t.Errorf("expected valid=false, got %v", m["valid"])
	}
}

func TestGetVerification(t *testing.T) {
	_, tc := setupTwilio(t)

	status, m := twilioPostForm(t, tc, verifySvcPath("/Verifications"), map[string]string{
		"To":      "+15554444444",
		"Channel": "sms",
	})
	if status != 201 {
		t.Fatalf("expected 201, got %d", status)
	}
	sid := m["sid"].(string)

	resp := twilioGet(tc, verifySvcPath("/Verifications/"+sid))
	resp.AssertStatus(200)
	got := resp.JSONMap()
	if got["sid"] != sid {
		t.Errorf("expected sid=%s, got %v", sid, got["sid"])
	}
}

func TestCheckVerificationMissingTo(t *testing.T) {
	_, tc := setupTwilio(t)

	status, _ := twilioPostForm(t, tc, verifySvcPath("/VerificationCheck"), map[string]string{
		"Code": "123456",
	})
	if status != 400 {
		t.Errorf("expected 400, got %d", status)
	}
}

// --- Admin Tests ---

func TestAdminListMessages(t *testing.T) {
	_, tc := setupTwilio(t)

	twilioPostForm(t, tc, msgPath("/Messages.json"), map[string]string{
		"To": "+15555555555", "From": "+15550000000", "Body": "admin test msg",
	})

	resp := tc.Get("/admin/messages?to=%2B15555555555")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	msgs, ok := m["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %v", m["messages"])
	}
}

func TestAdminGetOTP(t *testing.T) {
	_, tc := setupTwilio(t)

	twilioPostForm(t, tc, msgPath("/Messages.json"), map[string]string{
		"To":   "+15556666666",
		"From": "+15550000000",
		"Body": "Your code is 123456. Use it now.",
	})

	resp := tc.Get("/admin/otp?to=%2B15556666666")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["found"] != true {
		t.Error("expected found=true")
	}
	if m["code"] != "123456" {
		t.Errorf("expected code=123456, got %v", m["code"])
	}
}

func TestAdminExpireVerification(t *testing.T) {
	_, tc := setupTwilio(t)

	// Create a verification
	_, m := twilioPostForm(t, tc, verifySvcPath("/Verifications"), map[string]string{
		"To":      "+15557777777",
		"Channel": "sms",
	})
	sid := m["sid"].(string)

	// Expire it via admin
	resp := tc.Post("/admin/verifications/"+sid+"/expire", nil)
	resp.AssertStatus(200)
	got := resp.JSONMap()
	if got["status"] != "canceled" {
		t.Errorf("expected status=canceled, got %v", got["status"])
	}
}

func TestAdminReset(t *testing.T) {
	_, tc := setupTwilio(t)

	twilioPostForm(t, tc, msgPath("/Messages.json"), map[string]string{
		"To": "+15551111111", "From": "+15550000000", "Body": "reset test",
	})

	tc.Post("/admin/reset", nil).AssertStatus(200)

	resp := tc.Get("/admin/messages")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	msgs := m["messages"]
	// After reset, should be null or empty
	if msgs != nil {
		if arr, ok := msgs.([]any); ok && len(arr) > 0 {
			t.Errorf("expected 0 messages after reset, got %d", len(arr))
		}
	}
}

func TestAdminHealth(t *testing.T) {
	_, tc := setupTwilio(t)
	tc.Get("/admin/health").AssertStatus(200)
}
