package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/seed"
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
	it, ok := h.liveItemWith(w, r, "transactions")
	if !ok {
		return
	}
	page, err := h.store.Sync(it.ID, cursor, count)
	if errors.Is(err, store.ErrBadCursor) {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "INVALID_FIELD", "cursor is not valid for this Item")
		return
	}
	h.store.UpdateItem(it.ID, func(i *store.Item) { i.SyncCalled = true })

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
func rowsOrEmpty(rows []store.TransactionBody) []store.TransactionBody {
	if rows == nil {
		return []store.TransactionBody{}
	}
	return rows
}

// customTransaction reads one entry of /sandbox/transactions/create. ok is false
// for a wrong type. date_transacted, date_posted, amount and description are
// required; iso_currency_code defaults to USD; account_id is optional.
func customTransaction(v any) (in seed.CustomTransaction, accountID string, missing []string, ok bool) {
	m, isObj := v.(map[string]any)
	if !isObj {
		return in, "", nil, false
	}
	var ok1, ok2, ok3, ok4, ok5 bool
	in.DateTransacted, ok1 = asString(m["date_transacted"])
	in.DatePosted, ok2 = asString(m["date_posted"])
	in.Description, ok3 = asString(m["description"])
	in.Currency, ok4 = asString(m["iso_currency_code"])
	accountID, ok5 = asString(m["account_id"])
	num, isNum := m["amount"].(json.Number)
	f, err := num.Float64()
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || (m["amount"] != nil && (!isNum || err != nil)) {
		return in, "", nil, false
	}
	in.Amount = f
	if in.Currency == "" {
		in.Currency = "USD"
	}
	for _, f := range []struct {
		field  string
		absent bool
	}{{"date_transacted", in.DateTransacted == ""}, {"date_posted", in.DatePosted == ""}, {"amount", m["amount"] == nil}, {"description", in.Description == ""}} {
		if f.absent {
			missing = append(missing, f.field)
		}
	}
	return in, accountID, missing, true
}

// sandboxTransactionsCreate is POST /sandbox/transactions/create: up to ten
// transactions added to an account, which sync then reports as added. Any
// Sandbox Item takes them. Dates run from 730 days ago to today.
func (h *Handler) sandboxTransactionsCreate(w http.ResponseWriter, r *http.Request) {
	call, _ := CallFrom(r)
	list, isList := call.Body["transactions"].([]any)
	if call.Body["transactions"] != nil && !isList {
		h.invalidBody(w)
		return
	}
	var inputs []seed.CustomTransaction
	var accountIDs []string
	for _, v := range list {
		in, accountID, missing, ok := customTransaction(v)
		if !ok {
			h.invalidBody(w)
			return
		}
		if len(missing) > 0 {
			h.writeMissing(w, missing)
			return
		}
		inputs, accountIDs = append(inputs, in), append(accountIDs, accountID)
	}
	if len(inputs) == 0 {
		h.writeMissing(w, []string{"transactions"})
		return
	}
	if len(inputs) > 10 {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "INVALID_FIELD", "transactions must hold between 1 and 10 entries")
		return
	}
	today := h.store.Clock.Now().UTC().Truncate(24 * time.Hour)
	for _, in := range inputs {
		for _, d := range []string{in.DatePosted, in.DateTransacted} {
			t, err := time.Parse("2006-01-02", d)
			if err != nil || t.After(today) || t.Before(today.AddDate(0, 0, -730)) {
				h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "INVALID_FIELD",
					"dates must be YYYY-MM-DD, today or up to 730 days in the past")
				return
			}
		}
	}
	it, ok := h.liveItemFor(w, r)
	if !ok {
		return
	}
	accounts := h.store.AccountsForItem(it.ID)
	byID := map[string]store.Account{}
	fallback := store.Account{}
	for i, a := range accounts {
		byID[a.ID] = a
		if i == 0 || a.Subtype == "checking" && fallback.Subtype != "checking" {
			fallback = a
		}
	}
	rows := make([]store.Transaction, 0, len(inputs))
	for i, in := range inputs {
		a, found := fallback, true
		if accountIDs[i] != "" {
			a, found = byID[accountIDs[i]]
		}
		if !found || a.Subtype == "student" { // student loans take no custom transactions (documented)
			h.writeError(w, http.StatusBadRequest, "INVALID_INPUT", "INVALID_ACCOUNT_ID",
				"failed to find requested account ID for requested item")
			return
		}
		rows = append(rows, seed.NewCustomTransaction(it.ID, a.ID, h.store.NewID(), in))
	}
	for _, row := range rows {
		h.store.AddTransaction(row)
	}
	h.writeJSON(w, http.StatusOK, map[string]any{})
}

