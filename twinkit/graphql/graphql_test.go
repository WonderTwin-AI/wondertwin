package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupSchema() *Schema {
	s := NewSchema()
	s.Query("issue", func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		if id == "not_found" {
			return nil, fmt.Errorf("issue not found")
		}
		return map[string]any{"id": id, "title": "Test Issue", "status": "open"}, nil
	})
	s.Query("issues", func(ctx context.Context, args map[string]any) (any, error) {
		limit := 10
		if l, ok := args["first"].(int); ok {
			limit = l
		}
		items := make([]map[string]any, 0, limit)
		for i := 0; i < limit; i++ {
			items = append(items, map[string]any{"id": fmt.Sprintf("iss_%d", i+1), "title": fmt.Sprintf("Issue %d", i+1)})
		}
		return items, nil
	})
	s.Mutation("createIssue", func(ctx context.Context, args map[string]any) (any, error) {
		input, _ := args["input"].(map[string]any)
		title := "Untitled"
		if input != nil {
			if t, ok := input["title"].(string); ok {
				title = t
			}
		}
		return map[string]any{"id": "new_1", "title": title, "status": "open"}, nil
	})
	return s
}

func doGraphQL(handler http.HandlerFunc, query string, variables map[string]any) *httptest.ResponseRecorder {
	body := map[string]any{"query": query}
	if variables != nil {
		body["variables"] = variables
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func parseGQLResponse(t *testing.T, w *httptest.ResponseRecorder) Response {
	t.Helper()
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v\nbody: %s", err, w.Body.String())
	}
	return resp
}

func TestQuery_Basic(t *testing.T) {
	handler := Handler(setupSchema())
	w := doGraphQL(handler, `query { issue(id: "iss_1") { id title } }`, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	resp := parseGQLResponse(t, w)
	if len(resp.Errors) > 0 {
		t.Fatalf("errors: %v", resp.Errors)
	}
	data := resp.Data.(map[string]any)
	issue := data["issue"].(map[string]any)
	if issue["title"] != "Test Issue" {
		t.Errorf("title = %v, want Test Issue", issue["title"])
	}
}

func TestQuery_WithVariables(t *testing.T) {
	handler := Handler(setupSchema())
	w := doGraphQL(handler, `query GetIssue($id: ID!) { issue(id: $id) { id title } }`,
		map[string]any{"id": "iss_42"})

	resp := parseGQLResponse(t, w)
	if len(resp.Errors) > 0 {
		t.Fatalf("errors: %v", resp.Errors)
	}
	data := resp.Data.(map[string]any)
	issue := data["issue"].(map[string]any)
	if issue["id"] != "iss_42" {
		t.Errorf("id = %v, want iss_42", issue["id"])
	}
}

func TestQuery_WithArgs(t *testing.T) {
	handler := Handler(setupSchema())
	w := doGraphQL(handler, `{ issues(first: 3) { id title } }`, nil)

	resp := parseGQLResponse(t, w)
	data := resp.Data.(map[string]any)
	issues := data["issues"].([]any)
	if len(issues) != 3 {
		t.Errorf("expected 3 issues, got %d", len(issues))
	}
}

func TestMutation(t *testing.T) {
	handler := Handler(setupSchema())
	w := doGraphQL(handler, `mutation { createIssue(input: {title: "New Bug"}) { id title } }`, nil)

	resp := parseGQLResponse(t, w)
	if len(resp.Errors) > 0 {
		t.Fatalf("errors: %v", resp.Errors)
	}
	data := resp.Data.(map[string]any)
	issue := data["createIssue"].(map[string]any)
	if issue["title"] != "New Bug" {
		t.Errorf("title = %v, want New Bug", issue["title"])
	}
}

func TestMutation_WithVariables(t *testing.T) {
	handler := Handler(setupSchema())
	w := doGraphQL(handler, `mutation CreateIssue($input: IssueInput!) { createIssue(input: $input) { id title } }`,
		map[string]any{"input": map[string]any{"title": "Variable Bug"}})

	resp := parseGQLResponse(t, w)
	data := resp.Data.(map[string]any)
	issue := data["createIssue"].(map[string]any)
	if issue["title"] != "Variable Bug" {
		t.Errorf("title = %v, want Variable Bug", issue["title"])
	}
}

func TestQuery_ResolverError(t *testing.T) {
	handler := Handler(setupSchema())
	w := doGraphQL(handler, `{ issue(id: "not_found") { id } }`, nil)

	resp := parseGQLResponse(t, w)
	if len(resp.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(resp.Errors))
	}
	if resp.Errors[0].Message != "issue not found" {
		t.Errorf("error = %q", resp.Errors[0].Message)
	}
}

func TestQuery_FieldNotFound(t *testing.T) {
	handler := Handler(setupSchema())
	w := doGraphQL(handler, `{ nonexistent { id } }`, nil)

	resp := parseGQLResponse(t, w)
	if len(resp.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(resp.Errors))
	}
}

func TestIntrospection(t *testing.T) {
	handler := Handler(setupSchema())
	w := doGraphQL(handler, `{ __schema { queryType { name } } }`, nil)

	resp := parseGQLResponse(t, w)
	if len(resp.Errors) > 0 {
		t.Fatalf("errors: %v", resp.Errors)
	}
	if resp.Data == nil {
		t.Fatal("expected data")
	}
}

func TestEmptyQuery(t *testing.T) {
	handler := Handler(setupSchema())
	w := doGraphQL(handler, "", nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	handler := Handler(setupSchema())
	req := httptest.NewRequest("GET", "/graphql", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}
