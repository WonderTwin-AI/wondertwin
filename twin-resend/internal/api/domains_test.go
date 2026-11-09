package api_test

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

// wantDomainRecords are the record, name, type and value Resend lists for a
// us-east-1 domain, DKIM value aside (probe m01 of 2026-11-08).
var wantDomainRecords = [][4]string{
	{"DKIM", "resend._domainkey", "TXT", ""},
	{"SPF", "send", "MX", "feedback-smtp.us-east-1.amazonses.com"},
	{"SPF", "send", "TXT", "v=spf1 include:amazonses.com ~all"},
	{"SPF", "rsend", "CNAME", "send.forge.rmta.net"},
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// TestCreateDomain checks POST /domains answers Resend's create shape: the
// four DNS records, capabilities, and no tracking switches.
func TestCreateDomain(t *testing.T) {
	_, tc := setupResend(t)
	m := resendPost(tc, "/domains", map[string]any{"name": "acme.dev"}).AssertStatus(201).JSONMap()

	want := []string{"capabilities", "created_at", "id", "name", "object", "records", "region", "status"}
	if got := sortedKeys(m); !reflect.DeepEqual(got, want) {
		t.Errorf("create keys %v, want %v", got, want)
	}
	if m["object"] != "domain" || m["status"] != "not_started" || m["region"] != "us-east-1" {
		t.Errorf("create got %v", m)
	}
	wantCaps := map[string]any{"sending": "enabled", "receiving": "disabled"}
	if !reflect.DeepEqual(m["capabilities"], wantCaps) {
		t.Errorf("capabilities %v, want %v", m["capabilities"], wantCaps)
	}
	assertDomainRecords(t, m["records"].([]any))
}

func assertDomainRecords(t *testing.T, records []any) {
	t.Helper()
	if len(records) != len(wantDomainRecords) {
		t.Fatalf("got %d records, want %d", len(records), len(wantDomainRecords))
	}
	for i, w := range wantDomainRecords {
		r := records[i].(map[string]any)
		if r["record"] != w[0] || r["name"] != w[1] || r["type"] != w[2] || r["ttl"] != "Auto" || r["status"] != "not_started" {
			t.Errorf("record %d: got %v, want %v", i, r, w)
		}
		if w[3] != "" && r["value"] != w[3] {
			t.Errorf("record %d value %v, want %s", i, r["value"], w[3])
		}
	}
	if p := records[1].(map[string]any)["priority"]; p != float64(10) {
		t.Errorf("MX priority %v, want 10", p)
	}
	dkim, _ := records[0].(map[string]any)["value"].(string)
	der, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dkim, "p="))
	if !strings.HasPrefix(dkim, "p=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQ") || err != nil || len(der) != 162 {
		t.Errorf("DKIM value %q, want p= and a base64 1024-bit RSA public key", dkim)
	}
}

// TestGetDomain checks GET /domains/{id} adds open_tracking and
// click_tracking to the create shape, and GET /domains lists a domain
// without object or records (probes m02, m03 of 2026-11-08).
func TestGetDomain(t *testing.T) {
	_, tc := setupResend(t)
	created := resendPost(tc, "/domains", map[string]any{"name": "acme.dev", "region": "eu-west-1"}).AssertStatus(201).JSONMap()
	id := created["id"].(string)

	got := resendGet(tc, "/domains/"+id).AssertStatus(200).JSONMap()
	if got["open_tracking"] != false || got["click_tracking"] != false {
		t.Errorf("tracking: got %v", got)
	}
	delete(got, "open_tracking")
	delete(got, "click_tracking")
	if !reflect.DeepEqual(got, created) {
		t.Errorf("get %v, want the create shape %v", got, created)
	}
	mx := got["records"].([]any)[1].(map[string]any)
	if mx["value"] != "feedback-smtp.eu-west-1.amazonses.com" {
		t.Errorf("MX value %v, want the eu-west-1 host", mx["value"])
	}

	item := resendGet(tc, "/domains").AssertStatus(200).JSONMap()["data"].([]any)[0].(map[string]any)
	want := []string{"capabilities", "click_tracking", "created_at", "id", "name", "open_tracking", "region", "status"}
	if keys := sortedKeys(item); !reflect.DeepEqual(keys, want) {
		t.Errorf("list item keys %v, want %v", keys, want)
	}

	resendGet(tc, "/domains/00000000-0000-4000-8000-000000000000").AssertStatus(404)

	id = resendPost(tc, "/domains", map[string]any{
		"name": "tracked.acme.dev", "open_tracking": true, "click_tracking": true,
		"capabilities": map[string]any{"receiving": "enabled"},
	}).AssertStatus(201).JSONMap()["id"].(string)
	got = resendGet(tc, "/domains/"+id).AssertStatus(200).JSONMap()
	wantCaps := map[string]any{"sending": "enabled", "receiving": "enabled"}
	if got["open_tracking"] != true || got["click_tracking"] != true || !reflect.DeepEqual(got["capabilities"], wantCaps) {
		t.Errorf("create switches not kept: got %v", got)
	}
}

