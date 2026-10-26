package api

import (
	"errors"
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

// transactionsSync is POST /transactions/sync: what changed in an Item's
// transactions since a cursor, in pages (quirks PAGINATION-004, LIFECYCLE-004).
// Rows are available at once, so the update status is always complete.
func (h *Handler) transactionsSync(w http.ResponseWriter, r *http.Request) {
	call, _ := CallFrom(r)
	cursor, ok1 := asString(call.Body["cursor"])
	count, hasCount, ok2 := asInt(call.Body["count"])
	_, isObj := call.Body["options"].(map[string]any)
	if !ok1 || !ok2 || (call.Body["options"] != nil && !isObj) {
		h.invalidBody(w)
		return
	}
	if !hasCount {
		count = 100
	}
	if count < 1 || count > 500 {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "INVALID_FIELD", "count must be between 1 and 500")
		return
	}
	it, ok := h.liveItemFor(w, r)
	if !ok {
		return
	}
	page, err := h.store.Sync(it.ID, cursor, count)
	if errors.Is(err, store.ErrBadCursor) {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "INVALID_FIELD", "cursor is not valid for this Item")
		return
	}
	it.SyncCalled = true
	h.store.PutItem(it)

	accounts := []accountJSON{}
	for _, a := range h.store.AccountsForItem(it.ID) {
		accounts = append(accounts, newAccountJSON(a))
	}
	removed := make([]store.RemovedTransaction, 0, len(page.Removed))
	removed = append(removed, page.Removed...)
	h.writeJSON(w, http.StatusOK, map[string]any{
		"accounts": accounts, "added": rowsOrEmpty(page.Added), "modified": rowsOrEmpty(page.Modified), "removed": removed,
		"next_cursor": page.NextCursor, "has_more": page.HasMore, "transactions_update_status": "HISTORICAL_UPDATE_COMPLETE",
	})
}

// rowsOrEmpty keeps an empty list as [], never null.
func rowsOrEmpty(rows []map[string]any) []map[string]any {
	if rows == nil {
		return []map[string]any{}
	}
	return rows
}
