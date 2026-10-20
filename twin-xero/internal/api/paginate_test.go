package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type row struct {
	ID      string
	Updated time.Time
	Lines   int // stands in for line items, which only a paged read carries
}

func rowKey(r row) (time.Time, string) { return r.Updated, r.ID }
func summarise(r row) row              { r.Lines = 0; return r }

func rows(n int) []row {
	out := make([]row, n)
	for i := range out {
		out[i] = row{ID: fmt.Sprintf("id-%04d", i), Updated: time.Unix(int64(1000+i), 0), Lines: 3}
	}
	return out
}

func page(t *testing.T, items []row, query string) ([]row, *pagination) {
	t.Helper()
	return paginate(httptest.NewRequest(http.MethodGet, "/x"+query, nil), items, rowKey, summarise)
}

func ids(items []row) string {
	var s []string
	for _, r := range items {
		s = append(s, r.ID)
	}
	return strings.Join(s, ",")
}

func TestPaginateOrdersByUpdatedThenID(t *testing.T) {
	at := time.Unix(5000, 0)
	shuffled := []row{{ID: "c", Updated: at}, {ID: "z", Updated: at.Add(-time.Hour)}, {ID: "a", Updated: at}, {ID: "b", Updated: at}}

	got, _ := page(t, shuffled, "")
	if ids(got) != "z,a,b,c" {
		t.Fatalf("order = %s, want z,a,b,c (UpdatedDateUTC, then ID)", ids(got))
	}
	if ids(shuffled) != "c,z,a,b" {
		t.Fatal("the caller's slice must not be reordered")
	}
}

func TestPaginateWithoutPageReturnsEverythingInSummary(t *testing.T) {
	got, pg := page(t, rows(250), "?pageSize=10")

	if pg != nil || len(got) != 250 {
		t.Fatalf("pagination %v, %d items: want no pagination object and every record", pg, len(got))
	}
	if got[0].Lines != 0 {
		t.Fatal("an unpaged list is summary only, without line items")
	}
}

func TestPaginateWithPageReturnsFullDetailAndThePaginationObject(t *testing.T) {
	cases := []struct {
		query string
		want  string
		pg    pagination
	}{
		{"?page=1&pageSize=2", "id-0000,id-0001", pagination{1, 2, 3, 5}},
		{"?page=3&pageSize=2", "id-0004", pagination{3, 2, 3, 5}},
		{"?page=9&pageSize=2", "", pagination{9, 2, 3, 5}},
		{"?page=2&pageSize=2&unrelated=1", "id-0002,id-0003", pagination{2, 2, 3, 5}},
	}
	for _, tc := range cases {
		got, pg := page(t, rows(5), tc.query)
		if ids(got) != tc.want || pg == nil || *pg != tc.pg {
			t.Errorf("%s: got %q %+v, want %q %+v", tc.query, ids(got), pg, tc.want, tc.pg)
		}
		if len(got) > 0 && got[0].Lines != 3 {
			t.Errorf("%s: a paged read carries full detail", tc.query)
		}
	}
}

func TestPaginatePageSizeIsClamped(t *testing.T) {
	cases := map[string]int{"": 100, "&pageSize=5000": 1000, "&pageSize=0": 1, "&pageSize=-3": 1, "&pageSize=abc": 100, "&pageSize=7": 7}
	for suffix, want := range cases {
		got, pg := page(t, rows(1200), "?page=1"+suffix)
		if pg.PageSize != want || len(got) != want {
			t.Errorf("pageSize%q: pageSize %d with %d items, want %d", suffix, pg.PageSize, len(got), want)
		}
	}
	if _, pg := page(t, rows(3), "?page=abc"); pg == nil || pg.Page != 1 {
		t.Fatalf("a page that is not a number takes page 1, got %+v", pg)
	}
	if _, pg := page(t, rows(3), "?page=0"); pg.Page != 1 {
		t.Fatalf("pages are 1-based, page=0 is page 1, got %+v", pg)
	}
}

func TestPaginateHugePageIsEmptyNotAPanic(t *testing.T) {
	for _, size := range []string{"", "&pageSize=1000"} {
		got, pg := page(t, rows(5), "?page=9223372036854775807"+size)
		if len(got) != 0 || pg == nil || pg.ItemCount != 5 {
			t.Errorf("pageSize%q: got %d items, %+v, want an empty page and itemCount 5", size, len(got), pg)
		}
	}
}

func TestPaginateEnvelopeCarriesThePaginationObjectBeforeTheArray(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	h.writeListEnvelope(rec, 200, "Invoices", []string{"a"}, &pagination{1, 100, 1, 1})

	body := rec.Body.String()
	if !(strings.Index(body, `"DateTimeUTC"`) < strings.Index(body, `"pagination"`) && strings.Index(body, `"pagination"`) < strings.Index(body, `"Invoices"`)) {
		t.Fatalf("pagination must sit between DateTimeUTC and the array: %s", body)
	}
	var out struct {
		Pagination map[string]int `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Pagination["pageSize"] != 100 || out.Pagination["itemCount"] != 1 {
		t.Fatalf("pagination = %v, %v", out.Pagination, err)
	}
	plain := httptest.NewRecorder()
	h.writeEnvelope(plain, 200, "Invoices", []string{"a"})
	if strings.Contains(plain.Body.String(), "pagination") {
		t.Fatal("an unpaged envelope has no pagination object")
	}
}
