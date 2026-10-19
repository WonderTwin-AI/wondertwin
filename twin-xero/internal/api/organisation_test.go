package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func accounting(t *testing.T, h *Handler, tenant string) map[string]string {
	t.Helper()
	hdr := map[string]string{"Authorization": "Bearer " + tokenFor(t, h)}
	if tenant != "" {
		hdr["xero-tenant-id"] = tenant
	}
	return hdr
}

func TestOrganisationIsTheDefaultTenantsInThePluralEnvelope(t *testing.T) {
	h := newTestHandler()
	rec := get(h, "/api.xro/2.0/Organisation", accounting(t, h, store.DefaultTenantID))
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Status        string
		ProviderName  string
		Organisations []map[string]any
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Status != "OK" || len(body.Organisations) != 1 {
		t.Fatalf("want one organisation in the envelope: %v %s", err, rec.Body)
	}
	o := body.Organisations[0]
	if o["Name"] != store.DefaultTenantName || o["Version"] != "GLOBAL" || o["BaseCurrency"] != "USD" {
		t.Fatalf("organisation = %v", o)
	}
	// xero-python parses these as ms-format dates and raises on ISO strings (time-001).
	if s, _ := o["CreatedDateUTC"].(string); !msdateValue.MatchString(s) {
		t.Fatalf("CreatedDateUTC = %v, want /Date(ms+0000)/", o["CreatedDateUTC"])
	}
	for _, k := range []string{"Addresses", "Phones", "ExternalLinks"} {
		if list, ok := o[k].([]any); !ok || list == nil {
			t.Fatalf("%s must be an array, never null: %v", k, o[k])
		}
	}
	for _, k := range []string{"APIKey", "TaxNumber", "PeriodLockDate"} {
		if _, ok := o[k]; ok {
			t.Fatalf("%s is unset and must be omitted, not null", k)
		}
	}
}

func TestTenantHeaderIsRequiredAfterTheToken(t *testing.T) {
	h := newTestHandler()
	if rec := get(h, "/api.xro/2.0/Organisation", accounting(t, h, "")); rec.Code != 403 {
		t.Fatalf("no xero-tenant-id: want 403, got %d", rec.Code)
	}
	// Token validation precedes tenant validation: no valid token is 401 whatever tenant it names.
	h.store.StrictAuth.Store(true)
	for _, tenant := range []string{"", store.DefaultTenantID, "nope"} {
		hdr := map[string]string{"Authorization": "Bearer made-up"}
		if tenant != "" {
			hdr["xero-tenant-id"] = tenant
		}
		if rec := get(h, "/api.xro/2.0/Organisation", hdr); rec.Code != http.StatusUnauthorized {
			t.Fatalf("invalid token with tenant %q: want 401, got %d", tenant, rec.Code)
		}
	}
}

func TestTenantUnknownIdIsEmptyUnlessStrict(t *testing.T) {
	h := newTestHandler()
	rec := get(h, "/api.xro/2.0/Organisation", accounting(t, h, "0f0f0f0f-0000-4000-8000-00000000dead"))
	if rec.Code != 200 || !json.Valid(rec.Body.Bytes()) {
		t.Fatalf("lenient: an unknown tenant is accepted, got %d", rec.Code)
	}
	var body struct{ Organisations []any }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Organisations == nil || len(body.Organisations) != 0 {
		t.Fatalf("an unknown tenant sees an empty list, got %v", body.Organisations)
	}
	h.store.StrictAuth.Store(true)
	if rec := get(h, "/api.xro/2.0/Organisation", accounting(t, h, "0f0f0f0f-0000-4000-8000-00000000dead")); rec.Code != 403 {
		t.Fatalf("strict: an unknown tenant must be 403, got %d", rec.Code)
	}
}