// transactionsRefresh is POST /transactions/refresh: Plaid starts an extraction
// and, if it finds changes, tells the Item's webhook (quirk DELIVERY-006). Rows
// are available here at once, so the extraction has nothing to fetch; the call
// reports to the webhook what changed since the last report. A receiver gets the
// legacy DEFAULT_UPDATE (new_transactions), which Plaid keeps sending for
// compatibility, and SYNC_UPDATES_AVAILABLE only once /transactions/sync has run
// on the Item. Creating a transaction alone fires nothing. TRANSACTIONS_REMOVED
// is not sent: no route removes a transaction.
func (h *Handler) transactionsRefresh(w http.ResponseWriter, r *http.Request) {
	it, ok := h.liveItemWith(w, r, "transactions")
	if !ok {
		return
	}
	added, total := h.store.ChangesSince(it.ID, it.NotifiedChanges)
	var advanced bool
	locked, found := h.store.UpdateItem(it.ID, func(i *store.Item) {
		if total > i.NotifiedChanges {
			i.NotifiedChanges = total
			advanced = true
		}
	})
	if found && advanced {
		if added > 0 {
			h.fireWebhook(locked, "TRANSACTIONS", "DEFAULT_UPDATE", map[string]any{"new_transactions": added, "error": nil})
		}
		if locked.SyncCalled {
			h.fireWebhook(locked, "TRANSACTIONS", "SYNC_UPDATES_AVAILABLE",
				map[string]any{"initial_update_complete": true, "historical_update_complete": true})
		}
	}
	h.writeJSON(w, http.StatusOK, map[string]any{})
}

// getOptions reads options.count (1 to 500, default 100) and options.offset (0 or
// more, default 0). ok is false for a wrong type.
func getOptions(body map[string]any) (count, offset int, ok bool) {
	opts, isObj := body["options"].(map[string]any)
	if body["options"] != nil && !isObj {
		return 0, 0, false
	}
	count, hasCount, ok1 := asInt(opts["count"])
	offset, _, ok2 := asInt(opts["offset"])
	if !hasCount {
		count = 100
	}
	return count, offset, ok1 && ok2
}

// transactionsGet is POST /transactions/get: the Item's transactions dated from
// start_date to end_date, newest first, paged by options.count and a 0-based
// options.offset (quirk PAGINATION-002). total_transactions is how many match,
// whatever the page. Plaid steers new integrations to /transactions/sync.
func (h *Handler) transactionsGet(w http.ResponseWriter, r *http.Request) {
	call, _ := CallFrom(r)
	start, ok1 := asString(call.Body["start_date"])
	end, ok2 := asString(call.Body["end_date"])
	count, offset, ok3 := getOptions(call.Body)
	if !ok1 || !ok2 || !ok3 {
		h.invalidBody(w)
		return
	}
	var missing []string
	if start == "" {
		missing = append(missing, "start_date")
	}
	if end == "" {
		missing = append(missing, "end_date")
	}
	if len(missing) > 0 {
		h.writeMissing(w, missing)
		return
	}
	from, errFrom := time.Parse("2006-01-02", start)
	to, errTo := time.Parse("2006-01-02", end)
	if errFrom != nil || errTo != nil || to.Before(from) {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "INVALID_FIELD", "start_date and end_date must be YYYY-MM-DD dates, start_date first")
		return
	}
	if count < 1 || count > 500 || offset < 0 {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "INVALID_FIELD", "count must be between 1 and 500 and offset 0 or more")
		return
	}
	it, accounts, ok := h.accountsForRequest(w, r, "transactions")
	if !ok {
		return
	}
	wanted := map[string]bool{}
	accountRows := []accountJSON{}
	for _, a := range accounts {
		wanted[a.ID] = true
		accountRows = append(accountRows, newAccountJSON(a))
	}
	var matching []store.Transaction
	for _, t := range h.store.TransactionsForItem(it.ID) {
		if wanted[t.AccountID] && t.Date >= start && t.Date <= end {
			matching = append(matching, t)
		}
	}
	page := []store.TransactionBody{}
	for i := offset; i < min(offset+count, len(matching)); i++ {
		page = append(page, matching[i].Body)
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"accounts": accountRows, "transactions": page,
		"total_transactions": len(matching), "item": newItemJSON(it)})
}
