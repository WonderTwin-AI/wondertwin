package api_test

import (
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/testutil"
)

func listInstitutions(tc *testutil.TwinClient, body map[string]any) map[string]any {
	return tc.Post("/institutions/get", creds(body)).AssertStatus(200).JSONMap()
}

func ids(m map[string]any) []string {
	var out []string
	for _, in := range m["institutions"].([]any) {
		out = append(out, in.(map[string]any)["institution_id"].(string))
	}
	return out
}

func TestInstitutionsGetPagesByOffsetInIDOrder(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	all := listInstitutions(tc, map[string]any{"count": 500, "offset": 0, "country_codes": []string{"US"}})
	full := ids(all)
	if len(full) < 15 || all["total"] != float64(len(full)) {
		t.Fatalf("total = %v for %d institutions", all["total"], len(full))
	}
	var paged []string
	for offset := 0; offset < len(full); offset += 7 {
		page := listInstitutions(tc, map[string]any{"count": 7, "offset": offset, "country_codes": []string{"US"}})
		if page["total"] != all["total"] {
			t.Fatalf("total changed across pages: %v", page["total"])
		}
		paged = append(paged, ids(page)...)
	}
	if strings.Join(paged, ",") != strings.Join(full, ",") {
		t.Fatalf("pages do not add up to the full list:\n%v\n%v", paged, full)
	}
	past := listInstitutions(tc, map[string]any{"count": 5, "offset": 1000, "country_codes": []string{"US"}})
	if len(ids(past)) != 0 || past["total"] != all["total"] {
		t.Fatalf("an offset past the end = %v", past)
	}
}

func TestInstitutionsGetFiltersByCountryAndOptions(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	if got := ids(listInstitutions(tc, map[string]any{"count": 50, "offset": 0, "country_codes": []string{"CA"}})); len(got) != 1 || got[0] != "ins_43" {
		t.Fatalf("CA = %v", got)
	}
	for _, id := range ids(listInstitutions(tc, map[string]any{"count": 50, "offset": 0, "country_codes": []string{"US", "GB"},
		"options": map[string]any{"oauth": true}})) {
		if id == "ins_109508" {
			t.Fatalf("a non-OAuth institution came back for oauth=true")
		}
	}
	none := listInstitutions(tc, map[string]any{"count": 50, "offset": 0, "country_codes": []string{"US"},
		"options": map[string]any{"products": []string{"auth", "not_a_product"}}})
	if len(ids(none)) != 0 || none["total"] != 0.0 {
		t.Fatalf("an institution supports a product nobody does: %v", none)
	}
}

func TestInstitutionsGetChecksItsFields(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	m := tc.Post("/institutions/get", creds(map[string]any{})).AssertStatus(400).JSONMap()
	if m["error_code"] != "MISSING_FIELDS" || m["error_message"] != "the following required fields are missing: count, offset, country_codes" {
		t.Fatalf("missing = %v", m)
	}
	for body, code := range map[string]string{"count 0": "INVALID_FIELD", "count 501": "INVALID_FIELD", "offset -1": "INVALID_FIELD", "count text": "INVALID_BODY", "count 1.5": "INVALID_BODY"} {
		b := map[string]any{"count": 5, "offset": 0, "country_codes": []string{"US"}}
		switch body {
		case "count 0":
			b["count"] = 0
		case "count 501":
			b["count"] = 501
		case "offset -1":
			b["offset"] = -1
		case "count text":
			b["count"] = "5"
		case "count 1.5":
			b["count"] = 1.5
		}
		if m := tc.Post("/institutions/get", creds(b)).AssertStatus(400).JSONMap(); m["error_code"] != code {
			t.Fatalf("%s: %v", body, m)
		}
	}
}

func TestInstitutionsSearchMatchesNamesAndLimitsToTen(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	search := func(q string, extra map[string]any) []any {
		b := map[string]any{"query": q, "country_codes": []string{"US"}}
		for k, v := range extra {
			b[k] = v
		}
		return tc.Post("/institutions/search", creds(b)).AssertStatus(200).JSONMap()["institutions"].([]any)
	}
	for _, in := range search("PLATYPUS", nil) {
		if !strings.Contains(strings.ToLower(in.(map[string]any)["name"].(string)), "platypus") {
			t.Fatalf("a name without the query came back: %v", in)
		}
	}
	if got := len(search("bank", nil)); got != 10 {
		t.Fatalf("a broad query returned %d, want the limit of 10", got)
	}
	if got := search("houndstooth", map[string]any{"products": []string{"auth"}}); len(got) != 1 {
		t.Fatalf("houndstooth with auth = %v", got)
	}
	if got := search("houndstooth", map[string]any{"products": []string{"not_a_product"}}); len(got) != 0 {
		t.Fatalf("houndstooth with an unsupported product = %v", got)
	}
	if m := tc.Post("/institutions/search", creds(map[string]any{"country_codes": []string{"US"}})).AssertStatus(400).JSONMap(); m["error_code"] != "MISSING_FIELDS" {
		t.Fatalf("missing query = %v", m)
	}
}

func TestInstitutionsGetByIDReturnsThePublishedSampleAndOmitsOptionalFields(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	res := tc.Post("/institutions/get_by_id", creds(map[string]any{"institution_id": "ins_109512", "country_codes": []string{"US"}})).
		AssertStatus(200).JSONMap()
	in := res["institution"].(map[string]any)
	if in["name"] != "Houndstooth Bank" || in["oauth"] != false || len(in["dtc_numbers"].([]any)) != 3 ||
		in["routing_numbers"].([]any)[0] != "011000138" || res["request_id"] == nil {
		t.Fatalf("institution = %v", in)
	}
	for _, k := range []string{"url", "logo", "primary_color", "status"} {
		if _, ok := in[k]; ok {
			t.Fatalf("optional %s must be omitted, not sent: %v", k, in)
		}
	}
	bare := tc.Post("/institutions/get_by_id", creds(map[string]any{"institution_id": "ins_109508", "country_codes": []string{"US"}})).
		AssertStatus(200).JSONMap()["institution"].(map[string]any)
	if r, ok := bare["routing_numbers"].([]any); !ok || len(r) != 0 {
		t.Fatalf("routing_numbers must be an empty array when none are published: %v", bare)
	}
	if m := tc.Post("/institutions/get_by_id", creds(map[string]any{"institution_id": "ins_nope", "country_codes": []string{"US"}})).
		AssertStatus(400).JSONMap(); m["error_code"] != "INVALID_INSTITUTION" || m["error_type"] != "INVALID_INPUT" {
		t.Fatalf("unknown id = %v", m)
	}
}

func TestInstitutionsComeBackAfterAReset(t *testing.T) {
	tc, ac, _ := setupPlaid(t)
	ac.Reset().AssertStatus(200)
	if got := listInstitutions(tc, map[string]any{"count": 1, "offset": 0, "country_codes": []string{"US"}}); got["total"].(float64) < 15 {
		t.Fatalf("institutions after a reset = %v", got["total"])
	}
}
