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
		"domain":  resendPost(tc, "/domains", map[string]any{"name": "acme.dev"}).AssertStatus(201).JSONMap(),
		"contact": resendPost(tc, "/audiences/"+audienceID+"/contacts", map[string]any{"email": "c@acme.dev"}).AssertStatus(201).JSONMap(),
		"broadcast": resendPost(tc, "/broadcasts", map[string]any{
			"audience_id": audienceID, "from": "a@acme.dev", "subject": "s",
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

// listIDs answers the ids in a list response, in order, and its has_more.
func listIDs(t *testing.T, m map[string]any) ([]string, bool) {
	t.Helper()
	var ids []string
	for _, d := range m["data"].([]any) {
		ids = append(ids, d.(map[string]any)["id"].(string))
	}
	return ids, m["has_more"].(bool)
}

func assertPage(t *testing.T, m map[string]any, wantIDs []string, wantMore bool) {
	t.Helper()
	ids, more := listIDs(t, m)
	if strings.Join(ids, ",") != strings.Join(wantIDs, ",") || more != wantMore {
		t.Errorf("got ids %v has_more %v, want %v has_more %v", ids, more, wantIDs, wantMore)
	}
}

// TestListDomainsPagination checks limit, after and before on GET /domains:
// newest first, every domain without a limit, the cursor excluded (quirks
// pagination-001, pagination-002).
func TestListDomainsPagination(t *testing.T) {
	_, tc := setupResend(t)
	var ids []string
	for _, name := range []string{"one.acme.dev", "two.acme.dev", "three.acme.dev"} {
		ids = append(ids, resendPost(tc, "/domains", map[string]any{"name": name}).AssertStatus(201).JSONMap()["id"].(string))
	}
	d1, d2, d3 := ids[0], ids[1], ids[2]

	assertPage(t, resendGet(tc, "/domains").AssertStatus(200).JSONMap(), []string{d3, d2, d1}, false)
	assertPage(t, resendGet(tc, "/domains?limit=2").AssertStatus(200).JSONMap(), []string{d3, d2}, true)
	assertPage(t, resendGet(tc, "/domains?limit=2&after="+d2).AssertStatus(200).JSONMap(), []string{d1}, false)
	assertPage(t, resendGet(tc, "/domains?after="+d3).AssertStatus(200).JSONMap(), []string{d2, d1}, false)
	assertPage(t, resendGet(tc, "/domains?limit=1&before="+d1).AssertStatus(200).JSONMap(), []string{d2}, true)
	assertPage(t, resendGet(tc, "/domains?limit=5&before="+d1).AssertStatus(200).JSONMap(), []string{d3, d2}, false)
	assertPage(t, resendGet(tc, "/domains?after=00000000-0000-4000-8000-000000000000").AssertStatus(200).JSONMap(), nil, false)
}

// TestListAPIKeysPagination checks limit and after on GET /api-keys, newest
// first, as recorded on 2026-11-08 (n03, n04).
func TestListAPIKeysPagination(t *testing.T) {
	_, tc := setupResend(t)
	k1 := resendPost(tc, "/api-keys", map[string]any{"name": "first"}).AssertStatus(201).JSONMap()["id"].(string)
	k2 := resendPost(tc, "/api-keys", map[string]any{"name": "second"}).AssertStatus(201).JSONMap()["id"].(string)

	assertPage(t, resendGet(tc, "/api-keys").AssertStatus(200).JSONMap(), []string{k2, k1}, false)
	page := resendGet(tc, "/api-keys?limit=1").AssertStatus(200).JSONMap()
	assertPage(t, page, []string{k2}, true)
	if _, ok := page["data"].([]any)[0].(map[string]any)["token"]; ok {
		t.Error("a listed key carries its token")
	}
	assertPage(t, resendGet(tc, "/api-keys?limit=1&after="+k2).AssertStatus(200).JSONMap(), []string{k1}, false)
}

// TestPaginationErrors checks the 422 answers for a bad limit and for both
// cursors at once.
func TestPaginationErrors(t *testing.T) {
	_, tc := setupResend(t)
	for _, path := range []string{"/domains?limit=0", "/domains?limit=101", "/api-keys?limit=abc", "/api-keys?limit=-1"} {
		m := resendGet(tc, path).AssertStatus(422).JSONMap()
		if m["name"] != "validation_error" || !strings.HasPrefix(m["message"].(string), "The pagination limit must be a number between 1 and 100.") {
			t.Errorf("GET %s: got %v", path, m)
		}
	}
	m := resendGet(tc, "/domains?after=a&before=b").AssertStatus(422).JSONMap()
	if m["name"] != "validation_error" {
		t.Errorf("both cursors: got %v", m)
	}
}
