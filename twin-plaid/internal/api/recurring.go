package api

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

// How a stream is found is Plaid's own algorithm and is not published. This is
// the emulator's rule, small on purpose: rows on one account with the same
// name, the same sign (outflows are positive amounts, inflows negative) and
// the same currency form a stream when the gaps between their dates all fit one
// frequency. Three or more rows make it MATURE, two make it EARLY_DETECTION. Plaid also
// reports SEMI_MONTHLY and amount-aware streams; this does not.
type frequency struct {
	name     string
	min, max int // days between neighbouring rows
	next     func(time.Time) time.Time
}

var frequencies = []frequency{
	{"WEEKLY", 6, 8, func(t time.Time) time.Time { return t.AddDate(0, 0, 7) }},
	{"BIWEEKLY", 12, 16, func(t time.Time) time.Time { return t.AddDate(0, 0, 14) }},
	{"MONTHLY", 27, 33, func(t time.Time) time.Time { return t.AddDate(0, 1, 0) }},
	{"ANNUALLY", 355, 375, func(t time.Time) time.Time { return t.AddDate(1, 0, 0) }},
}

type streamAmount struct {
	Amount                 float64 `json:"amount"`
	ISOCurrencyCode        *string `json:"iso_currency_code"`
	UnofficialCurrencyCode *string `json:"unofficial_currency_code"`
}

// streamJSON is a recurring stream. category and category_id are required and
// null; personal_finance_category is optional and not nullable, so it is left
// out when no row carries one.
type streamJSON struct {
	AccountID               string       `json:"account_id"`
	StreamID                string       `json:"stream_id"`
	Category                any          `json:"category"`
	CategoryID              any          `json:"category_id"`
	Description             string       `json:"description"`
	MerchantName            *string      `json:"merchant_name"`
	FirstDate               string       `json:"first_date"`
	LastDate                string       `json:"last_date"`
	Frequency               string       `json:"frequency"`
	TransactionIDs          []string     `json:"transaction_ids"`
	AverageAmount           streamAmount `json:"average_amount"`
	LastAmount              streamAmount `json:"last_amount"`
	IsActive                bool         `json:"is_active"`
	Status                  string       `json:"status"`
	IsUserModified          bool         `json:"is_user_modified"`
	PredictedNextDate       *string      `json:"predicted_next_date"`
	PersonalFinanceCategory any          `json:"personal_finance_category,omitempty"`
}

// fit returns the frequency whose range holds every gap between the sorted dates.
func fit(dates []time.Time) (frequency, bool) {
	for _, f := range frequencies {
		ok := true
		for i := 1; i < len(dates); i++ {
			if gap := int(dates[i].Sub(dates[i-1]).Hours() / 24); gap < f.min || gap > f.max {
				ok = false
			}
		}
		if ok {
			return f, true
		}
	}
	return frequency{}, false
}

// streamsOf finds the streams in rows, which are one Item's rows on the wanted accounts.
func streamsOf(rows []store.Transaction, now time.Time) (inflow, outflow []streamJSON) {
	groups := map[string][]store.Transaction{}
	for _, t := range rows {
		name, cur := t.Body.Name, deref(t.Body.IsoCurrencyCode)
		key := strings.Join([]string{t.AccountID, strings.Join(words(name), " "), cur, map[bool]string{true: "out", false: "in"}[t.Amount >= 0]}, "|")
		groups[key] = append(groups[key], t)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		g := groups[key]
		sort.Slice(g, func(i, j int) bool { return g[i].Date < g[j].Date })
		dates := make([]time.Time, len(g))
		for i, t := range g {
			dates[i], _ = time.Parse("2006-01-02", t.Date)
		}
		f, ok := fit(dates)
		if len(g) < 2 || !ok {
			continue
		}
		var sum float64
		ids := make([]string, len(g))
		for i, t := range g {
			sum, ids[i] = sum+t.Amount, t.ID
		}
		last := g[len(g)-1]
		cur, name := deref(last.Body.IsoCurrencyCode), last.Body.Name
		amount := func(v float64) streamAmount { return streamAmount{math.Round(v*100) / 100, strPtr(cur), nil} }
		sum256 := sha256.Sum256([]byte(key))
		next := f.next(dates[len(dates)-1]).Format("2006-01-02")
		s := streamJSON{AccountID: g[0].AccountID, StreamID: hex.EncodeToString(sum256[:])[:37], Description: name, FirstDate: g[0].Date,
			LastDate: last.Date, Frequency: f.name, TransactionIDs: ids, AverageAmount: amount(sum / float64(len(g))),
			LastAmount: amount(last.Amount), Status: "MATURE", PredictedNextDate: &next,
			IsActive: now.Sub(dates[len(dates)-1]).Hours()/24 <= 1.5*float64(f.max)}
		if last.Body.MerchantName.V != nil && *last.Body.MerchantName.V != "" {
			s.MerchantName = last.Body.MerchantName.V
		}
		if len(g) == 2 {
			s.Status = "EARLY_DETECTION"
		}
		if last.Body.PersonalFinanceCategory.Set {
			s.PersonalFinanceCategory = last.Body.PersonalFinanceCategory.V
		}
		if last.Amount >= 0 {
			outflow = append(outflow, s)
		} else {
			inflow = append(inflow, s)
		}
	}
	return inflow, outflow
}

// transactionsRecurringGet is POST /transactions/recurring/get: the recurring
// inflow and outflow streams on the Item's accounts, or on the
// account_ids named at the top level of the request (INVALID_ACCOUNT_ID for one the Item lacks).
func (h *Handler) transactionsRecurringGet(w http.ResponseWriter, r *http.Request) {
	call, _ := CallFrom(r)
	ids, ok := asStrings(call.Body["account_ids"])
	if !ok {
		h.invalidBody(w)
		return
	}
	it, accounts, ok := h.accountsForRequestIDs(w, r, ids)
	if !ok {
		return
	}
	var rows []store.Transaction
	for _, t := range h.store.TransactionsForItem(it.ID) {
		if slices.ContainsFunc(accounts, func(a store.Account) bool { return a.ID == t.AccountID }) {
			rows = append(rows, t)
		}
	}
	now := h.store.Clock.Now().UTC()
	inflow, outflow := streamsOf(rows, now)
	if inflow == nil {
		inflow = []streamJSON{}
	}
	if outflow == nil {
		outflow = []streamJSON{}
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"inflow_streams": inflow, "outflow_streams": outflow,
		"updated_datetime": rfc3339(now), "personal_finance_category_version": "v2"})
}

// deref is the string a pointer holds, or "" for nil.
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
