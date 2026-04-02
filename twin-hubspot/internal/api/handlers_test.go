package api_test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-hubspot/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-hubspot/internal/store"
)

func setupHubSpot(t *testing.T) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	memStore := store.New()
	cfg := &twincore.Config{Name: "twin-hubspot-test"}
	twin := twincore.New(cfg)
	handler := api.NewHandler(memStore, twin.Middleware(), nil, nil)
	handler.Routes(twin.Router)
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.Routes(twin.Router)
	srv := httptest.NewServer(twin.Router)
	t.Cleanup(srv.Close)
	tc := testutil.NewTwinClient(t, srv)
	return srv, tc
}

var hsHeaders = map[string]string{
	"Authorization": "Bearer pat-test-token",
	"Content-Type":  "application/json",
}

func hsGet(tc *testutil.TwinClient, path string) *testutil.Response {
	return tc.DoWithHeaders("GET", path, nil, hsHeaders)
}

func hsPost(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("POST", path, body, hsHeaders)
}

func hsPatch(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("PATCH", path, body, hsHeaders)
}

func hsDelete(tc *testutil.TwinClient, path string) *testutil.Response {
	return tc.DoWithHeaders("DELETE", path, nil, hsHeaders)
}

func hsPut(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("PUT", path, body, hsHeaders)
}

// --- Auth ---

func TestAuthRequired(t *testing.T) {
	_, tc := setupHubSpot(t)
	resp := tc.Get("/crm/v3/objects/contacts")
	resp.AssertStatus(401)
}

// --- CRM Objects (Contacts) ---

func TestContactCRUD(t *testing.T) {
	_, tc := setupHubSpot(t)

	// Create
	resp := hsPost(tc, "/crm/v3/objects/contacts", map[string]any{
		"properties": map[string]any{
			"email":     "alice@example.com",
			"firstname": "Alice",
			"lastname":  "Smith",
		},
	})
	resp.AssertStatus(201)
	contact := resp.JSONMap()
	id := contact["id"].(string)
	props := contact["properties"].(map[string]any)
	if props["email"] != "alice@example.com" {
		t.Error("expected email=alice@example.com")
	}

	// Get
	resp = hsGet(tc, "/crm/v3/objects/contacts/"+id)
	resp.AssertStatus(200)

	// Update
	resp = hsPatch(tc, "/crm/v3/objects/contacts/"+id, map[string]any{
		"properties": map[string]any{"firstname": "Alicia"},
	})
	resp.AssertStatus(200)
	if resp.JSONMap()["properties"].(map[string]any)["firstname"] != "Alicia" {
		t.Error("expected updated firstname")
	}

	// List
	resp = hsGet(tc, "/crm/v3/objects/contacts")
	resp.AssertStatus(200)
	results := resp.JSONMap()["results"].([]any)
	if len(results) != 1 {
		t.Errorf("expected 1 contact, got %d", len(results))
	}

	// Archive
	hsDelete(tc, "/crm/v3/objects/contacts/"+id).AssertStatus(204)
}

// --- CRM Objects (Deals) ---

func TestDealCRUD(t *testing.T) {
	_, tc := setupHubSpot(t)

	resp := hsPost(tc, "/crm/v3/objects/deals", map[string]any{
		"properties": map[string]any{
			"dealname": "Big Deal",
			"amount":   "50000",
		},
	})
	resp.AssertStatus(201)
	id := resp.JSONMap()["id"].(string)

	resp = hsGet(tc, "/crm/v3/objects/deals/"+id)
	resp.AssertStatus(200)
	if resp.JSONMap()["properties"].(map[string]any)["dealname"] != "Big Deal" {
		t.Error("expected dealname=Big Deal")
	}
}

// --- Batch Operations ---

