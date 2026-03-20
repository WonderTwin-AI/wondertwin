// Package pagination provides shared pagination utilities for WonderTwin twins.
// Supports cursor-based (Stripe, Slack), offset-based (QBO), and page-number
// (Xero, HubSpot) pagination patterns.
package pagination

// CursorPage is the result of cursor-based pagination.
type CursorPage[T any] struct {
	Data    []T    `json:"data"`
	HasMore bool   `json:"has_more"`
	Cursor  string `json:"cursor,omitempty"` // last item ID, pass as starting_after
	Total   int    `json:"total"`
}

// CursorPaginate returns a page of items using cursor-based pagination.
// The cursor is the ID of the last item seen. Empty cursor starts from the beginning.
// getID extracts the ID from an item for cursor comparison.
func CursorPaginate[T any](items []T, cursor string, limit int, getID func(T) string) CursorPage[T] {
	if limit <= 0 {
		limit = 10
	}

	startIdx := 0
	if cursor != "" {
		for i, item := range items {
			if getID(item) == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	endIdx := startIdx + limit
	hasMore := false
	if endIdx >= len(items) {
		endIdx = len(items)
	} else {
		hasMore = true
	}

	data := make([]T, 0, endIdx-startIdx)
	var lastCursor string
	for i := startIdx; i < endIdx; i++ {
		data = append(data, items[i])
		lastCursor = getID(items[i])
	}

	return CursorPage[T]{
		Data:    data,
		HasMore: hasMore,
		Cursor:  lastCursor,
		Total:   len(items),
	}
}

// OffsetPage is the result of offset-based pagination.
type OffsetPage[T any] struct {
	Data       []T `json:"data"`
	StartPos   int `json:"start_position"` // 1-based
	MaxResults int `json:"max_results"`
	TotalCount int `json:"total_count"`
}

// OffsetPaginate returns a page of items using offset-based pagination.
// startPos is 1-based (QBO convention). maxResults is the page size.
func OffsetPaginate[T any](items []T, startPos, maxResults int) OffsetPage[T] {
	if startPos < 1 {
		startPos = 1
	}
	if maxResults <= 0 {
		maxResults = 100
	}

	start := startPos - 1 // convert to 0-based
	if start > len(items) {
		start = len(items)
	}
	end := start + maxResults
	if end > len(items) {
		end = len(items)
	}

	data := make([]T, end-start)
	copy(data, items[start:end])

	return OffsetPage[T]{
		Data:       data,
		StartPos:   startPos,
		MaxResults: maxResults,
		TotalCount: len(items),
	}
}

// NumberPage is the result of page-number pagination.
type NumberPage[T any] struct {
	Data       []T `json:"data"`
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalPages int `json:"total_pages"`
	TotalCount int `json:"total_count"`
}

// PagePaginate returns a page of items using page-number pagination.
// page is 1-based. pageSize is the number of items per page.
func PagePaginate[T any](items []T, page, pageSize int) NumberPage[T] {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 100
	}

	totalPages := (len(items) + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}

	start := (page - 1) * pageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}

	data := make([]T, end-start)
	copy(data, items[start:end])

	return NumberPage[T]{
		Data:       data,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
		TotalCount: len(items),
	}
}
