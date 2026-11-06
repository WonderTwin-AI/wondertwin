package api_test

import (
	"regexp"
	"strings"
	"testing"
)

// TestEmptyListsAreArrays checks every older list endpoint answers Resend's
// envelope with data [] on a fresh emulator, never null.
func TestEmptyListsAreArrays(t *testing.T) {
	_, tc := setupResend(t)
	audienceID := resendPost(tc, "/audiences", map[string]any{"name": "General"}).
		AssertStatus(201).JSONMap()["id"].(string)

	for _, path := range []string{"/domains", "/api-keys", "/broadcasts", "/audiences/" + audienceID + "/contacts"} {
		resp := resendGet(tc, path).AssertStatus(200)
		if !strings.Contains(string(resp.Body), `"data":[]`) {
			t.Errorf("GET %s: want data [], got %s", path, resp.Body)
		}
		m := resp.JSONMap()
		if m["object"] != "list" || m["has_more"] != false {
			t.Errorf("GET %s: want object list and has_more false, got %v", path, m)
		}
	}
}

// TestCreatedAtPostgresForm checks every resource reports created_at in the
// Postgres form Resend reads back (quirk envelope-003).
func TestCreatedAtPostgresForm(t *testing.T) {
	_, tc := setupResend(t)
	pg := regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{6}\+00$`)
	audienceID := resendPost(tc, "/audiences", map[string]any{"name": "General"}).AssertStatus(201).JSONMap()["id"].(string)
	created := map[string]map[string]any{
		"domain":  resendPost(tc, "/domains", map[string]any{"name": "example.com"}).AssertStatus(201).JSONMap(),
		"contact": resendPost(tc, "/audiences/"+audienceID+"/contacts", map[string]any{"email": "c@example.com"}).AssertStatus(201).JSONMap(),
		"broadcast": resendPost(tc, "/broadcasts", map[string]any{
			"audience_id": audienceID, "from": "a@example.com", "subject": "s",
		}).AssertStatus(201).JSONMap(),
		"audience": resendGet(tc, "/audiences/"+audienceID).AssertStatus(200).JSONMap(),
	}
	resendPost(tc, "/api-keys", map[string]any{"name": "ci"}).AssertStatus(201)
	created["api_key"] = resendGet(tc, "/api-keys").JSONMap()["data"].([]any)[0].(map[string]any)
	for kind, m := range created {
		if s, _ := m["created_at"].(string); !pg.MatchString(s) {
			t.Errorf("%s created_at %q, want the Postgres form", kind, s)
		}
	}
}
