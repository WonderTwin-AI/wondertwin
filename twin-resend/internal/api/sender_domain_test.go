package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// setupResendWithConfig wires /admin/config the way main.go does.
func setupResendWithConfig(t *testing.T) *testutil.TwinClient {
	t.Helper()
	tc, _ := setupResendWithStore(t)
	return tc
}

// setupResendWithStore is setupResendWithConfig that also returns the store.
func setupResendWithStore(t *testing.T) (*testutil.TwinClient, *store.MemoryStore) {
	t.Helper()
	twin := twincore.New(&twincore.Config{Name: "twin-resend-test"})
	memStore := store.New()
	memStore.Rand = twin.Rand
	handler := api.NewHandler(memStore, twin.Middleware())
	handler.Routes(twin.Router)
	adm := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adm.SetConfigProvider(handler.ConfigProvider(twin))
	adm.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	return testutil.NewTwinClient(t, srv), memStore
}

func sendFrom(tc *testutil.TwinClient, from string, to any) *testutil.Response {
	return resendPost(tc, "/emails", map[string]any{"from": from, "to": to, "subject": "hi", "html": "<p>hi</p>"})
}

func createDomain(tc *testutil.TwinClient, name string, verify bool) {
	id := resendPost(tc, "/domains", map[string]any{"name": name}).AssertStatus(201).JSONMap()["id"].(string)
	if verify {
		resendPost(tc, "/domains/"+id+"/verify", nil).AssertStatus(200)
	}
}

func setStrict(tc *testutil.TwinClient, on bool) {
	tc.DoWithHeaders("PUT", "/admin/config", map[string]any{"strict_sender_domains": on}, nil).AssertStatus(200)
}

const unverifiedMsg = "The unverified.example.com domain is not verified. Please, add and verify your domain on https://resend.com/domains"

func TestSenderDomainDefaultMode(t *testing.T) {
	tc := setupResendWithConfig(t)

	sendFrom(tc, "a@unknown.example.com", "delivered@resend.dev").AssertStatus(200)
	createDomain(tc, "unverified.example.com", false)
	sendFrom(tc, "Sender <a@unverified.example.com>", "delivered@resend.dev").
		AssertStatus(403).AssertBodyContains(unverifiedMsg)
	createDomain(tc, "ok.example.com", true)
	sendFrom(tc, "a@ok.example.com", "someone@example.com").AssertStatus(200)
	sendFrom(tc, "onboarding@resend.dev", "someone@example.com").AssertStatus(200)
}

func TestSenderDomainStrictMode(t *testing.T) {
	tc := setupResendWithConfig(t)
	setStrict(tc, true)
	if got := tc.Get("/admin/config").AssertStatus(200).JSONMap()["strict_sender_domains"]; got != true {
		t.Fatalf("admin config strict_sender_domains = %v, want true", got)
	}

	sendFrom(tc, "snap@unverified.example.com", "delivered@resend.dev").
		AssertStatus(403).AssertBodyContains(unverifiedMsg)
	createDomain(tc, "ok.example.com", true)
	sendFrom(tc, "a@ok.example.com", "someone@example.com").AssertStatus(200)
	sendFrom(tc, "onboarding@resend.dev", "bounced@resend.dev").AssertStatus(200)
	sendFrom(tc, "onboarding@resend.dev", "someone@example.com").
		AssertStatus(403).AssertBodyContains("You can only send testing emails to your own email address")

	setStrict(tc, false)
	sendFrom(tc, "a@unknown.example.com", "delivered@resend.dev").AssertStatus(200)
}

func TestSenderDomainBatch(t *testing.T) {
	tc := setupResendWithConfig(t)
	createDomain(tc, "unverified.example.com", false)
	good := map[string]any{"from": "onboarding@resend.dev", "to": "delivered@resend.dev", "subject": "hi", "html": "<p>hi</p>"}
	bad := map[string]any{"from": "a@unverified.example.com", "to": "delivered@resend.dev", "subject": "hi", "html": "<p>hi</p>"}

	resendPost(tc, "/emails/batch", []any{good, bad}).AssertStatus(403).AssertBodyContains(unverifiedMsg)
	tc.DoWithHeaders("POST", "/emails/batch", []any{good, bad}, map[string]string{
		"Authorization": "Bearer re_sim_test_123", "x-batch-validation": "permissive",
	}).AssertStatus(202).AssertBodyContains(`"index":1`)
}

func TestSenderDomainConfigRejectsNonBoolean(t *testing.T) {
	tc := setupResendWithConfig(t)
	resp := tc.DoWithHeaders("PUT", "/admin/config", map[string]any{"strict_sender_domains": "yes"}, nil)
	if resp.StatusCode < 400 {
		t.Errorf("non-boolean strict_sender_domains answered %d", resp.StatusCode)
	}
}
