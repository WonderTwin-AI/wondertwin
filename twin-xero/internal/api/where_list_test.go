package api

import (
	"net/url"
	"reflect"
	"testing"
)

func TestWhereListParsesCommaSeparatedFilters(t *testing.T) {
	q := url.Values{
		"IDs":            {"a, b ,,c"},
		"ContactIDs":     {"x"},
		"InvoiceNumbers": {"INV-1,INV-2"},
		"Statuses":       {"PAID,AUTHORISED"},
		"References":     {" "},
	}
	want := Filters{
		IDs: []string{"a", "b", "c"}, ContactIDs: []string{"x"},
		InvoiceNumbers: []string{"INV-1", "INV-2"}, Statuses: []string{"PAID", "AUTHORISED"},
	}
	if got := ParseFilters(q); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	if got := ParseFilters(url.Values{}); !reflect.DeepEqual(got, Filters{}) {
		t.Fatalf("an empty query must send no filters, got %#v", got)
	}
}

func TestWhereListAppliesEveryFilterThatWasSent(t *testing.T) {
	rec := FilterFields{ID: "AB12-CD", ContactID: "99-EF", InvoiceNumber: "INV-2", Status: "PAID", Reference: "r1"}
	cases := []struct {
		name string
		q    url.Values
		want bool
	}{
		{"nothing sent", url.Values{}, true},
		{"id in list, any case", url.Values{"IDs": {"zz,ab12-cd"}}, true},
		{"id not in list", url.Values{"IDs": {"zz,yy"}}, false},
		{"contact id any case", url.Values{"ContactIDs": {"99-ef"}}, true},
		{"invoice number is exact", url.Values{"InvoiceNumbers": {"inv-2"}}, false},
		{"invoice number match", url.Values{"InvoiceNumbers": {"INV-1,INV-2"}}, true},
		{"status is exact", url.Values{"Statuses": {"paid"}}, false},
		{"reference match", url.Values{"References": {"r1"}}, true},
		{"every sent list must match", url.Values{"IDs": {"ab12-cd"}, "Statuses": {"DRAFT"}}, false},
		{"all lists match", url.Values{"IDs": {"ab12-cd"}, "ContactIDs": {"99-EF"}, "Statuses": {"PAID,DRAFT"}}, true},
	}
	for _, tc := range cases {
		if got := ParseFilters(tc.q).Match(rec); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
