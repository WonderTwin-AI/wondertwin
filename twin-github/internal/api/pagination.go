package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	defaultPerPage = 30
	maxPerPage     = 100
)

// pageParams reads GitHub's page and per_page query parameters: per_page
// defaults to 30 and is capped at 100, page is 1-based.
func pageParams(r *http.Request) (page, perPage int) {
	page, perPage = 1, defaultPerPage
	if v, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && v > 0 {
		perPage = min(v, maxPerPage)
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		page = v
	}
	return page, perPage
}

// paginate returns the requested page of items and sets GitHub's Link
// header, with prev, next, last and first relations as they apply. Links are
// absolute and point at the emulator, so Octokit's paginate and go-github's
// NextPage follow them without leaving it. It never returns nil, so an empty
// page encodes as [] rather than null.
func paginate[T any](w http.ResponseWriter, r *http.Request, items []T) []T {
	page, perPage := pageParams(r)
	total := len(items)
	last := max((total+perPage-1)/perPage, 1)

	start := min((page-1)*perPage, total)
	end := min(start+perPage, total)
	out := make([]T, end-start)
	copy(out, items[start:end])

	var links []string
	link := func(p int, rel string) {
		q := r.URL.Query()
		q.Set("page", strconv.Itoa(p))
		if r.URL.Query().Get("per_page") != "" {
			q.Set("per_page", strconv.Itoa(perPage))
		}
		links = append(links, fmt.Sprintf("<%s%s?%s>; rel=%q", origin(r), r.URL.Path, q.Encode(), rel))
	}
	if page > 1 {
		link(min(page-1, last), "prev")
	}
	if page < last {
		link(page+1, "next")
		link(last, "last")
	}
	if page > 1 {
		link(1, "first")
	}
	if len(links) > 0 {
		w.Header().Set("Link", strings.Join(links, ", "))
	}
	return out
}
