package api

import (
	"net/http"
	"slices"
	"strconv"
)

// listBody is Resend's list envelope with every item and has_more false. An
// empty list is [], never null, so SDK callers can iterate data on a fresh
// emulator.
func listBody[T any](items []T) map[string]any {
	return pageBody(items, false)
}

func pageBody[T any](items []T, hasMore bool) map[string]any {
	if items == nil {
		items = []T{}
	}
	return map[string]any{"object": "list", "has_more": hasMore, "data": items}
}

const paginationLimitMessage = "The pagination limit must be a number between 1 and 100. See https://resend.com/docs/api-reference/pagination for more information."

// pageQuery is a list request's limit, after and before (quirk pagination-001).
// limit is 0 when the request names none.
type pageQuery struct {
	limit         int
	after, before string
}

// readPageQuery parses the pagination parameters. A limit outside 1 to 100,
// or one that is not a number, answers 422 as Resend does (probes g04, g05 of
// 2026-11-04; m04, o10 of 2026-11-08). It reports false once it has answered.
func readPageQuery(w http.ResponseWriter, r *http.Request) (pageQuery, bool) {
	q := r.URL.Query()
	var pq pageQuery
	if raw, ok := q["limit"]; ok {
		n, err := strconv.Atoi(raw[0])
		if err != nil || n < 1 || n > 100 {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", paginationLimitMessage)
			return pq, false
		}
		pq.limit = n
	}
	pq.after, pq.before = q.Get("after"), q.Get("before")
	if pq.after != "" && pq.before != "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error",
			"You can only use either `after` or `before`, not both simultaneously.")
		return pq, false
	}
	return pq, true
}

// paginate answers one page of items, which arrive oldest first (store
// insertion order) and are listed newest first. after starts past the named
// item and before ends short of it, the cursor itself excluded. Without a
// limit, defaultLimit applies; 0 returns every item, as the older Resend
// lists do (quirk pagination-002). An unknown cursor yields an empty page.
func paginate[T any](items []T, pq pageQuery, defaultLimit int, id func(T) string) map[string]any {
	newest := slices.Clone(items)
	slices.Reverse(newest)
	limit := pq.limit
	if limit == 0 {
		limit = defaultLimit
	}
	idx := func(cursor string) int {
		return slices.IndexFunc(newest, func(t T) bool { return id(t) == cursor })
	}
	switch {
	case pq.after != "":
		i := idx(pq.after)
		if i < 0 {
			return pageBody([]T{}, false)
		}
		rest := newest[i+1:]
		if limit == 0 || limit >= len(rest) {
			return pageBody(rest, false)
		}
		return pageBody(rest[:limit], true)
	case pq.before != "":
		i := idx(pq.before)
		if i < 0 {
			return pageBody([]T{}, false)
		}
		head := newest[:i]
		if limit == 0 || limit >= len(head) {
			return pageBody(head, false)
		}
		return pageBody(head[len(head)-limit:], true)
	default:
		if limit == 0 || limit >= len(newest) {
			return pageBody(newest, false)
		}
		return pageBody(newest[:limit], true)
	}
}
