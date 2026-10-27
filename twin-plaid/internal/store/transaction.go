package store

import (
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Transaction is one row Plaid returns. The core fields are what the emulator
// orders and filters by; Body is the whole documented object, typed so every
// field keeps the presence rule plaid-python applies (null or omitted).
type Transaction struct {
	ID        string          `json:"transaction_id"`
	ItemID    string          `json:"item_id"`
	AccountID string          `json:"account_id"`
	Date      string          `json:"date"`
	Amount    float64         `json:"amount"`
	Pending   bool            `json:"pending"`
	Body      TransactionBody `json:"body"`
}

// Change kinds in an Item's log.
const (
	Added    = "added"
	Modified = "modified"
	Removed  = "removed"
)

// Change is one entry of an Item's change log, the history /transactions/sync
// replays. It keeps the row's body as it was then, so a replay does not depend
// on what the store holds later.
type Change struct {
	Kind          string           `json:"kind"`
	TransactionID string           `json:"transaction_id"`
	AccountID     string           `json:"account_id"`
	Body          *TransactionBody `json:"body,omitempty"`
}

// RemovedTransaction is how sync reports a removal.
type RemovedTransaction struct {
	AccountID     string `json:"account_id"`
	TransactionID string `json:"transaction_id"`
}

// SyncPage is one page of /transactions/sync.
type SyncPage struct {
	Added, Modified []TransactionBody
	Removed         []RemovedTransaction
	NextCursor      string
	HasMore         bool
}

// SyncNow is the cursor value that skips an Item's history: the page is empty
// and its next_cursor is the end of the log, as Plaid documents for migrating
// an Item from /transactions/get.
const SyncNow = "now"

// ErrBadCursor is a cursor this store did not issue for this Item, or one that
// points past the end of the Item's log.
var ErrBadCursor = errors.New("bad transactions cursor")

// AddTransaction stores a new row and logs it as added.
func (s *MemoryStore) AddTransaction(t Transaction) {
	s.Transactions.Set(t.ID, t)
	s.appendChange(t.ItemID, Change{Added, t.ID, t.AccountID, &t.Body})
}

// ModifyTransaction replaces a stored row and logs it as modified. It reports
// false when there is no such row.
func (s *MemoryStore) ModifyTransaction(t Transaction) bool {
	if _, ok := s.Transactions.Get(t.ID); !ok {
		return false
	}
	s.Transactions.Set(t.ID, t)
	s.appendChange(t.ItemID, Change{Modified, t.ID, t.AccountID, &t.Body})
	return true
}

// RemoveTransaction deletes a row and logs it as removed.
func (s *MemoryStore) RemoveTransaction(id string) bool {
	t, ok := s.Transactions.Get(id)
	if !ok {
		return false
	}
	s.Transactions.Delete(id)
	s.appendChange(t.ItemID, Change{Kind: Removed, TransactionID: id, AccountID: t.AccountID})
	return true
}

func (s *MemoryStore) appendChange(itemID string, c Change) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	s.ChangeLogs[itemID] = append(s.ChangeLogs[itemID], c)
}

// TransactionsForItem returns an Item's rows, newest date first, then by id.
func (s *MemoryStore) TransactionsForItem(itemID string) []Transaction {
	out := s.Transactions.Filter(func(_ string, t Transaction) bool { return t.ItemID == itemID })
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// removeTransactions drops an Item's rows and its change log.
func (s *MemoryStore) removeTransactions(itemID string) {
	for _, t := range s.TransactionsForItem(itemID) {
		s.Transactions.Delete(t.ID)
	}
	s.logMu.Lock()
	delete(s.ChangeLogs, itemID)
	s.logMu.Unlock()
}

// cursor is a position in an Item's log: the window [start, end) of changes this
// run of pages covers, and how many of that window's changes earlier pages sent.
// Once a run has sent a page (skip above zero) its window is fixed, so a change
// that lands mid-run waits for the next run instead of shifting the pages. With
// skip at zero the cursor starts a run, and end is whatever the log holds then.
type cursor struct{ start, end, skip int }

func (c cursor) String() string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("v1:%d:%d:%d", c.start, c.end, c.skip)))
}

