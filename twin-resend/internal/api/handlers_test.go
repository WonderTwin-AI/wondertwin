package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/pkg/admin"
	"github.com/wondertwin-ai/wondertwin/pkg/testutil"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
)

func setupResend(t *testing.T) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-resend-test"}
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

var resendHeaders = map[string]string{
	"Authorization": "Bearer re_sim_test_123",
}

func resendPost(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("POST", path, body, resendHeaders)
}

func resendGet(tc *testutil.TwinClient, path string) *testutil.Response {
	return tc.DoWithHeaders("GET", path, nil, resendHeaders)
}

// --- Auth Tests ---

func TestResendAuthRequired(t *testing.T) {
	_, tc := setupResend(t)

	resp := tc.Post("/emails", map[string]any{})
	resp.AssertStatus(401)
	resp.AssertBodyContains("missing_api_key")
}

func TestResendAuthInvalidFormat(t *testing.T) {
	_, tc := setupResend(t)

	resp := tc.DoWithHeaders("POST", "/emails", map[string]any{}, map[string]string{
		"Authorization": "InvalidNoBearer",
	})
	resp.AssertStatus(401)
	resp.AssertBodyContains("invalid_api_key")
}

// --- Email Tests ---

func TestSendAndGetEmail(t *testing.T) {
	_, tc := setupResend(t)

	resp := resendPost(tc, "/emails", map[string]any{
		"from":    "sender@example.com",
		"to":      []string{"recipient@example.com"},
		"subject": "Test Email",
		"html":    "<p>Hello</p>",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected email id in response")
	}

	// Get email
	resp = resendGet(tc, "/emails/"+id)
	resp.AssertStatus(200)
	got := resp.JSONMap()
	if got["id"] != id {
		t.Errorf("expected id=%s, got %v", id, got["id"])
	}
	if got["from"] != "sender@example.com" {
		t.Errorf("expected from=sender@example.com, got %v", got["from"])
	}
	if got["subject"] != "Test Email" {
		t.Errorf("expected subject=Test Email, got %v", got["subject"])
	}
	if got["status"] != "delivered" {
		t.Errorf("expected status=delivered, got %v", got["status"])
	}
}

func TestSendEmailMissingFrom(t *testing.T) {
	_, tc := setupResend(t)

	resp := resendPost(tc, "/emails", map[string]any{
		"to":      []string{"recipient@example.com"},
		"subject": "No From",
	})
	resp.AssertStatus(422)
	resp.AssertBodyContains("validation_error")
}

func TestSendEmailMissingTo(t *testing.T) {
	_, tc := setupResend(t)

	resp := resendPost(tc, "/emails", map[string]any{
		"from":    "sender@example.com",
		"subject": "No To",
	})
	resp.AssertStatus(422)
	resp.AssertBodyContains("validation_error")
}

func TestSendEmailMissingSubject(t *testing.T) {
	_, tc := setupResend(t)

	resp := resendPost(tc, "/emails", map[string]any{
		"from": "sender@example.com",
		"to":   []string{"recipient@example.com"},
	})
	resp.AssertStatus(422)
	resp.AssertBodyContains("validation_error")
}

func TestEmailNotFound(t *testing.T) {
	_, tc := setupResend(t)

	resp := resendGet(tc, "/emails/email_nonexistent")
	resp.AssertStatus(404)
	resp.AssertBodyContains("not_found")
}

func TestSendBatchEmails(t *testing.T) {
	_, tc := setupResend(t)

	resp := resendPost(tc, "/emails/batch", []map[string]any{
		{
			"from":    "sender@example.com",
			"to":      []string{"a@example.com"},
			"subject": "Batch 1",
			"html":    "<p>A</p>",
		},
		{
			"from":    "sender@example.com",
			"to":      []string{"b@example.com"},
			"subject": "Batch 2",
			"html":    "<p>B</p>",
		},
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	data, ok := m["data"].([]any)
	if !ok || len(data) != 2 {
		t.Fatalf("expected 2 batch results, got %v", m["data"])
	}
	for i, item := range data {
		entry := item.(map[string]any)
		if entry["id"] == nil || entry["id"] == "" {
			t.Errorf("batch item %d missing id", i)
		}
	}
}

// --- Admin Tests ---

func TestAdminListEmails(t *testing.T) {
	_, tc := setupResend(t)

	resendPost(tc, "/emails", map[string]any{
		"from":    "sender@example.com",
		"to":      []string{"alice@example.com"},
		"subject": "Hello Alice",
		"html":    "<p>Hi</p>",
	}).AssertStatus(200)

	resp := tc.Get("/admin/emails?to=alice@example.com")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	emails, ok := m["emails"].([]any)
	if !ok || len(emails) != 1 {
		t.Fatalf("expected 1 email, got %v", m["emails"])
	}
}

func TestAdminListEmailsBySubject(t *testing.T) {
	_, tc := setupResend(t)

	resendPost(tc, "/emails", map[string]any{
		"from":    "sender@example.com",
		"to":      []string{"bob@example.com"},
		"subject": "Password Reset",
		"html":    "<p>Reset your password</p>",
	}).AssertStatus(200)

	resp := tc.Get("/admin/emails?subject=password")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	emails := m["emails"].([]any)
	if len(emails) != 1 {
		t.Fatalf("expected 1 email, got %d", len(emails))
	}
}

func TestAdminReset(t *testing.T) {
	_, tc := setupResend(t)

	resendPost(tc, "/emails", map[string]any{
		"from":    "sender@example.com",
		"to":      []string{"ghost@example.com"},
		"subject": "Ghost",
		"html":    "<p>boo</p>",
	}).AssertStatus(200)

	tc.Post("/admin/reset", nil).AssertStatus(200)

	resp := tc.Get("/admin/emails")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	emails := m["emails"]
	if emails != nil {
		if arr, ok := emails.([]any); ok && len(arr) > 0 {
			t.Errorf("expected 0 emails after reset, got %d", len(arr))
		}
	}
}

func TestAdminHealth(t *testing.T) {
	_, tc := setupResend(t)
	tc.Get("/admin/health").AssertStatus(200)
}
