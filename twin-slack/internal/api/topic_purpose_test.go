package api_test

import (
	"net/url"
	"testing"
)

// conversations.setTopic answers the conversation; setPurpose answers it as
// well as the purpose string its docs show. Neither changes an archived one.
func TestSetTopicAndPurposeAnswerTheConversation(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "described")

	topic := form(t, srv, "conversations.setTopic", url.Values{"channel": {ch}, "topic": {"Plans"}})
	mustOK(t, 200, topic)
	c := topic["channel"].(map[string]any)
	if c["id"] != ch || c["topic"].(map[string]any)["value"] != "Plans" {
		t.Errorf("setTopic channel: %v", c)
	}

	purpose := form(t, srv, "conversations.setPurpose", url.Values{"channel": {ch}, "purpose": {"Planning"}})
	mustOK(t, 200, purpose)
	if purpose["purpose"] != "Planning" || purpose["channel"].(map[string]any)["purpose"].(map[string]any)["value"] != "Planning" {
		t.Errorf("setPurpose: %v", purpose)
	}

	long := make([]byte, 251)
	for i := range long {
		long[i] = 'p'
	}
	wantErrors(t, srv, "conversations.setPurpose", map[string]errCase{
		"too long": {url.Values{"channel": {ch}, "purpose": {string(long)}}, "too_long"},
	})

	mustOK(t, 200, form(t, srv, "conversations.archive", url.Values{"channel": {ch}}))
	wantErrors(t, srv, "conversations.setTopic", map[string]errCase{
		"archived": {url.Values{"channel": {ch}, "topic": {"x"}}, "is_archived"},
	})
	wantErrors(t, srv, "conversations.setPurpose", map[string]errCase{
		"archived": {url.Values{"channel": {ch}, "purpose": {"x"}}, "is_archived"},
	})
}
