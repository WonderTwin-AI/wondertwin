package api

import (
	"math"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"time"

	kitpage "github.com/wondertwin-ai/wondertwin/twinkit/pagination"
)

const (
	defaultPageSize = 100
	maxPageSize     = 1000
	maxPage         = math.MaxInt / maxPageSize
)

// pagination is the object Xero returns beside a page of results.
type pagination struct {
	Page      int `json:"page"`
	PageSize  int `json:"pageSize"`
	PageCount int `json:"pageCount"`
	ItemCount int `json:"itemCount"`
}

// paginate orders a list the way Xero does and cuts it the way the request asks.
//
// Lists are ordered by UpdatedDateUTC and then by ID, oldest first (page-003).
// A request without page gets every record in summary form, with no pagination
// object; a request with page gets that 1-based page in full detail plus the
// pagination object (page-001, page-002). pageSize defaults to 100 and is
// clamped to 1..1000. A page or pageSize that is not a number takes its default.
func paginate[T any](r *http.Request, items []T, key func(T) (time.Time, string), summary func(T) T) ([]T, *pagination) {
	sorted := slices.Clone(items)
	sort.SliceStable(sorted, func(i, j int) bool {
		ti, idi := key(sorted[i])
		tj, idj := key(sorted[j])
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return idi < idj
	})

	q := r.URL.Query()
	if q.Get("page") == "" {
		out := make([]T, len(sorted))
		for i, item := range sorted {
			out[i] = summary(item)
		}
		return out, nil
	}
	size := min(max(intOr(q.Get("pageSize"), defaultPageSize), 1), maxPageSize)
	p := kitpage.PagePaginate(sorted, min(intOr(q.Get("page"), 1), maxPage), size)
	return p.Data, &pagination{Page: p.Page, PageSize: p.PageSize, PageCount: p.TotalPages, ItemCount: p.TotalCount}
}

func intOr(s string, fallback int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return fallback
}
