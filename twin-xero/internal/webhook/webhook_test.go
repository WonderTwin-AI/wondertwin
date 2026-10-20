package webhook_test

import (
	"strings"
	"testing"

	xerowh "github.com/wondertwin-ai/wondertwin/twin-xero/internal/webhook"
)

// The vectors were computed with openssl (echo -n BODY | openssl dgst -sha256
// -hmac KEY -binary | base64), not with this package.
func TestWebhookSignMatchesIndependentVectors(t *testing.T) {
	cases := []struct{ name, body, key, want string }{
		{"hello", "hello", "secret", "iKqz7ejTrflNJquQ07r9SiCDBww7zOnAFO4EpEOEfAs="},
		{"empty intent-to-receive body", `{"events":[],"lastEventSequence":0,"firstEventSequence":0,"entropy":"E"}`, "whkey", "818bg+dYTTNSNjkWgT41BdICdOjQALbriWxQAgTYSpk="},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := xerowh.Signature([]byte(tc.body), tc.key); got != tc.want {
				t.Fatalf("Signature = %s, want %s", got, tc.want)
			}
			h := xerowh.Signer{}.Sign([]byte(tc.body), tc.key)
			if len(h) != 1 || h[xerowh.SignatureHeader] != tc.want {
				t.Fatalf("Sign = %v, want only %s: %s", h, xerowh.SignatureHeader, tc.want)
			}
		})
	}
}

func TestWebhookSignChangesWithTheBodyAndTheKey(t *testing.T) {
	base := xerowh.Signature([]byte("body"), "key")
	if xerowh.Signature([]byte("body "), "key") == base || xerowh.Signature([]byte("body"), "key2") == base {
		t.Fatal("a different body or key must give a different signature")
	}
	if xerowh.SignatureHeader != "x-xero-signature" {
		t.Fatalf("header = %q", xerowh.SignatureHeader)
	}
}

func TestWebhookPayloadBodyHasXerosFieldsInXerosOrder(t *testing.T) {
	p := xerowh.Payload{
		Events: []xerowh.Event{{
			ResourceURL: "http://h/api.xro/2.0/Contacts/c1", ResourceID: "c1", EventDateUTC: "2026-10-19T01:02:03.456",
			EventType: "CREATE", EventCategory: "CONTACT", TenantID: "t1", TenantType: "ORGANISATION",
		}},
		FirstEventSequence: 7, LastEventSequence: 7, Entropy: "ABC",
	}
	want := `{"events":[{"resourceUrl":"http://h/api.xro/2.0/Contacts/c1","resourceId":"c1","eventDateUtc":"2026-10-19T01:02:03.456",` +
		`"eventType":"CREATE","eventCategory":"CONTACT","tenantId":"t1","tenantType":"ORGANISATION"}],` +
		`"lastEventSequence":7,"firstEventSequence":7,"entropy":"ABC"}`
	if got := string(p.Body()); got != want {
		t.Fatalf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestWebhookPayloadWithNoEventsIsAnEmptyArrayNotNull(t *testing.T) {
	got := string(xerowh.Payload{Entropy: "E"}.Body())
	if !strings.Contains(got, `"events":[]`) || strings.Contains(got, "null") {
		t.Fatalf("an intent-to-receive body must carry events: [], got %s", got)
	}
}

func TestWebhookPayloadDataIsOnlyWrittenWhenSet(t *testing.T) {
	with := string(xerowh.Payload{Events: []xerowh.Event{{Data: map[string]any{"Type": "ACCRECCREDIT", "Status": "AUTHORISED"}}}}.Body())
	without := string(xerowh.Payload{Events: []xerowh.Event{{}}}.Body())
	if !strings.Contains(with, `"data":{"Status":"AUTHORISED","Type":"ACCRECCREDIT"}`) || strings.Contains(without, `"data"`) {
		t.Fatalf("data must appear only for the categories that carry it:\n%s\n%s", with, without)
	}
}
