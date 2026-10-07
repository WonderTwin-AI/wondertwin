package api_test

import (
	"net/url"
	"testing"
)

// usergroups.create needs a name no other group has, and a handle no channel,
// user or group has; it answers the group with its team, prefs and updater.
func TestUsergroupsCreateRules(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "engineering")

	created := form(t, srv, "usergroups.create", url.Values{"name": {"Marketing"}, "handle": {"marketing"},
		"channels": {ch}, "include_count": {"true"}})
	mustOK(t, 200, created)
	ug := created["usergroup"].(map[string]any)
	prefs := ug["prefs"].(map[string]any)
	if ug["team_id"] != "T0001" || ug["updated_by"] != "U_BOT" || ug["user_count"] != float64(0) ||
		len(prefs["channels"].([]any)) != 1 || prefs["groups"] == nil {
		t.Errorf("created group: %v", ug)
	}

	wantErrors(t, srv, "usergroups.create", map[string]errCase{
		"no name":             {url.Values{"handle": {"x"}}, "missing_subteam_name"},
		"name taken":          {url.Values{"name": {"marketing"}}, "name_already_exists"},
		"handle of a group":   {url.Values{"name": {"Other"}, "handle": {"Marketing"}}, "handle_already_exists"},
		"handle of a channel": {url.Values{"name": {"Other"}, "handle": {"engineering"}}, "handle_already_exists"},
		"handle of a user":    {url.Values{"name": {"Other"}, "handle": {"bot"}}, "handle_already_exists"},
		"unknown channel":     {url.Values{"name": {"Other"}, "channels": {"CNOPE"}}, "invalid_channel_provided"},
	})
	if n := len(form(t, srv, "usergroups.list", nil)["usergroups"].([]any)); n != 1 {
		t.Errorf("refused creates were kept: %d groups", n)
	}

	status, m := call(t, srv, "POST", "/api/usergroups.create", jsonType, `{"name":"Ops","channels":["`+ch+`"]}`, true)
	mustOK(t, status, m)
	if got := m["usergroup"].(map[string]any)["prefs"].(map[string]any)["channels"].([]any); len(got) != 1 || got[0] != ch {
		t.Errorf("channels as a JSON array: %v", got)
	}
}
