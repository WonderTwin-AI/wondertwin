// Package graphql provides a lightweight GraphQL handler for WonderTwin twins.
// It supports query and mutation operations with variable substitution,
// but does not implement the full GraphQL specification. Designed for
// simulating GraphQL APIs like Linear, GitHub v4, and Shopify Storefront.
package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// ResolverFunc handles a single query or mutation field.
// args contains the parsed arguments from the query.
// Returns the data to include in the response.
type ResolverFunc func(ctx context.Context, args map[string]any) (any, error)

// Schema defines the available queries and mutations.
type Schema struct {
	mu        sync.RWMutex
	queries   map[string]ResolverFunc
	mutations map[string]ResolverFunc
}

// NewSchema creates a new empty schema.
func NewSchema() *Schema {
	return &Schema{
		queries:   make(map[string]ResolverFunc),
		mutations: make(map[string]ResolverFunc),
	}
}

// Query registers a query resolver.
func (s *Schema) Query(name string, resolver ResolverFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queries[name] = resolver
}

// Mutation registers a mutation resolver.
func (s *Schema) Mutation(name string, resolver ResolverFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mutations[name] = resolver
}

// Request is the incoming GraphQL request body.
type Request struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operationName,omitempty"`
	Variables     map[string]any `json:"variables,omitempty"`
}

// Response is the GraphQL response envelope.
type Response struct {
	Data   any        `json:"data,omitempty"`
	Errors []GQLError `json:"errors,omitempty"`
}

