package api

import (
	"net/http"

	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// paginate returns one Stripe list page over s, newest first.
//
// Stripe lists are in reverse chronological order (api/pagination). The kit
// store keeps insertion order, oldest first, so this walks it backwards.
// starting_after pages towards older objects and ending_before towards newer
// ones; keep, when non-nil, filters before the page is cut, so has_more and
// the cursors refer to the filtered list.
func paginate[T any](r *http.Request, s *pkgstate.Store[T], limit int, keep func(T) bool) pkgstate.Page[T] {
	ids := s.ListIDs()
	type entry struct {
		id   string
		item T
	}
	all := make([]entry, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		item, ok := s.Get(ids[i])
		if !ok {
			continue
		}
		if keep != nil && !keep(item) {
			continue
		}
		all = append(all, entry{id: ids[i], item: item})
	}

	q := r.URL.Query()
	start, end := 0, len(all)
	if after := q.Get("starting_after"); after != "" {
		for i, e := range all {
			if e.id == after {
				start = i + 1
				break
			}
		}
	} else if before := q.Get("ending_before"); before != "" {
		for i, e := range all {
			if e.id == before {
				end = i
				break
			}
		}
	}

	hasMore := false
	if limit > 0 && end-start > limit {
		hasMore = true
		if q.Get("starting_after") == "" && q.Get("ending_before") != "" {
			start = end - limit
		} else {
			end = start + limit
		}
	}

	data := make([]T, 0, end-start)
	cursor := ""
	for _, e := range all[start:end] {
		data = append(data, e.item)
		cursor = e.id
	}
	return pkgstate.Page[T]{Data: data, HasMore: hasMore, Cursor: cursor, Total: len(all)}
}