func parseCursor(s string, logLen int) (cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	parts := strings.Split(string(raw), ":")
	if err != nil || len(parts) != 4 || parts[0] != "v1" {
		return cursor{}, ErrBadCursor
	}
	var n [3]int
	for i := range n {
		if n[i], err = strconv.Atoi(parts[i+1]); err != nil || n[i] < 0 {
			return cursor{}, ErrBadCursor
		}
	}
	if c := (cursor{n[0], n[1], n[2]}); c.start <= c.end && c.end <= logLen {
		return c, nil
	}
	return cursor{}, ErrBadCursor
}

// Sync is /transactions/sync over an Item's log. An empty cursor starts a run
// from the beginning, which reports every row the Item has as added; a cursor
// from the end of one run starts the next, with what changed since. Inside a
// window a row added and then changed is reported once, added, with its latest
// body, and one added and then removed is not reported at all. count must be
// positive.
func (s *MemoryStore) Sync(itemID, cursorText string, count int) (SyncPage, error) {
	s.logMu.Lock()
	log := append([]Change(nil), s.ChangeLogs[itemID]...)
	s.logMu.Unlock()

	c := cursor{0, len(log), 0}
	if cursorText == SyncNow {
		return SyncPage{NextCursor: cursor{len(log), len(log), 0}.String()}, nil
	}
	if cursorText != "" {
		var err error
		if c, err = parseCursor(cursorText, len(log)); err != nil {
			return SyncPage{}, err
		}
	}

	if c.skip == 0 {
		c.end = len(log) // a cursor with nothing consumed starts a run: it covers all that has changed since
	}

	// Collapse the window into one net change per row, in first-seen order.
	var order []string
	net := map[string]Change{}
	for _, ch := range log[c.start:c.end] {
		prev, seen := net[ch.TransactionID]
		switch {
		case ch.Kind == Added:
			net[ch.TransactionID] = ch
		case ch.Kind == Modified && seen && prev.Kind == Added:
			ch.Kind = Added
			net[ch.TransactionID] = ch
		case ch.Kind == Removed && seen && prev.Kind == Added:
			delete(net, ch.TransactionID)
			continue
		default:
			net[ch.TransactionID] = ch
		}
		if !seen {
			order = append(order, ch.TransactionID)
		}
	}
	var changes []Change
	for _, kind := range []string{Added, Modified, Removed} {
		for _, id := range order {
			if ch, ok := net[id]; ok && ch.Kind == kind {
				changes = append(changes, ch)
			}
		}
	}

	rest := changes[min(c.skip, len(changes)):]
	page := rest[:min(count, len(rest))]
	out := SyncPage{HasMore: len(rest) > len(page)}
	for _, ch := range page {
		switch ch.Kind {
		case Added:
			out.Added = append(out.Added, *ch.Body)
		case Modified:
			out.Modified = append(out.Modified, *ch.Body)
		case Removed:
			out.Removed = append(out.Removed, RemovedTransaction{ch.AccountID, ch.TransactionID})
		}
	}
	if out.HasMore {
		out.NextCursor = cursor{c.start, c.end, c.skip + len(page)}.String()
	} else {
		out.NextCursor = cursor{c.end, c.end, 0}.String()
	}
	return out, nil
}

// ChangeLogLen is how many changes an Item's log holds.
func (s *MemoryStore) ChangeLogLen(itemID string) int {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	return len(s.ChangeLogs[itemID])
}

// ChangesSince counts the entries of an Item's log from a position to the end
// that added a row, and returns the log's length.
func (s *MemoryStore) ChangesSince(itemID string, from int) (added, total int) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	log := s.ChangeLogs[itemID]
	for _, c := range log[min(from, len(log)):] {
		if c.Kind == Added {
			added++
		}
	}
	return added, len(log)
}
