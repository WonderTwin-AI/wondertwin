package api_test

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"
)

const homeView = `{"type":"home","callback_id":"home_v1","blocks":[{"type":"section","text":{"type":"mrkdwn","text":"Welcome"}}]}`

// publishForm publishes a Home view the way the node SDK sends it: a form
// body, with the view as JSON text in a field.
func publishForm(t *testing.T, srv *httptest.Server, form url.Values) map[string]any {
	t.Helper()
	_, m := call(t, srv, "POST", "/api/views.publish", formType, form.Encode(), true)
	return m
}

func viewOf(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	if m["ok"] != true {
		t.Fatalf("want ok:true, got %v", m)
	}
	v, ok := m["view"].(map[string]any)
	if !ok {
		t.Fatalf("no view in %v", m)
	}
	return v
}

func TestViewsPublishFormAndJSON(t *testing.T) {
	srv, _ := setupSlack(t)

	form := viewOf(t, publishForm(t, srv, url.Values{"user_id": {"U_USER"}, "view": {homeView}}))
	if form["type"] != "home" || form["id"] == "" || form["root_view_id"] != form["id"] {
		t.Errorf("form publish: %v", form)
	}
	if form["team_id"] != "T0001" || form["bot_id"] != "B_BOT" || form["app_id"] == "" {
		t.Errorf("form publish identity fields: %v", form)
	}
	if form["close"] != nil || form["submit"] != nil {
		t.Errorf("a Home view has null close and submit: %v", form)
	}
	if _, has := form["close"]; !has {
		t.Errorf("close is rendered as null, not left out: %v", form)
	}
	blocks := form["blocks"].([]any)
	if id, _ := blocks[0].(map[string]any)["block_id"].(string); len(blocks) != 1 || len(id) != 5 {
		t.Errorf("blocks: %v", blocks)
	}

	// The python SDK sends a JSON body with the view as an object.
	status, m := call(t, srv, "POST", "/api/views.publish", jsonType,
		`{"user_id":"U_BOT","view":`+homeView+`}`, true)
	mustOK(t, status, m)
	if viewOf(t, m)["callback_id"] != "home_v1" {
		t.Errorf("json publish: %v", m)
	}
}

func TestViewsPublishReplacesTheUsersHomeView(t *testing.T) {
	srv, _ := setupSlack(t)
	first := viewOf(t, publishForm(t, srv, url.Values{"user_id": {"U_USER"}, "view": {homeView}}))
	second := viewOf(t, publishForm(t, srv, url.Values{"user_id": {"U_USER"},
		"view": {`{"type":"home","callback_id":"home_v2","blocks":[]}`}}))
	if second["id"] != first["id"] {
		t.Errorf("republish changed the view id: %v then %v", first["id"], second["id"])
	}
	if second["callback_id"] != "home_v2" || second["hash"] == first["hash"] {
		t.Errorf("republish did not replace the view: %v", second)
	}
	other := viewOf(t, publishForm(t, srv, url.Values{"user_id": {"U_BOT"}, "view": {homeView}}))
	if other["id"] == first["id"] {
		t.Error("two users share one Home view")
	}
}

func TestViewsPublishErrors(t *testing.T) {
	srv, _ := setupSlack(t)
	for name, form := range map[string]url.Values{
		"no user":      {"view": {homeView}},
		"unknown user": {"user_id": {"U_NOBODY"}, "view": {homeView}},
		"no view":      {"user_id": {"U_USER"}},
		"bad view":     {"user_id": {"U_USER"}, "view": {"not json"}},
		"modal view":   {"user_id": {"U_USER"}, "view": {`{"type":"modal","blocks":[]}`}},
	} {
		m := publishForm(t, srv, form)
		if m["ok"] != false || m["error"] != "invalid_arguments" {
			t.Errorf("%s: want invalid_arguments, got %v", name, m)
		}
	}
	m := publishForm(t, srv, url.Values{"view": {homeView}})
	msgs, _ := m["response_metadata"].(map[string]any)["messages"].([]any)
	if len(msgs) != 1 || msgs[0] != "invalid `user_id`" {
		t.Errorf("response_metadata.messages: %v", m)
	}
}

