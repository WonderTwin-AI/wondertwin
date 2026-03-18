package api

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ParsedQuery represents a parsed QBO SQL-like query.
type ParsedQuery struct {
	Entity        string
	WhereField    string
	WhereOp       string
	WhereValue    string
	OrderBy       string
	OrderDir      string // ASC or DESC
	StartPosition int
	MaxResults    int
	IsCount       bool
}

var (
	selectRe    = regexp.MustCompile(`(?i)^SELECT\s+(\*|COUNT\(\*\))\s+FROM\s+(\w+)`)
	whereRe     = regexp.MustCompile(`(?i)WHERE\s+(\w+)\s*(=|<|>|<=|>=|!=|LIKE|IN)\s*'([^']*)'`)
	orderRe     = regexp.MustCompile(`(?i)ORDERBY\s+(\w+)\s*(ASC|DESC)?`)
	startPosRe  = regexp.MustCompile(`(?i)STARTPOSITION\s+(\d+)`)
	maxResultsRe = regexp.MustCompile(`(?i)MAXRESULTS\s+(\d+)`)
)

// ParseQuery parses a QBO SQL-like query string.
func ParseQuery(q string) (*ParsedQuery, error) {
	q = strings.TrimSpace(q)

	m := selectRe.FindStringSubmatch(q)
	if m == nil {
		return nil, fmt.Errorf("invalid query syntax: must start with SELECT * FROM or SELECT COUNT(*) FROM")
	}

	pq := &ParsedQuery{
		Entity:        m[2],
		StartPosition: 1,
		MaxResults:    100,
		IsCount:       strings.ToUpper(m[1]) == "COUNT(*)",
	}

	if wm := whereRe.FindStringSubmatch(q); wm != nil {
		pq.WhereField = wm[1]
		pq.WhereOp = strings.ToUpper(wm[2])
		pq.WhereValue = wm[3]
	}

	if om := orderRe.FindStringSubmatch(q); om != nil {
		pq.OrderBy = om[1]
		if len(om) > 2 && om[2] != "" {
			pq.OrderDir = strings.ToUpper(om[2])
		} else {
			pq.OrderDir = "ASC"
		}
	}

	if sm := startPosRe.FindStringSubmatch(q); sm != nil {
		if v, err := strconv.Atoi(sm[1]); err == nil && v > 0 {
			pq.StartPosition = v
		}
	}

	if mm := maxResultsRe.FindStringSubmatch(q); mm != nil {
		if v, err := strconv.Atoi(mm[1]); err == nil && v > 0 {
			pq.MaxResults = v
			if pq.MaxResults > 1000 {
				pq.MaxResults = 1000
			}
		}
	}

	return pq, nil
}

// Paginate applies STARTPOSITION and MAXRESULTS to a slice.
func Paginate[T any](items []T, start, max int) []T {
	if start < 1 {
		start = 1
	}
	offset := start - 1 // 1-based to 0-based
	if offset >= len(items) {
		return []T{}
	}
	end := offset + max
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}
