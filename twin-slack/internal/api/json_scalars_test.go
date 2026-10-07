package api_test

import (
	"strconv"
	"testing"
	"time"
)

// A JSON body may send a number or a boolean as a string, as slack_sdk does
// for chat.scheduleMessage's post_at. A string that is not one is still
// refused.
func TestJSONNumbersAndBooleansAsStrings(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "json-strings")

	at := time.Now().Add(time.Hour).Unix()
	status, m := call(t, srv, "POST", "/api/chat.scheduleMessage", jsonType,
		`{"channel":"`+ch+`","text":"later","post_at":"`+strconv.FormatInt(at, 10)+`"}`, true)
	mustOK(t, status, m)
	if m["post_at"] != float64(at) {
		t.Errorf("post_at as a string: %v", m["post_at"])
	}

	status, m = call(t, srv, "POST", "/api/conversations.history", jsonType,
		`{"channel":"`+ch+`","limit":"1","include_all_metadata":"true"}`, true)
	mustOK(t, status, m)
	if n := len(m["messages"].([]any)); n != 1 {
		t.Errorf("limit as a string: %d messages", n)
	}

	status, m = call(t, srv, "POST", "/api/chat.scheduleMessage", jsonType,
		`{"channel":"`+ch+`","text":"later","post_at":"tomorrow"}`, true)
	wantError(t, status, m, "invalid_arguments")
	status, m = call(t, srv, "POST", "/api/auth.revoke", jsonType, `{"test":"maybe"}`, true)
	wantError(t, status, m, "invalid_arguments")
}
