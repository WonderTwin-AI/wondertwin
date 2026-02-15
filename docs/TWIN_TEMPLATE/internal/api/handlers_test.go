package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-TEMPLATE/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-TEMPLATE/internal/store"
)

// setupTwin creates a test server with the twin wired up.
// Every test file should have a setup function like this.
func setupTwin(t *testing.T) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-TEMPLATE-test"}
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

// authHeaders returns the default auth headers for test requests.
// Adjust the token format to match the real service's auth pattern.
var authHeaders = map[string]string{
	"Authorization": "Bearer test_key_123",
}

func authedPost(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("POST", path, body, authHeaders)
}

func authedGet(tc *testutil.TwinClient, path string) *testutil.Response {
	return tc.DoWithHeaders("GET", path, nil, authHeaders)
}

func authedPatch(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("PATCH", path, body, authHeaders)
}

func authedDelete(tc *testutil.TwinClient, path string) *testutil.Response {
	return tc.DoWithHeaders("DELETE", path, nil, authHeaders)
}

// --- Auth Tests ---

func TestAuthRequired(t *testing.T) {
	_, tc := setupTwin(t)

	resp := tc.Post("/v1/resources", map[string]any{})
	resp.AssertStatus(401)
	resp.AssertBodyContains("authentication_error")
}

// --- CRUD Tests ---

func TestCreateAndGetResource(t *testing.T) {
	_, tc := setupTwin(t)

	// Create
	resp := authedPost(tc, "/v1/resources", map[string]any{
		"name":        "Test Resource",
		"description": "A test resource for validation",
	})
	resp.AssertStatus(201)

	m := resp.JSONMap()
	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected resource id in response")
	}

	// Get
	resp = authedGet(tc, "/v1/resources/"+id)
	resp.AssertStatus(200)
	got := resp.JSONMap()
	if got["id"] != id {
		t.Errorf("expected id=%s, got %v", id, got["id"])
	}
	if got["name"] != "Test Resource" {
		t.Errorf("expected name=Test Resource, got %v", got["name"])
	}
}

func TestCreateResourceMissingName(t *testing.T) {
	_, tc := setupTwin(t)

	resp := authedPost(tc, "/v1/resources", map[string]any{
		"description": "no name provided",
	})
	resp.AssertStatus(422)
	resp.AssertBodyContains("invalid_request_error")
}

func TestGetResourceNotFound(t *testing.T) {
	_, tc := setupTwin(t)

	resp := authedGet(tc, "/v1/resources/nonexistent_id")
	resp.AssertStatus(404)
	resp.AssertBodyContains("invalid_request_error")
}

func TestListResources(t *testing.T) {
	_, tc := setupTwin(t)

	authedPost(tc, "/v1/resources", map[string]any{
		"name": "Resource A",
	}).AssertStatus(201)

	authedPost(tc, "/v1/resources", map[string]any{
		"name": "Resource B",
	}).AssertStatus(201)

	resp := authedGet(tc, "/v1/resources")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	data, ok := m["data"].([]any)
	if !ok || len(data) != 2 {
		t.Fatalf("expected 2 resources, got %v", m["data"])
	}
}

func TestDeleteResource(t *testing.T) {
	_, tc := setupTwin(t)

	resp := authedPost(tc, "/v1/resources", map[string]any{
		"name": "To Delete",
	})
	resp.AssertStatus(201)
	id := resp.JSONMap()["id"].(string)

	resp = authedDelete(tc, "/v1/resources/"+id)
	resp.AssertStatus(200)
	resp.AssertBodyContains("deleted")

	// Verify it is gone.
	resp = authedGet(tc, "/v1/resources/"+id)
	resp.AssertStatus(404)
}

// --- Admin Tests ---

func TestAdminHealth(t *testing.T) {
	_, tc := setupTwin(t)
	tc.Get("/admin/health").AssertStatus(200)
}

func TestAdminReset(t *testing.T) {
	_, tc := setupTwin(t)

	authedPost(tc, "/v1/resources", map[string]any{
		"name": "Will Be Cleared",
	}).AssertStatus(201)

	tc.Post("/admin/reset", nil).AssertStatus(200)

	resp := authedGet(tc, "/v1/resources")
	resp.AssertStatus(200)
	m := resp.JSONMap()
	data, ok := m["data"].([]any)
	if ok && len(data) > 0 {
		t.Errorf("expected 0 resources after reset, got %d", len(data))
	}
}
