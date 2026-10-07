package api_test

import (
	"net/url"
	"slices"
	"testing"
)

// usergroups.update changes only the fields given, and the users methods
// replace and list a group's members.
func TestUsergroupUpdateAndUsers(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, twoUsers)
	created := form(t, srv, "usergroups.create", url.Values{"name": {"Oncall"}, "handle": {"oncall"}})
	mustOK(t, 200, created)
	id := created["usergroup"].(map[string]any)["id"].(string)

	updated := form(t, srv, "usergroups.update", url.Values{"usergroup": {id}, "description": {"Pager rota"}})
	mustOK(t, 200, updated)
	ug := updated["usergroup"].(map[string]any)
	if ug["id"] != id || ug["description"] != "Pager rota" || ug["name"] != "Oncall" || ug["handle"] != "oncall" {
		t.Errorf("update answer: %v", ug)
	}

	mustOK(t, 200, form(t, srv, "usergroups.users.list", url.Values{"usergroup": {id}}))

	mustOK(t, 200, form(t, srv, "usergroups.users.update", url.Values{"usergroup": {id}, "users": {"U1,U2"}}))
	listed := anyStrings(form(t, srv, "usergroups.users.list", url.Values{"usergroup": {id}})["users"])
	slices.Sort(listed)
	if !slices.Equal(listed, []string{"U1", "U2"}) {
		t.Errorf("users after update: %v", listed)
	}
	mustOK(t, 200, form(t, srv, "usergroups.users.update", url.Values{"usergroup": {id}, "users": {"U3"}}))
	if listed := anyStrings(form(t, srv, "usergroups.users.list", url.Values{"usergroup": {id}})["users"]); !slices.Equal(listed, []string{"U3"}) {
		t.Errorf("users.update must replace the members: %v", listed)
	}

	for _, method := range []string{"usergroups.update", "usergroups.users.list", "usergroups.users.update"} {
		wantErrors(t, srv, method, map[string]errCase{
			"unknown group": {url.Values{"usergroup": {"S-nope"}, "users": {"U1"}}, "not_found"},
			"no group":      {url.Values{"users": {"U1"}}, "not_found"},
		})
	}
}

// usergroups.users.update replaces the group's users with users of the
// workspace: an empty list is no_users_provided (disable the group instead),
// and a user outside the workspace is failed_for_some_users. A refused update
// leaves the group as it was.
func TestUsergroupUsersUpdateChecksTheUsers(t *testing.T) {
	srv, _ := setupSlack(t)
	seedUsers(t, srv, twoUsers)
	id := form(t, srv, "usergroups.create", url.Values{"name": {"Team"}})["usergroup"].(map[string]any)["id"].(string)
	mustOK(t, 200, form(t, srv, "usergroups.users.update", url.Values{"usergroup": {id}, "users": {"U1"}}))

	wantErrors(t, srv, "usergroups.users.update", map[string]errCase{
		"no users":     {url.Values{"usergroup": {id}}, "no_users_provided"},
		"empty users":  {url.Values{"usergroup": {id}, "users": {""}}, "no_users_provided"},
		"unknown user": {url.Values{"usergroup": {id}, "users": {"U2,UFAKE"}}, "failed_for_some_users"},
	})
	if listed := anyStrings(form(t, srv, "usergroups.users.list", url.Values{"usergroup": {id}})["users"]); !slices.Equal(listed, []string{"U1"}) {
		t.Errorf("a refused update changed the group: %v", listed)
	}
}
