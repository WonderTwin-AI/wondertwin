package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-clerk/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-clerk/internal/store"
)

func setupClerk(t *testing.T) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-clerk-test"}
	twin := twincore.New(cfg)
	jwtMgr, err := api.NewJWTManager()
	if err != nil {
		t.Fatalf("failed to create JWT manager: %v", err)
	}
	handler := api.NewHandler(memStore, twin.Middleware(), jwtMgr)
	handler.Routes(twin.Router)
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	tc := testutil.NewTwinClient(t, srv)
	return srv, tc
}

// clerkAuth returns headers with Clerk-style Bearer auth.
var clerkHeaders = map[string]string{
	"Authorization": "Bearer sk_test_sim_123",
}

func clerkPost(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("POST", path, body, clerkHeaders)
}

func clerkGet(tc *testutil.TwinClient, path string) *testutil.Response {
	return tc.DoWithHeaders("GET", path, nil, clerkHeaders)
}

func clerkPatch(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("PATCH", path, body, clerkHeaders)
}

func clerkDelete(tc *testutil.TwinClient, path string) *testutil.Response {
	return tc.DoWithHeaders("DELETE", path, nil, clerkHeaders)
}

// --- Auth Tests ---

func TestClerkAuthRequired(t *testing.T) {
	_, tc := setupClerk(t)

	resp := tc.Get("/v1/users")
	resp.AssertStatus(401)
	resp.AssertBodyContains("authentication_invalid")
}

func TestClerkAuthInvalidPrefix(t *testing.T) {
	_, tc := setupClerk(t)

	resp := tc.DoWithHeaders("GET", "/v1/users", nil, map[string]string{
		"Authorization": "Bearer bad_key_123",
	})
	resp.AssertStatus(401)
	resp.AssertBodyContains("authentication_invalid")
}

// --- User Tests ---

