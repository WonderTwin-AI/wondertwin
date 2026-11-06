package api_test

import (
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
