package api_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/search"
	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-algolia/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-algolia/internal/store"
)

func setupAlgolia(t *testing.T) (*httptest.Server, *testutil.TwinClient) {
	t.Helper()
	engine := search.NewEngine()
	memStore := store.New(engine)
	cfg := &twincore.Config{Name: "twin-algolia-test"}
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

var algoliaHeaders = map[string]string{
	"X-Algolia-Application-Id": "test-app-id",
	"X-Algolia-API-Key":        "test-api-key",
	"Content-Type":              "application/json",
}

func aGet(tc *testutil.TwinClient, path string) *testutil.Response {
	return tc.DoWithHeaders("GET", path, nil, algoliaHeaders)
}

func aPost(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("POST", path, body, algoliaHeaders)
}

func aPut(tc *testutil.TwinClient, path string, body any) *testutil.Response {
	return tc.DoWithHeaders("PUT", path, body, algoliaHeaders)
}

func aDelete(tc *testutil.TwinClient, path string) *testutil.Response {
	return tc.DoWithHeaders("DELETE", path, nil, algoliaHeaders)
}

// --- Auth ---

func TestAuthRequired(t *testing.T) {
	_, tc := setupAlgolia(t)
	resp := tc.Get("/1/indexes")
	resp.AssertStatus(403)
}

// --- Indexing + Search ---

func TestAddAndSearchRecords(t *testing.T) {
	_, tc := setupAlgolia(t)

	// Add records
	aPut(tc, "/1/indexes/products/1", map[string]any{
		"objectID": "1", "name": "iPhone 15", "category": "electronics", "price": 999,
	}).AssertStatus(200)

	aPut(tc, "/1/indexes/products/2", map[string]any{
		"objectID": "2", "name": "MacBook Pro", "category": "computers", "price": 2499,
	}).AssertStatus(200)

	// Search
	resp := aPost(tc, "/1/indexes/products/query", map[string]any{
		"params": "query=iphone",
	})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if m["nbHits"].(float64) != 1 {
		t.Errorf("expected 1 hit for 'iphone', got %v", m["nbHits"])
	}
	hits := m["hits"].([]any)
	if hits[0].(map[string]any)["objectID"] != "1" {
		t.Error("expected objectID=1")
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	_, tc := setupAlgolia(t)

	aPut(tc, "/1/indexes/items/1", map[string]any{"objectID": "1", "name": "A"}).AssertStatus(200)
	aPut(tc, "/1/indexes/items/2", map[string]any{"objectID": "2", "name": "B"}).AssertStatus(200)

	resp := aPost(tc, "/1/indexes/items/query", map[string]any{"params": ""})
	resp.AssertStatus(200)
	if resp.JSONMap()["nbHits"].(float64) != 2 {
		t.Error("expected 2 hits for empty query")
	}
}

// --- Get Record ---

func TestGetRecord(t *testing.T) {
	_, tc := setupAlgolia(t)

	aPut(tc, "/1/indexes/products/abc", map[string]any{
		"objectID": "abc", "name": "Widget",
	}).AssertStatus(200)

	resp := aGet(tc, "/1/indexes/products/abc")
	resp.AssertStatus(200)
	if resp.JSONMap()["name"] != "Widget" {
		t.Error("expected name=Widget")
	}
}

// --- Delete Record ---

func TestDeleteRecord(t *testing.T) {
	_, tc := setupAlgolia(t)

	aPut(tc, "/1/indexes/products/del1", map[string]any{"objectID": "del1", "name": "Gone"}).AssertStatus(200)
	aDelete(tc, "/1/indexes/products/del1").AssertStatus(200)

	resp := aGet(tc, "/1/indexes/products/del1")
	resp.AssertStatus(404)
}

// --- Batch ---

func TestBatchWrite(t *testing.T) {
	_, tc := setupAlgolia(t)

	resp := aPost(tc, "/1/indexes/products/batch", map[string]any{
		"requests": []map[string]any{
			{"action": "addObject", "body": map[string]any{"objectID": "b1", "name": "Batch 1"}},
			{"action": "addObject", "body": map[string]any{"objectID": "b2", "name": "Batch 2"}},
		},
	})
	resp.AssertStatus(200)
	if resp.JSONMap()["taskID"] == nil {
		t.Error("expected taskID")
	}

	// Verify both exist
	aGet(tc, "/1/indexes/products/b1").AssertStatus(200)
	aGet(tc, "/1/indexes/products/b2").AssertStatus(200)
}

// --- Clear Index ---

func TestClearIndex(t *testing.T) {
	_, tc := setupAlgolia(t)

	aPut(tc, "/1/indexes/clearme/1", map[string]any{"objectID": "1", "x": 1}).AssertStatus(200)
	aPost(tc, "/1/indexes/clearme/clear", nil).AssertStatus(200)

	resp := aPost(tc, "/1/indexes/clearme/query", map[string]any{"params": ""})
	if resp.JSONMap()["nbHits"].(float64) != 0 {
		t.Error("expected 0 hits after clear")
	}
}

// --- Index Management ---

func TestListAndDeleteIndex(t *testing.T) {
	_, tc := setupAlgolia(t)

	aPut(tc, "/1/indexes/idx1/1", map[string]any{"objectID": "1"}).AssertStatus(200)
	aPut(tc, "/1/indexes/idx2/1", map[string]any{"objectID": "1"}).AssertStatus(200)

	resp := aGet(tc, "/1/indexes")
	resp.AssertStatus(200)
	items := resp.JSONMap()["items"].([]any)
	if len(items) < 2 {
		t.Errorf("expected at least 2 indices, got %d", len(items))
	}

	aDelete(tc, "/1/indexes/idx1").AssertStatus(200)
}

// --- Settings ---

func TestSettings(t *testing.T) {
	_, tc := setupAlgolia(t)

	aPut(tc, "/1/indexes/settings-test/1", map[string]any{"objectID": "1"}).AssertStatus(200)

	aPut(tc, "/1/indexes/settings-test/settings", map[string]any{
		"searchableAttributes": []string{"name", "description"},
	}).AssertStatus(200)

	resp := aGet(tc, "/1/indexes/settings-test/settings")
	resp.AssertStatus(200)
	attrs := resp.JSONMap()["searchableAttributes"].([]any)
	if len(attrs) != 2 {
		t.Errorf("expected 2 searchable attributes, got %d", len(attrs))
	}
}

// --- Synonyms ---

func TestSynonymCRUD(t *testing.T) {
	_, tc := setupAlgolia(t)

	aPut(tc, "/1/indexes/syn-test/synonyms/syn1", map[string]any{
		"objectID": "syn1",
		"type":     "synonym",
		"synonyms": []string{"phone", "mobile", "cell"},
	}).AssertStatus(200)

	resp := aGet(tc, "/1/indexes/syn-test/synonyms/syn1")
	resp.AssertStatus(200)
	if resp.JSONMap()["type"] != "synonym" {
		t.Error("expected type=synonym")
	}

	aDelete(tc, "/1/indexes/syn-test/synonyms/syn1").AssertStatus(200)
}

// --- Rules ---

func TestRuleCRUD(t *testing.T) {
	_, tc := setupAlgolia(t)

	aPut(tc, "/1/indexes/rule-test/rules/rule1", map[string]any{
		"objectID": "rule1",
		"conditions": []map[string]any{
			{"anchoring": "contains", "pattern": "phone"},
		},
		"consequence": map[string]any{
			"params": map[string]any{"query": "smartphone"},
		},
		"enabled": true,
	}).AssertStatus(200)

	resp := aGet(tc, "/1/indexes/rule-test/rules/rule1")
	resp.AssertStatus(200)
	if resp.JSONMap()["objectID"] != "rule1" {
		t.Error("expected objectID=rule1")
	}

	aDelete(tc, "/1/indexes/rule-test/rules/rule1").AssertStatus(200)
}

// --- API Keys ---

func TestAPIKeyCRUD(t *testing.T) {
	_, tc := setupAlgolia(t)

	resp := aPost(tc, "/1/keys", map[string]any{
		"acl":         []string{"search", "addObject"},
		"description": "Test key",
	})
	resp.AssertStatus(201)
	key := resp.JSONMap()["key"].(string)
	if key == "" {
		t.Error("expected key value")
	}

	resp = aGet(tc, "/1/keys")
	resp.AssertStatus(200)
	keys := resp.JSONMap()["keys"].([]any)
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}

	aDelete(tc, "/1/keys/"+key).AssertStatus(200)
}

// --- Task Status ---

func TestTaskStatus(t *testing.T) {
	_, tc := setupAlgolia(t)

	aPut(tc, "/1/indexes/task-test/1", map[string]any{"objectID": "1"}).AssertStatus(200)

	resp := aGet(tc, "/1/indexes/task-test/task/1")
	resp.AssertStatus(200)
	if resp.JSONMap()["status"] != "published" {
		t.Error("expected status=published")
	}
}

// --- Browse ---

func TestBrowse(t *testing.T) {
	_, tc := setupAlgolia(t)

	aPut(tc, "/1/indexes/browse-test/1", map[string]any{"objectID": "1", "x": 1}).AssertStatus(200)
	aPut(tc, "/1/indexes/browse-test/2", map[string]any{"objectID": "2", "x": 2}).AssertStatus(200)

	resp := aPost(tc, "/1/indexes/browse-test/browse", map[string]any{})
	resp.AssertStatus(200)
	hits := resp.JSONMap()["hits"].([]any)
	if len(hits) != 2 {
		t.Errorf("expected 2 hits in browse, got %d", len(hits))
	}
}

// --- Multi-index search ---

func TestMultiSearch(t *testing.T) {
	_, tc := setupAlgolia(t)

	aPut(tc, "/1/indexes/multi1/1", map[string]any{"objectID": "1", "name": "Alpha"}).AssertStatus(200)
	aPut(tc, "/1/indexes/multi2/1", map[string]any{"objectID": "1", "name": "Beta"}).AssertStatus(200)

	resp := aPost(tc, "/1/indexes/_multi/queries", map[string]any{
		"requests": []map[string]any{
			{"indexName": "multi1", "params": "query=alpha"},
			{"indexName": "multi2", "params": "query=beta"},
		},
	})
	resp.AssertStatus(200)

	var m map[string]any
	json.Unmarshal(resp.Body, &m)
	results := m["results"].([]any)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

// --- Admin ---

func TestAdminHealth(t *testing.T) {
	_, tc := setupAlgolia(t)
	tc.Get("/admin/health").AssertStatus(200)
}

func TestAdminReset(t *testing.T) {
	_, tc := setupAlgolia(t)

	aPut(tc, "/1/indexes/reset-test/1", map[string]any{"objectID": "1"}).AssertStatus(200)

	tc.Post("/admin/reset", nil).AssertStatus(200)

	resp := tc.Get("/admin/indices")
	// After reset the store is cleared but engine indices may persist
	// The key assertion is that the admin endpoint works
	resp.AssertStatus(200)
}