func TestCreateAndGetUser(t *testing.T) {
	_, tc := setupClerk(t)

	resp := clerkPost(tc, "/v1/users", map[string]any{
		"email_address": []string{"test@example.com"},
		"first_name":    "Alice",
		"last_name":     "Smith",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected user id")
	}
	if m["object"] != "user" {
		t.Errorf("expected object=user, got %v", m["object"])
	}
	if m["first_name"] != "Alice" {
		t.Errorf("expected first_name=Alice, got %v", m["first_name"])
	}

	// Verify email address was attached
	emails, _ := m["email_addresses"].([]any)
	if len(emails) != 1 {
		t.Fatalf("expected 1 email address, got %d", len(emails))
	}

	// Get user
	resp = clerkGet(tc, "/v1/users/"+id)
	resp.AssertStatus(200)
	m = resp.JSONMap()
	if m["id"] != id {
		t.Errorf("expected id=%s, got %v", id, m["id"])
	}
}

func TestUpdateUser(t *testing.T) {
	_, tc := setupClerk(t)

	// Create user
	resp := clerkPost(tc, "/v1/users", map[string]any{
		"first_name": "Bob",
		"last_name":  "Jones",
	})
	id := resp.JSONMap()["id"].(string)

	// Update user
	resp = clerkPatch(tc, "/v1/users/"+id, map[string]any{
		"first_name": "Robert",
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["first_name"] != "Robert" {
		t.Errorf("expected first_name=Robert, got %v", m["first_name"])
	}
	// last_name should be unchanged
	if m["last_name"] != "Jones" {
		t.Errorf("expected last_name=Jones, got %v", m["last_name"])
	}
}

func TestDeleteUser(t *testing.T) {
	_, tc := setupClerk(t)

	resp := clerkPost(tc, "/v1/users", map[string]any{
		"first_name": "Delete",
	})
	id := resp.JSONMap()["id"].(string)

	resp = clerkDelete(tc, "/v1/users/"+id)
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["deleted"] != true {
		t.Error("expected deleted=true")
	}

	// Verify gone
	clerkGet(tc, "/v1/users/"+id).AssertStatus(404)
}

func TestListUsers(t *testing.T) {
	_, tc := setupClerk(t)

	clerkPost(tc, "/v1/users", map[string]any{"first_name": "User1"}).AssertStatus(200)
	clerkPost(tc, "/v1/users", map[string]any{"first_name": "User2"}).AssertStatus(200)

	resp := clerkGet(tc, "/v1/users")
	resp.AssertStatus(200)

	m := resp.JSONMap()
	data, ok := m["data"].([]any)
	if !ok || len(data) < 2 {
		t.Fatalf("expected at least 2 users, got %v", m["data"])
	}
	count, ok := m["total_count"].(float64)
	if !ok || count < 2 {
		t.Errorf("expected total_count >= 2, got %v", m["total_count"])
	}
}

func TestListUsersFilterByEmail(t *testing.T) {
	_, tc := setupClerk(t)

	clerkPost(tc, "/v1/users", map[string]any{
		"email_address": []string{"alice@example.com"},
		"first_name":    "Alice",
	}).AssertStatus(200)
	clerkPost(tc, "/v1/users", map[string]any{
		"email_address": []string{"bob@example.com"},
		"first_name":    "Bob",
	}).AssertStatus(200)

	resp := clerkGet(tc, "/v1/users?email_address=alice@example.com")
	resp.AssertStatus(200)

	m := resp.JSONMap()
	data := m["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("expected 1 user, got %d", len(data))
	}
	user := data[0].(map[string]any)
	if user["first_name"] != "Alice" {
		t.Errorf("expected Alice, got %v", user["first_name"])
	}
}

func TestUserNotFound(t *testing.T) {
	_, tc := setupClerk(t)

	resp := clerkGet(tc, "/v1/users/user_nonexistent")
	resp.AssertStatus(404)
	resp.AssertBodyContains("resource_not_found")
}

// --- Session Tests ---

func TestCreateAndGetSession(t *testing.T) {
	_, tc := setupClerk(t)

	// Create a user first (sessions require user_id)
	resp := clerkPost(tc, "/v1/users", map[string]any{"first_name": "SessionUser"})
	userID := resp.JSONMap()["id"].(string)

	// Create session via admin endpoint
	resp = tc.Post("/admin/sessions", map[string]any{"user_id": userID})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	sessID, ok := m["id"].(string)
	if !ok || sessID == "" {
		t.Fatal("expected session id")
	}
	if m["object"] != "session" {
		t.Errorf("expected object=session, got %v", m["object"])
	}
	if m["status"] != "active" {
		t.Errorf("expected status=active, got %v", m["status"])
	}

	// Get session via authenticated endpoint
	resp = clerkGet(tc, "/v1/sessions/"+sessID)
	resp.AssertStatus(200)
	m = resp.JSONMap()
	if m["id"] != sessID {
		t.Errorf("expected id=%s, got %v", sessID, m["id"])
	}
}

func TestListSessions(t *testing.T) {
	_, tc := setupClerk(t)

	resp := clerkPost(tc, "/v1/users", map[string]any{"first_name": "Lister"})
	userID := resp.JSONMap()["id"].(string)

	tc.Post("/admin/sessions", map[string]any{"user_id": userID}).AssertStatus(200)
	tc.Post("/admin/sessions", map[string]any{"user_id": userID}).AssertStatus(200)

	resp = clerkGet(tc, "/v1/sessions")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	data := m["data"].([]any)
	if len(data) < 2 {
		t.Fatalf("expected at least 2 sessions, got %d", len(data))
	}
}

func TestRevokeSession(t *testing.T) {
	_, tc := setupClerk(t)

	resp := clerkPost(tc, "/v1/users", map[string]any{"first_name": "RevokeMe"})
	userID := resp.JSONMap()["id"].(string)

	resp = tc.Post("/admin/sessions", map[string]any{"user_id": userID})
	sessID := resp.JSONMap()["id"].(string)

	resp = clerkPost(tc, "/v1/sessions/"+sessID+"/revoke", nil)
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["status"] != "revoked" {
		t.Errorf("expected status=revoked, got %v", m["status"])
	}
}

func TestVerifySession(t *testing.T) {
	_, tc := setupClerk(t)

	resp := clerkPost(tc, "/v1/users", map[string]any{"first_name": "VerifyMe"})
	userID := resp.JSONMap()["id"].(string)

	resp = tc.Post("/admin/sessions", map[string]any{"user_id": userID})
	sessID := resp.JSONMap()["id"].(string)

	resp = clerkPost(tc, "/v1/sessions/"+sessID+"/verify", map[string]any{"token": "ignored"})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["status"] != "active" {
		t.Errorf("expected status=active, got %v", m["status"])
	}
	// Should have last_active_token
	token, ok := m["last_active_token"].(map[string]any)
	if !ok {
		t.Fatal("expected last_active_token")
	}
	if token["jwt"] == nil || token["jwt"] == "" {
		t.Error("expected JWT in last_active_token")
	}
}

func TestSessionNotFound(t *testing.T) {
	_, tc := setupClerk(t)

	resp := clerkGet(tc, "/v1/sessions/sess_nonexistent")
	resp.AssertStatus(404)
	resp.AssertBodyContains("resource_not_found")
}

// --- Organization Tests ---

func TestCreateAndGetOrganization(t *testing.T) {
	_, tc := setupClerk(t)

	resp := clerkPost(tc, "/v1/organizations", map[string]any{
		"name": "Acme Corp",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected organization id")
	}
	if m["object"] != "organization" {
		t.Errorf("expected object=organization, got %v", m["object"])
	}
	if m["name"] != "Acme Corp" {
		t.Errorf("expected name=Acme Corp, got %v", m["name"])
	}
	if m["slug"] != "acme-corp" {
		t.Errorf("expected slug=acme-corp, got %v", m["slug"])
	}

	// Get organization
	resp = clerkGet(tc, "/v1/organizations/"+id)
	resp.AssertStatus(200)
	m = resp.JSONMap()
	if m["id"] != id {
		t.Errorf("expected id=%s, got %v", id, m["id"])
	}
}

func TestGetOrganizationBySlug(t *testing.T) {
	_, tc := setupClerk(t)

	resp := clerkPost(tc, "/v1/organizations", map[string]any{
		"name": "Slug Test Org",
		"slug": "my-org",
	})
	resp.AssertStatus(200)

	resp = clerkGet(tc, "/v1/organizations/my-org")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["slug"] != "my-org" {
		t.Errorf("expected slug=my-org, got %v", m["slug"])
	}
}

func TestUpdateOrganization(t *testing.T) {
	_, tc := setupClerk(t)

	resp := clerkPost(tc, "/v1/organizations", map[string]any{"name": "Old Name"})
	id := resp.JSONMap()["id"].(string)

	resp = clerkPatch(tc, "/v1/organizations/"+id, map[string]any{
		"name": "New Name",
	})
	resp.AssertStatus(200)
	if resp.JSONMap()["name"] != "New Name" {
		t.Errorf("expected name=New Name, got %v", resp.JSONMap()["name"])
	}
}

func TestDeleteOrganization(t *testing.T) {
	_, tc := setupClerk(t)

	resp := clerkPost(tc, "/v1/organizations", map[string]any{"name": "DeleteMe"})
	id := resp.JSONMap()["id"].(string)

	resp = clerkDelete(tc, "/v1/organizations/"+id)
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["deleted"] != true {
		t.Error("expected deleted=true")
	}

	clerkGet(tc, "/v1/organizations/"+id).AssertStatus(404)
}

func TestListOrganizations(t *testing.T) {
	_, tc := setupClerk(t)

	clerkPost(tc, "/v1/organizations", map[string]any{"name": "Org1"}).AssertStatus(200)
	clerkPost(tc, "/v1/organizations", map[string]any{"name": "Org2"}).AssertStatus(200)

	resp := clerkGet(tc, "/v1/organizations")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	data := m["data"].([]any)
	if len(data) < 2 {
		t.Fatalf("expected at least 2 organizations, got %d", len(data))
	}
}

func TestOrganizationNameRequired(t *testing.T) {
	_, tc := setupClerk(t)

	resp := clerkPost(tc, "/v1/organizations", map[string]any{})
	resp.AssertStatus(422)
	resp.AssertBodyContains("form_param_missing")
}

// --- JWKS Tests ---

func TestJWKSEndpoint(t *testing.T) {
	_, tc := setupClerk(t)

	// JWKS is public, no auth needed
	resp := tc.Get("/.well-known/jwks.json")
	resp.AssertStatus(200)

	m := resp.JSONMap()
	keys, ok := m["keys"].([]any)
	if !ok || len(keys) == 0 {
		t.Fatal("expected at least 1 key in JWKS")
	}
	key := keys[0].(map[string]any)
	if key["kty"] != "RSA" {
		t.Errorf("expected kty=RSA, got %v", key["kty"])
	}
	if key["alg"] != "RS256" {
		t.Errorf("expected alg=RS256, got %v", key["alg"])
	}
}

// --- Admin Tests ---

func TestAdminGenerateJWT(t *testing.T) {
	_, tc := setupClerk(t)

	resp := tc.Post("/admin/jwt/generate", map[string]any{
		"user_id":    "user_test123",
		"session_id": "sess_test123",
	})
	resp.AssertStatus(200)

	m := resp.JSONMap()
	token, ok := m["token"].(string)
	if !ok || token == "" {
		t.Fatal("expected JWT token")
	}
	if m["user_id"] != "user_test123" {
		t.Errorf("expected user_id=user_test123, got %v", m["user_id"])
	}
}

func TestAdminReset(t *testing.T) {
	_, tc := setupClerk(t)

	// Create a user
	clerkPost(tc, "/v1/users", map[string]any{"first_name": "Ghost"}).AssertStatus(200)

	// Reset
	tc.Post("/admin/reset", nil).AssertStatus(200)

	// List should be empty
	resp := clerkGet(tc, "/v1/users")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	count := m["total_count"].(float64)
	if count != 0 {
		t.Errorf("expected 0 users after reset, got %v", count)
	}
}

func TestAdminHealth(t *testing.T) {
	_, tc := setupClerk(t)
	tc.Get("/admin/health").AssertStatus(200)
}
