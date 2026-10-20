package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// seededContacts creates four contacts a minute apart, so UpdatedDateUTC orders them.
func seededContacts(t *testing.T) (*Handler, map[string]string) {
	t.Helper()
	h, ids := newTestHandler(), map[string]string{}
	for _, c := range []string{
		`{"Name":"Acme Ltd","AccountNumber":"cus_1","IsCustomer":true,"EmailAddress":"ap@acme.test"}`,
		`{"Name":"Bolt Co","AccountNumber":"cus_2","IsSupplier":true,"FirstName":"Bo","ContactPersons":[{"FirstName":"P"}]}`,
		`{"Name":"Cora Inc","AccountNumber":"cus_3","IsCustomer":true}`,
		`{"Name":"Dormant","AccountNumber":"cus_4","ContactStatus":"ARCHIVED"}`,
	} {
		list := putContacts(t, h, c)["Contacts"].([]any)
		ids[list[0].(map[string]any)["Name"].(string)] = list[0].(map[string]any)["ContactID"].(string)
		h.store.Clock.Advance(time.Minute)
	}
	return h, ids
}

func listContacts(t *testing.T, h *Handler, query url.Values) (int, map[string]any) {
	t.Helper()
	rec := do(h, http.MethodGet, "/api.xro/2.0/Contacts?"+query.Encode(), "", accounting(t, h, store.DefaultTenantID))
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func names(body map[string]any) []string {
	var out []string
	for _, c := range body["Contacts"].([]any) {
		out = append(out, c.(map[string]any)["Name"].(string))
	}
	return out
}

func TestContactListWhere(t *testing.T) {
	h, ids := seededContacts(t)
	cases := []struct {
		where string
		want  []string
	}{
		{`AccountNumber="cus_2"`, []string{"Bolt Co"}}, // finance-ops sends a single =
		{`AccountNumber=="cus_2"`, []string{"Bolt Co"}},
		{`IsCustomer==true`, []string{"Acme Ltd", "Cora Inc"}},
		{`Name.StartsWith("B") OR Name.Contains("ora")`, []string{"Bolt Co", "Cora Inc"}},
		{`IsCustomer=true AND EmailAddress!=null`, []string{"Acme Ltd"}},
		{`ContactID==Guid("` + ids["Cora Inc"] + `")`, []string{"Cora Inc"}},
		{`UpdatedDateUTC>=DateTime(2000,1,1)`, []string{"Acme Ltd", "Bolt Co", "Cora Inc"}},
		{`AccountNumber="nobody"`, nil},
	}
	for _, tc := range cases {
		code, body := listContacts(t, h, url.Values{"where": {tc.where}})
		if got := names(body); code != 200 || len(got) != len(tc.want) || (len(got) > 0 && got[0] != tc.want[0]) || (len(got) > 1 && got[1] != tc.want[1]) {
			t.Errorf("%s: %d %v, want %v", tc.where, code, got, tc.want)
		}
	}
}

func TestContactListWhereErrorsAre400(t *testing.T) {
	h, _ := seededContacts(t)
	for _, where := range []string{`Name==`, `Name=="x" AND`, `IsCustomer>true`, `Name==5`} {
		code, body := listContacts(t, h, url.Values{"where": {where}})
		if code != 400 || body["Type"] != "ValidationException" || body["Message"] == "" {
			t.Errorf("%s: want a 400 ValidationException with a message, got %d %v", where, code, body)
		}
	}
}

func TestContactListFiltersSearchAndArchived(t *testing.T) {
	h, ids := seededContacts(t)
	both := ids["Acme Ltd"] + "," + ids["Cora Inc"]
	for name, tc := range map[string]struct {
		q    url.Values
		want int
	}{
		"IDs match any case and any of the list": {url.Values{"IDs": {both}}, 2},
		"IDs narrow the where":                   {url.Values{"IDs": {both}, "where": {"Name.Contains(\"Acme\")"}}, 1},
		"archived contacts are left out":         {nil, 3},
		"includeArchived brings them back":       {url.Values{"includeArchived": {"true"}}, 4},
		"searchTerm reads name and email":        {url.Values{"searchTerm": {"AP@ACME"}}, 1},
	} {
		if _, body := listContacts(t, h, tc.q); len(names(body)) != tc.want {
			t.Errorf("%s: got %v, want %d", name, names(body), tc.want)
		}
	}
}

func TestContactListPaginationAndSummaryForm(t *testing.T) {
	h, _ := seededContacts(t)

	_, plain := listContacts(t, h, nil)
	if _, ok := plain["pagination"]; ok {
		t.Fatal("without page there is no pagination object (page-001)")
	}
	if got := names(plain); got[0] != "Acme Ltd" || got[2] != "Cora Inc" {
		t.Fatalf("contacts come oldest first (page-003): %v", got)
	}
	bolt := plain["Contacts"].([]any)[1].(map[string]any)
	if len(bolt["ContactPersons"].([]any)) != 0 {
		t.Fatalf("without page a contact is in summary form, without its heavy parts (page-002): %v", bolt)
	}

	_, paged := listContacts(t, h, url.Values{"page": {"1"}, "pageSize": {"2"}})
	pg, _ := paged["pagination"].(map[string]any)
	if pg["page"] != float64(1) || pg["pageSize"] != float64(2) || pg["pageCount"] != float64(2) || pg["itemCount"] != float64(3) || len(names(paged)) != 2 {
		t.Fatalf("pagination = %v, names = %v", pg, names(paged))
	}
	if p := paged["Contacts"].([]any)[1].(map[string]any); len(p["ContactPersons"].([]any)) != 1 {
		t.Fatalf("with page a contact is in full detail: %v", p)
	}
	if _, body := listContacts(t, h, url.Values{"page": {"1"}, "summaryOnly": {"true"}}); len(body["Contacts"].([]any)[1].(map[string]any)["ContactPersons"].([]any)) != 0 {
		t.Fatal("summaryOnly gives the summary form even with page")
	}
}

func TestContactListFieldGetterReadsJSONNames(t *testing.T) {
	get := fieldGetter(store.Contact{Name: "n", IsCustomer: true, UpdatedDateUTC: time.Unix(5, 0)})
	for path, want := range map[string]any{"Name": "n", "IsCustomer": true, "UpdatedDateUTC": time.Unix(5, 0), "AccountNumber": nil} {
		if v, ok := get(path); !ok || v != want {
			t.Errorf("%s = %v (found %v), want %v (an unset optional reads as null)", path, v, ok, want)
		}
	}
	if _, ok := get("NoSuchField"); ok {
		t.Error("a field the record does not have is not found")
	}
}
