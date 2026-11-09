package api_test

import (
	"reflect"
	"testing"
)

// TestUpdateDomain checks PATCH /domains/{id} answers {object, id} and the
// switches it names read back changed, the others unchanged.
func TestUpdateDomain(t *testing.T) {
	_, tc := setupResend(t)
	id := resendPost(tc, "/domains", map[string]any{"name": "example.com"}).AssertStatus(201).JSONMap()["id"].(string)

	m := resendPatch(tc, "/domains/"+id, map[string]any{"open_tracking": true, "tls": "enforced"}).AssertStatus(200).JSONMap()
	if !reflect.DeepEqual(m, map[string]any{"object": "domain", "id": id}) {
		t.Errorf("PATCH answered %v", m)
	}
	got := resendGet(tc, "/domains/"+id).AssertStatus(200).JSONMap()
	if got["open_tracking"] != true || got["click_tracking"] != false {
		t.Errorf("after open_tracking: got %v", got)
	}

	resendPatch(tc, "/domains/"+id, map[string]any{
		"open_tracking": false, "click_tracking": true, "capabilities": map[string]any{"receiving": "enabled"},
	}).AssertStatus(200)
	got = resendGet(tc, "/domains/"+id).AssertStatus(200).JSONMap()
	wantCaps := map[string]any{"sending": "enabled", "receiving": "enabled"}
	if got["open_tracking"] != false || got["click_tracking"] != true || !reflect.DeepEqual(got["capabilities"], wantCaps) {
		t.Errorf("after second PATCH: got %v", got)
	}

	m = resendPatch(tc, "/domains/00000000-0000-4000-8000-000000000000", map[string]any{"open_tracking": true}).AssertStatus(404).JSONMap()
	assertResendError(t, m, "not_found", "Domain not found")
}

// TestDeleteDomain checks DELETE answers {object, id, deleted: true} and the
// domain then reads 404 (probes m12, m13 of 2026-11-08).
func TestDeleteDomain(t *testing.T) {
	_, tc := setupResend(t)
	id := resendPost(tc, "/domains", map[string]any{"name": "example.com"}).AssertStatus(201).JSONMap()["id"].(string)

	m := tc.DoWithHeaders("DELETE", "/domains/"+id, nil, resendHeaders).AssertStatus(200).JSONMap()
	if !reflect.DeepEqual(m, map[string]any{"object": "domain", "id": id, "deleted": true}) {
		t.Errorf("DELETE answered %v", m)
	}
	m = resendGet(tc, "/domains/"+id).AssertStatus(404).JSONMap()
	assertResendError(t, m, "not_found", "Domain not found")
	tc.DoWithHeaders("DELETE", "/domains/"+id, nil, resendHeaders).AssertStatus(404)

	// The name is free again once deleted.
	resendPost(tc, "/domains", map[string]any{"name": "example.com"}).AssertStatus(201)
}

// TestVerifyDomain checks verify answers {object, id}, the domain reads
// verified by default (OD-4), and an unknown id answers 404.
func TestVerifyDomain(t *testing.T) {
	_, tc := setupResend(t)
	id := resendPost(tc, "/domains", map[string]any{"name": "example.com"}).AssertStatus(201).JSONMap()["id"].(string)

	m := resendPost(tc, "/domains/"+id+"/verify", nil).AssertStatus(200).JSONMap()
	if !reflect.DeepEqual(m, map[string]any{"object": "domain", "id": id}) {
		t.Errorf("verify answered %v", m)
	}
	if got := resendGet(tc, "/domains/"+id).AssertStatus(200).JSONMap(); got["status"] != "verified" {
		t.Errorf("status after verify %v, want verified", got["status"])
	}
	resendPost(tc, "/domains/00000000-0000-4000-8000-000000000000/verify", nil).AssertStatus(404)
}
