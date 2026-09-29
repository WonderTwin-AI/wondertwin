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
func paginate[T any](w http.ResponseWriter, r *http.Request, s *pkgstate.Store[T], object string, limit int, keep func(T) bool) (pkgstate.Page[T], bool) {
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
	cursorAt := func(param string) (int, bool) {
		id := q.Get(param)
		for i, e := range all {
			if e.id == id {
				return i, true
			}
		}
		writeError(w, http.StatusBadRequest, apiError{
			Type:    "invalid_request_error",
			Code:    "resource_missing",
			Message: "No such " + object + ": '" + id + "'",
			Param:   param,
		})
		return 0, false
	}
	if q.Get("starting_after") != "" {
		i, ok := cursorAt("starting_after")
		if !ok {
			return pkgstate.Page[T]{}, false
		}
		start = i + 1
	} else if q.Get("ending_before") != "" {
		i, ok := cursorAt("ending_before")
		if !ok {
			return pkgstate.Page[T]{}, false
		}
		end = i
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
	return pkgstate.Page[T]{Data: data, HasMore: hasMore, Cursor: cursor, Total: len(all)}, true
}