func TestViewsPublishHashConflict(t *testing.T) {
	srv, _ := setupSlack(t)
	v := viewOf(t, publishForm(t, srv, url.Values{"user_id": {"U_USER"}, "view": {homeView}}))
	stale := v["hash"].(string)
	viewOf(t, publishForm(t, srv, url.Values{"user_id": {"U_USER"}, "view": {homeView}, "hash": {stale}}))

	m := publishForm(t, srv, url.Values{"user_id": {"U_USER"}, "view": {homeView}, "hash": {stale}})
	if m["error"] != "hash_conflict" {
		t.Errorf("a stale hash: want hash_conflict, got %v", m)
	}
}

func TestViewsExternalIDIsUniqueAcrossViews(t *testing.T) {
	srv, _ := setupSlack(t)
	withExt := `{"type":"home","external_id":"ext-1","blocks":[]}`
	viewOf(t, publishForm(t, srv, url.Values{"user_id": {"U_USER"}, "view": {withExt}}))
	// The same user may republish with its own external_id.
	viewOf(t, publishForm(t, srv, url.Values{"user_id": {"U_USER"}, "view": {withExt}}))

	m := publishForm(t, srv, url.Values{"user_id": {"U_BOT"}, "view": {withExt}})
	if m["error"] != "duplicate_external_id" {
		t.Errorf("want duplicate_external_id, got %v", m)
	}
}

func TestViewsUpdate(t *testing.T) {
	srv, _ := setupSlack(t)
	home := viewOf(t, publishForm(t, srv, url.Values{"user_id": {"U_USER"},
		"view": {`{"type":"home","external_id":"ext-home","blocks":[]}`}}))

	update := func(form url.Values) map[string]any {
		_, m := call(t, srv, "POST", "/api/views.update", formType, form.Encode(), true)
		return m
	}
	byID := viewOf(t, update(url.Values{"view_id": {home["id"].(string)},
		"view": {`{"type":"home","callback_id":"by_id","blocks":[]}`}}))
	if byID["id"] != home["id"] || byID["type"] != "home" || byID["callback_id"] != "by_id" {
		t.Errorf("update by view_id: %v", byID)
	}
	if byID["external_id"] != "ext-home" {
		t.Errorf("an update with no external_id keeps the view's own: %v", byID)
	}
	byExt := viewOf(t, update(url.Values{"external_id": {"ext-home"},
		"view": {`{"type":"home","callback_id":"by_ext","blocks":[]}`}}))
	if byExt["id"] != home["id"] || byExt["callback_id"] != "by_ext" {
		t.Errorf("update by external_id: %v", byExt)
	}

	for name, tc := range map[string]struct {
		form url.Values
		want string
	}{
		"unknown view_id":     {url.Values{"view_id": {"V_NOSUCH"}, "view": {homeView}}, "not_found"},
		"unknown external_id": {url.Values{"external_id": {"nope"}, "view": {homeView}}, "not_found"},
		"neither id":          {url.Values{"view": {homeView}}, "invalid_arguments"},
		"no view":             {url.Values{"view_id": {home["id"].(string)}}, "invalid_arguments"},
		"stale hash":          {url.Values{"view_id": {home["id"].(string)}, "view": {homeView}, "hash": {home["hash"].(string)}}, "hash_conflict"},
	} {
		if m := update(tc.form); m["error"] != tc.want {
			t.Errorf("%s: want %s, got %v", name, tc.want, m)
		}
	}

	// The update is what a later read sees: publishing again with the hash
	// the update returned succeeds.
	viewOf(t, publishForm(t, srv, url.Values{"user_id": {"U_USER"}, "view": {homeView}, "hash": {byExt["hash"].(string)}}))
}

func TestViewsOpenedModalCanBeUpdated(t *testing.T) {
	srv, _ := setupSlack(t)
	_, m := call(t, srv, "POST", "/api/views.open", jsonType,
		`{"trigger_id":"1.2.abc","view":{"type":"modal","title":{"type":"plain_text","text":"Hi"},"blocks":[]}}`, true)
	modal := viewOf(t, m)
	if modal["type"] != "modal" {
		t.Fatalf("views.open: %v", modal)
	}
	body, _ := json.Marshal(map[string]any{"view_id": modal["id"],
		"view": map[string]any{"type": "modal", "callback_id": "step2", "blocks": []any{}}})
	_, m = call(t, srv, "POST", "/api/views.update", jsonType, string(body), true)
	if got := viewOf(t, m); got["id"] != modal["id"] || got["callback_id"] != "step2" {
		t.Errorf("update of an opened modal: %v", got)
	}
}
