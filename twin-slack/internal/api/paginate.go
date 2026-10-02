package api

import (
	"encoding/base64"
	"errors"
	"sort"
	"strings"
)

// Slack pages a collection with a cursor (docs.slack.dev apis/web-api/pagination):
// a method takes cursor and limit, returns a portion of the results, and puts a
// next_cursor in response_metadata that is empty on the last page. A cursor that
// does not compute is invalid_cursor, and an invalid limit is adjusted to
// something sensible and is never an error.

// pageSpec says how one collection pages.
type pageSpec struct {
	// kind names the collection. A cursor carries it, so a cursor from another
	// method is invalid_cursor.
	kind string
	// def is the number of items returned when no limit is given. Zero means the
	// whole collection.
	def int
	// max is the most items one call may return.
	max int
	// desc says the collection is returned in descending key order, newest
	// first, as a channel's history is.
	desc bool
}

// Page sizes documented on each method's page.
var (
	pageConversationsList    = pageSpec{kind: "channel", def: 100, max: 999}
	pageConversationsHistory = pageSpec{kind: "ts", def: 100, max: 999, desc: true}
	pageConversationsReplies = pageSpec{kind: "ts", def: 1000, max: 1000}
	pageConversationsMembers = pageSpec{kind: "user", def: 100, max: 1000}
	pageUsersConversations   = pageSpec{kind: "channel", def: 100, max: 999}
	// users.list returns every user when no limit is given.
	pageUsersList = pageSpec{kind: "user", def: 0, max: 1000}
)

var errInvalidCursor = errors.New("invalid_cursor")

// clampLimit adjusts a requested limit. Slack does not reject an unusable limit,
// it adjusts it: a missing, zero or negative one becomes the default, and one
// above the maximum becomes the maximum.
func (p pageSpec) clampLimit(limit int) int {
	if limit <= 0 {
		return p.def
	}
	if p.max > 0 && limit > p.max {
		return p.max
	}
	return limit
}

// encodeCursor builds an opaque cursor that points just past key. Slack's
// cursors are base64 text that usually ends in "=", and ours do too.
func encodeCursor(kind, key string) string {
	return base64.StdEncoding.EncodeToString([]byte(kind + ":" + key))
}

// decodeCursor recovers the key a cursor points past. It reports false for a
// cursor that is not base64, was issued for another kind of collection, or has
// no key.
func decodeCursor(kind, cursor string) (string, bool) {
	raw, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return "", false
	}
	k, key, found := strings.Cut(string(raw), ":")
	if !found || k != kind || key == "" {
		return "", false
	}
	return key, true
}

// pageOf returns one page of items and the cursor for the next, which is empty
// on the last page. Items are ordered by key, ascending or descending as the
// spec says, so a page is stable when items are added or removed between calls:
// a cursor points past a key, not at a position, and still works after the item
// it came from is gone. The page is never nil, so it encodes as [] when empty.
func pageOf[T any](items []T, key func(T) string, spec pageSpec, cursor string, limit int) ([]T, string, error) {
	ordered := make([]T, len(items))
	copy(ordered, items)
	sort.SliceStable(ordered, func(i, j int) bool {
		if spec.desc {
			return key(ordered[i]) > key(ordered[j])
		}
		return key(ordered[i]) < key(ordered[j])
	})

	start := 0
	if cursor != "" {
		past, ok := decodeCursor(spec.kind, cursor)
		if !ok {
			return nil, "", errInvalidCursor
		}
		start = sort.Search(len(ordered), func(i int) bool {
			if spec.desc {
				return key(ordered[i]) < past
			}
			return key(ordered[i]) > past
		})
	}

	end := len(ordered)
	if n := spec.clampLimit(limit); n > 0 && start+n < end {
		end = start + n
	}
	page := append([]T{}, ordered[start:end]...)

	next := ""
	if end < len(ordered) && len(page) > 0 {
		next = encodeCursor(spec.kind, key(page[len(page)-1]))
	}
	return page, next, nil
}