func TestBatchCreate(t *testing.T) {
	_, tc := setupHubSpot(t)

	resp := hsPost(tc, "/crm/v3/objects/contacts/batch/create", map[string]any{
		"inputs": []map[string]any{
			{"properties": map[string]any{"email": "a@b.com"}},
			{"properties": map[string]any{"email": "c@d.com"}},
		},
	})
	resp.AssertStatus(201)
	m := resp.JSONMap()
	if m["status"] != "COMPLETE" {
		t.Error("expected status=COMPLETE")
	}
	results := m["results"].([]any)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

// --- Search ---

func TestSearch(t *testing.T) {
	_, tc := setupHubSpot(t)

	hsPost(tc, "/crm/v3/objects/contacts", map[string]any{
		"properties": map[string]any{"email": "searchme@example.com", "firstname": "Search"},
	})
	hsPost(tc, "/crm/v3/objects/contacts", map[string]any{
		"properties": map[string]any{"email": "other@example.com", "firstname": "Other"},
	})

	resp := hsPost(tc, "/crm/v3/objects/contacts/search", map[string]any{
		"query": "searchme",
	})
	resp.AssertStatus(200)
	results := resp.JSONMap()["results"].([]any)
	if len(results) != 1 {
		t.Errorf("expected 1 search result, got %d", len(results))
	}
}

// --- Associations ---

func TestAssociations(t *testing.T) {
	_, tc := setupHubSpot(t)

	// Create contact and company
	resp := hsPost(tc, "/crm/v3/objects/contacts", map[string]any{
		"properties": map[string]any{"email": "assoc@test.com"},
	})
	contactID := resp.JSONMap()["id"].(string)

	resp = hsPost(tc, "/crm/v3/objects/companies", map[string]any{
		"properties": map[string]any{"name": "ACME"},
	})
	companyID := resp.JSONMap()["id"].(string)

	// Create association
	hsPut(tc, fmt.Sprintf("/crm/v4/objects/contacts/%s/associations/default/companies/%s", contactID, companyID), nil).AssertStatus(200)

	// List associations
	resp = hsGet(tc, fmt.Sprintf("/crm/v4/objects/contacts/%s/associations/companies", contactID))
	resp.AssertStatus(200)
	results := resp.JSONMap()["results"].([]any)
	if len(results) != 1 {
		t.Errorf("expected 1 association, got %d", len(results))
	}
}

// --- Properties ---

func TestPropertyCRUD(t *testing.T) {
	_, tc := setupHubSpot(t)

	resp := hsPost(tc, "/crm/v3/properties/contacts", map[string]any{
		"name":      "custom_field",
		"label":     "Custom Field",
		"type":      "string",
		"fieldType": "text",
		"groupName": "contactinformation",
	})
	resp.AssertStatus(201)

	resp = hsGet(tc, "/crm/v3/properties/contacts")
	resp.AssertStatus(200)
	results := resp.JSONMap()["results"].([]any)
	if len(results) != 1 {
		t.Errorf("expected 1 property, got %d", len(results))
	}

	hsDelete(tc, "/crm/v3/properties/contacts/custom_field").AssertStatus(204)
}

// --- Pipelines ---

func TestPipelineCRUD(t *testing.T) {
	_, tc := setupHubSpot(t)

	resp := hsPost(tc, "/crm/v3/pipelines/deals", map[string]any{
		"label": "Sales Pipeline",
		"stages": []map[string]any{
			{"label": "Qualified", "displayOrder": 0},
			{"label": "Closed Won", "displayOrder": 1},
		},
	})
	resp.AssertStatus(201)
	pipeID := resp.JSONMap()["id"].(string)

	resp = hsGet(tc, "/crm/v3/pipelines/deals")
	resp.AssertStatus(200)
	results := resp.JSONMap()["results"].([]any)
	if len(results) != 1 {
		t.Errorf("expected 1 pipeline, got %d", len(results))
	}

	hsDelete(tc, "/crm/v3/pipelines/deals/"+pipeID).AssertStatus(204)
}

// --- Owners ---

func TestOwners(t *testing.T) {
	_, tc := setupHubSpot(t)

	tc.Post("/admin/state", json.RawMessage(`{"owners":{"owner_001":{"id":"owner_001","email":"owner@test.com","firstName":"Own","lastName":"Er"}}}`)).AssertStatus(200)

	resp := hsGet(tc, "/crm/v3/owners")
	resp.AssertStatus(200)
	results := resp.JSONMap()["results"].([]any)
	if len(results) != 1 {
		t.Errorf("expected 1 owner, got %d", len(results))
	}
}

// --- Lists ---

func TestListCRUD(t *testing.T) {
	_, tc := setupHubSpot(t)

	resp := hsPost(tc, "/crm/v3/lists", map[string]any{
		"name":           "My List",
		"objectTypeId":   "0-1",
		"processingType": "MANUAL",
	})
	resp.AssertStatus(201)
	listID := resp.JSONMap()["listId"].(string)

	resp = hsGet(tc, "/crm/v3/lists/"+listID)
	resp.AssertStatus(200)

	hsDelete(tc, "/crm/v3/lists/"+listID).AssertStatus(204)
}

// --- Webhooks ---

func TestWebhookCRUD(t *testing.T) {
	_, tc := setupHubSpot(t)

	resp := hsPost(tc, "/webhooks/v3/12345/subscriptions", map[string]any{
		"subscriptionType": "contact.creation",
		"active":           true,
	})
	resp.AssertStatus(201)
	subID := resp.JSONMap()["id"].(string)

	resp = hsGet(tc, "/webhooks/v3/12345/subscriptions")
	resp.AssertStatus(200)
	results := resp.JSONMap()["results"].([]any)
	if len(results) != 1 {
		t.Errorf("expected 1 subscription, got %d", len(results))
	}

	hsDelete(tc, "/webhooks/v3/12345/subscriptions/"+subID).AssertStatus(204)
}

// --- Forms ---

func TestFormCRUD(t *testing.T) {
	_, tc := setupHubSpot(t)

	resp := hsPost(tc, "/marketing/v3/forms", map[string]any{
		"name":     "Contact Form",
		"formType": "hubspot",
	})
	resp.AssertStatus(201)
	formID := resp.JSONMap()["id"].(string)

	resp = hsGet(tc, "/marketing/v3/forms")
	resp.AssertStatus(200)
	results := resp.JSONMap()["results"].([]any)
	if len(results) != 1 {
		t.Errorf("expected 1 form, got %d", len(results))
	}

	hsDelete(tc, "/marketing/v3/forms/"+formID).AssertStatus(204)
}

// --- Admin ---

func TestAdminHealth(t *testing.T) {
	_, tc := setupHubSpot(t)
	tc.Get("/admin/health").AssertStatus(200)
}

func TestAdminReset(t *testing.T) {
	_, tc := setupHubSpot(t)

	hsPost(tc, "/crm/v3/objects/contacts", map[string]any{
		"properties": map[string]any{"email": "reset@test.com"},
	})

	tc.Post("/admin/reset", nil).AssertStatus(200)

	resp := tc.Get("/admin/objects")
	if resp.JSONMap()["total"].(float64) != 0 {
		t.Error("expected 0 objects after reset")
	}
}