// TestCreateDomainErrors checks a missing name, an unknown region and a
// name already on the account answer as Resend's do (probes m05 to m07 of
// 2026-11-08).
func TestCreateDomainErrors(t *testing.T) {
	_, tc := setupResend(t)

	m := resendPost(tc, "/domains", map[string]any{}).AssertStatus(422).JSONMap()
	assertResendError(t, m, "missing_required_field", "Missing `name` field.")

	m = resendPost(tc, "/domains", map[string]any{"name": "acme.dev", "region": "mars-1"}).AssertStatus(422).JSONMap()
	assertResendError(t, m, "validation_error",
		"The `region` field must be one of \"us-east-1\", \"eu-west-1\", \"sa-east-1\", or \"ap-northeast-1\".")

	for _, region := range []string{"us-east-1", "eu-west-1", "sa-east-1", "ap-northeast-1"} {
		resendPost(tc, "/domains", map[string]any{"name": region + ".acme.dev", "region": region}).AssertStatus(201)
	}

	resendPost(tc, "/domains", map[string]any{"name": "acme.dev"}).AssertStatus(201)
	m = resendPost(tc, "/domains", map[string]any{"name": "Acme.dev"}).AssertStatus(403).JSONMap()
	assertResendError(t, m, "validation_error", "The Acme.dev domain has been registered already.")
}

// TestDomainStateRoundTrip checks capabilities, tracking and the DKIM key
// survive /admin/state, and that a seed repeats the DKIM key.
func TestDomainStateRoundTrip(t *testing.T) {
	tc, s := setupResendWithStore(t)
	s.Seed(7)
	adm := testutil.NewAdminClient(tc)
	id := resendPost(tc, "/domains", map[string]any{"name": "acme.dev"}).AssertStatus(201).JSONMap()["id"].(string)
	before := resendGet(tc, "/domains/"+id).AssertStatus(200).JSONMap()

	snap := adm.GetState().AssertStatus(200).Body
	adm.Reset().AssertStatus(200)
	adm.LoadState(json.RawMessage(snap)).AssertStatus(200)
	if after := resendGet(tc, "/domains/"+id).AssertStatus(200).JSONMap(); !reflect.DeepEqual(after, before) {
		t.Errorf("after load %v, want %v", after, before)
	}

	adm.Reset().AssertStatus(200)
	again := resendPost(tc, "/domains", map[string]any{"name": "acme.dev"}).AssertStatus(201).JSONMap()
	if again["records"].([]any)[0].(map[string]any)["value"] != before["records"].([]any)[0].(map[string]any)["value"] {
		t.Error("the DKIM key did not repeat for the same seed after reset")
	}
}

// TestCreateDomainHeldElsewhere checks the names Resend refuses as
// registered by another account answer 422 (quirk lifecycle-013, probes m00
// to m00c of 2026-11-08), whatever the case, while a look-alike is accepted.
func TestCreateDomainHeldElsewhere(t *testing.T) {
	_, tc := setupResend(t)
	for _, name := range []string{"example.com", "mail.example.com", "News.Example.org", "app.test"} {
		m := resendPost(tc, "/domains", map[string]any{"name": name}).AssertStatus(422).JSONMap()
		assertResendError(t, m, "validation_error", "This domain has been registered already.")
	}
	for _, name := range []string{"myexample.com", "example.company", "test.dev"} {
		resendPost(tc, "/domains", map[string]any{"name": name}).AssertStatus(201)
	}
}