// GQLError is a single GraphQL error.
type GQLError struct {
	Message    string         `json:"message"`
	Path       []string       `json:"path,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

// Handler returns an http.HandlerFunc that processes GraphQL requests.
func Handler(schema *Schema) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeResponse(w, http.StatusMethodNotAllowed, Response{
				Errors: []GQLError{{Message: "POST required"}},
			})
			return
		}

		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeResponse(w, http.StatusBadRequest, Response{
				Errors: []GQLError{{Message: "Invalid JSON: " + err.Error()}},
			})
			return
		}

		if req.Query == "" {
			writeResponse(w, http.StatusBadRequest, Response{
				Errors: []GQLError{{Message: "query is required"}},
			})
			return
		}

		// Handle introspection.
		if strings.Contains(req.Query, "__schema") || strings.Contains(req.Query, "__type") {
			writeResponse(w, http.StatusOK, Response{
				Data: schema.introspect(),
			})
			return
		}

		// Parse and execute.
		result, errors := schema.execute(r.Context(), req)
		resp := Response{Data: result}
		if len(errors) > 0 {
			resp.Errors = errors
		}

		writeResponse(w, http.StatusOK, resp)
	}
}

// execute parses the query string and dispatches to resolvers.
func (s *Schema) execute(ctx context.Context, req Request) (any, []GQLError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := strings.TrimSpace(req.Query)

	// Determine operation type.
	isMutation := strings.HasPrefix(query, "mutation")

	// Extract the field calls from the query body.
	// This is a simplified parser — it handles:
	//   query { field(arg: value) { subfields } }
	//   mutation { field(input: $var) { subfields } }
	//   query Name($var: Type!) { field(arg: $var) { subfields } }
	fields := parseFields(query, req.Variables)

	data := make(map[string]any)
	var errors []GQLError

	for _, field := range fields {
		var resolver ResolverFunc
		var ok bool

		if isMutation {
			resolver, ok = s.mutations[field.Name]
		} else {
			resolver, ok = s.queries[field.Name]
		}

		if !ok {
			errors = append(errors, GQLError{
				Message: fmt.Sprintf("field %q not found", field.Name),
				Path:    []string{field.Name},
			})
			continue
		}

		result, err := resolver(ctx, field.Args)
		if err != nil {
			errors = append(errors, GQLError{
				Message: err.Error(),
				Path:    []string{field.Name},
			})
			continue
		}

		data[field.Name] = result
	}

	return data, errors
}

// introspect returns a minimal introspection response.
func (s *Schema) introspect() map[string]any {
	var queryFields []map[string]any
	for name := range s.queries {
		queryFields = append(queryFields, map[string]any{
			"name": name,
			"type": map[string]any{"name": "JSON", "kind": "SCALAR"},
		})
	}
	var mutationFields []map[string]any
	for name := range s.mutations {
		mutationFields = append(mutationFields, map[string]any{
			"name": name,
			"type": map[string]any{"name": "JSON", "kind": "SCALAR"},
		})
	}

	types := []map[string]any{
		{"name": "Query", "kind": "OBJECT", "fields": queryFields},
	}
	if len(mutationFields) > 0 {
		types = append(types, map[string]any{
			"name": "Mutation", "kind": "OBJECT", "fields": mutationFields,
		})
	}

	return map[string]any{
		"__schema": map[string]any{
			"queryType":    map[string]any{"name": "Query"},
			"mutationType": map[string]any{"name": "Mutation"},
			"types":        types,
		},
	}
}

// field represents a parsed field call.
type field struct {
	Name string
	Args map[string]any
}

// parseFields extracts top-level field calls from a GraphQL query string.
// This is intentionally simple — it handles the common patterns used by SDKs.
func parseFields(query string, variables map[string]any) []field {
	// Strip operation keyword and name: "query Name($v: T!) { ... }" → "{ ... }"
	body := query
	for _, prefix := range []string{"query", "mutation", "subscription"} {
		if strings.HasPrefix(body, prefix) {
			body = body[len(prefix):]
			break
		}
	}

	// Skip operation name and variable declarations.
	// Find the first '{'.
	braceStart := strings.Index(body, "{")
	if braceStart == -1 {
		return nil
	}
	body = body[braceStart+1:]

	// Find the matching closing '}'.
	depth := 1
	braceEnd := -1
	for i, ch := range body {
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				braceEnd = i
				break
			}
		}
	}
	if braceEnd > 0 {
		body = body[:braceEnd]
	}

	// Extract field names and arguments at depth 0.
	var fields []field
	body = strings.TrimSpace(body)

	i := 0
	for i < len(body) {
		// Skip whitespace.
		for i < len(body) && (body[i] == ' ' || body[i] == '\n' || body[i] == '\r' || body[i] == '\t' || body[i] == ',') {
			i++
		}
		if i >= len(body) {
			break
		}

		// Read field name.
		nameStart := i
		for i < len(body) && body[i] != '(' && body[i] != '{' && body[i] != ' ' && body[i] != '\n' && body[i] != ':' {
			i++
		}
		name := strings.TrimSpace(body[nameStart:i])
		if name == "" {
			break
		}

		// Handle alias: "alias: fieldName"
		for i < len(body) && (body[i] == ' ' || body[i] == '\t') {
			i++
		}
		if i < len(body) && body[i] == ':' {
			i++ // skip ':'
			for i < len(body) && (body[i] == ' ' || body[i] == '\t') {
				i++
			}
			nameStart = i
			for i < len(body) && body[i] != '(' && body[i] != '{' && body[i] != ' ' && body[i] != '\n' {
				i++
			}
			name = strings.TrimSpace(body[nameStart:i])
		}

		args := make(map[string]any)

		// Parse arguments if present.
		for i < len(body) && (body[i] == ' ' || body[i] == '\t') {
			i++
		}
		if i < len(body) && body[i] == '(' {
			argEnd := findMatchingParen(body, i)
			if argEnd > i {
				argStr := body[i+1 : argEnd]
				args = parseArgs(argStr, variables)
				i = argEnd + 1
			}
		}

		// Skip selection set (sub-fields).
		for i < len(body) && (body[i] == ' ' || body[i] == '\t' || body[i] == '\n') {
			i++
		}
		if i < len(body) && body[i] == '{' {
			end := findMatchingBrace(body, i)
			if end > i {
				i = end + 1
			}
		}

		fields = append(fields, field{Name: name, Args: args})
	}

	return fields
}

// parseArgs parses a GraphQL argument string like: id: "123", limit: 10, input: $var
func parseArgs(s string, variables map[string]any) map[string]any {
	args := make(map[string]any)
	s = strings.TrimSpace(s)
	if s == "" {
		return args
	}

	// Simple key: value parser.
	parts := splitArgs(s)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		colonIdx := strings.Index(part, ":")
		if colonIdx < 0 {
			continue
		}
		key := strings.TrimSpace(part[:colonIdx])
		val := strings.TrimSpace(part[colonIdx+1:])

		args[key] = resolveValue(val, variables)
	}

	return args
}

// splitArgs splits argument pairs, respecting nested braces and strings.
func splitArgs(s string) []string {
	var parts []string
	depth := 0
	inString := false
	start := 0

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' && (i == 0 || s[i-1] != '\\') {
			inString = !inString
		}
		if inString {
			continue
		}
		if ch == '{' || ch == '[' || ch == '(' {
			depth++
		} else if ch == '}' || ch == ']' || ch == ')' {
			depth--
		} else if ch == ',' && depth == 0 {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

// resolveValue converts a GraphQL value string to a Go value.
func resolveValue(s string, variables map[string]any) any {
	s = strings.TrimSpace(s)

	// Variable reference.
	if strings.HasPrefix(s, "$") {
		varName := s[1:]
		if val, ok := variables[varName]; ok {
			return val
		}
		return nil
	}

	// String literal.
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		return s[1 : len(s)-1]
	}

	// Boolean.
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}

	// Null.
	if s == "null" {
		return nil
	}

	// Object literal: {key: value, ...}
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		return parseArgs(inner, variables)
	}

	// Number.
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
		// Return int if no decimal.
		if f == float64(int(f)) && !strings.Contains(s, ".") {
			return int(f)
		}
		return f
	}

	// Enum or unquoted string.
	return s
}

func findMatchingParen(s string, start int) int {
	depth := 0
	for i := start; i < len(s); i++ {
		if s[i] == '(' {
			depth++
		} else if s[i] == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(s) - 1
}

func findMatchingBrace(s string, start int) int {
	depth := 0
	for i := start; i < len(s); i++ {
		if s[i] == '{' {
			depth++
		} else if s[i] == '}' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(s) - 1
}

func writeResponse(w http.ResponseWriter, status int, resp Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}
