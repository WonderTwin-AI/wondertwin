package api_test

import (
	"net/url"
	"testing"
)

func usergroupIDs(t *testing.T, m map[string]any) []string {
	t.Helper()
	mustOK(t, 200, m)
	var ids []string
	for _, g := range m["usergroups"].([]any) {
		ids = append(ids, g.(map[string]any)["id"].(string))
	}
	return ids
}

// Slack marks every user group with is_usergroup: true; a disabled group is
// the one with a non-zero date_delete, and usergroups.list leaves it out
// unless include_disabled is set.
func TestUsergroupDisableAndList(t *testing.T) {
	srv, _ := setupSlack(t)
	created := formAs(t, srv, "xoxp-admin", "usergroups.create", url.Values{"name": {"Oncall"}, "handle": {"oncall"}})
	ug := created["usergroup"].(map[string]any)
	id := ug["id"].(string)
	if ug["is_usergroup"] != true || ug["date_delete"] != float64(0) || ug["created_by"] != "U_USER" {
		t.Errorf("a new group: %v", ug)
	}

	disabled := form(t, srv, "usergroups.disable", url.Values{"usergroup": {id}})["usergroup"].(map[string]any)
	if disabled["is_usergroup"] != true || disabled["date_delete"] == float64(0) {
		t.Errorf("a disabled group keeps is_usergroup and gets a date_delete: %v", disabled)
	}
	if ids := usergroupIDs(t, form(t, srv, "usergroups.list", nil)); len(ids) != 0 {
		t.Errorf("usergroups.list lists a disabled group: %v", ids)
	}
	if ids := usergroupIDs(t, form(t, srv, "usergroups.list", url.Values{"include_disabled": {"true"}})); len(ids) != 1 || ids[0] != id {
		t.Errorf("include_disabled: %v", ids)
	}

	enabled := form(t, srv, "usergroups.enable", url.Values{"usergroup": {id}})["usergroup"].(map[string]any)
	if enabled["date_delete"] != float64(0) {
		t.Errorf("an enabled group has no date_delete: %v", enabled)
	}
	if ids := usergroupIDs(t, form(t, srv, "usergroups.list", nil)); len(ids) != 1 {
		t.Errorf("an enabled group is listed again: %v", ids)
	}
}

func TestUsergroupsListEmptyIsAnArray(t *testing.T) {
	srv, _ := setupSlack(t)
	if m := form(t, srv, "usergroups.list", nil); m["usergroups"] == nil {
		t.Errorf("no groups answers [], not null: %v", m)
	}
}
