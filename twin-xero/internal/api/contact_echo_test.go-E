package api

import (
	"net/http"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// Audit finding F9: a refused contact is echoed as it was submitted, with no
// fields the caller did not send.
func TestARefusedContactEchoesOnlyWhatWasSent(t *testing.T) {
	h := newTestHandler(t)
	for _, tc := range []struct{ method, body string }{
		{http.MethodPut, `{"Contacts":[{"Name":""}]}`},
		{http.MethodPost, `{"Contacts":[{"Name":""}]}`},
	} {
		rec := do(h, tc.method, "/api.xro/2.0/Contacts", tc.body, accounting(t, h, store.DefaultTenantID))
		if rec.Code != 400 {
			t.Fatalf("%s: %d %s", tc.method, rec.Code, rec.Body)
		}
		els := decode(t, rec)["Elements"].([]any)
		el := els[0].(map[string]any)
		for _, k := range []string{"ContactID", "Addresses", "UpdatedDateUTC", "ContactStatus"} {
			if _, ok := el[k]; ok {
				t.Errorf("%s: the element carries %s, which was not sent: %v", tc.method, k, el)
			}
		}
		if el["Name"] != "" || el["ValidationErrors"] == nil {
			t.Errorf("%s: element = %v, want the submitted Name and its ValidationErrors", tc.method, el)
		}
	}
}
