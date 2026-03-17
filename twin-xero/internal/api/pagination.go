package api

import (
	"net/http"
	"strconv"
)

const (
	defaultPageSize = 100
	maxPageSize     = 1000
)

// parsePagination extracts page and pageSize from query params.
func parsePagination(r *http.Request) (page, pageSize int) {
	page = 1
	pageSize = defaultPageSize

	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := r.URL.Query().Get("pageSize"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 {
			pageSize = v
		}
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return
}

// paginate returns a slice of items for the given page.
func paginate[T any](items []T, page, pageSize int) []T {
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []T{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
