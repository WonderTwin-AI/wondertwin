package api

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// arrayParamsMiddleware makes indexed array parameters readable under the
// bracket form. Stripe accepts both `name[]=a` and `name[0]=a`: curl examples
// in the docs send the first, and every official SDK (stripe-go, stripe-node,
// stripe-python) sends the second, because they always encode arrays indexed.
// Handlers read the `name[]` key, so without this an SDK's expand[0] or
// enabled_events[0] was silently dropped.
//
// For every key whose last bracket segment is a number (`expand[0]`,
// `items[0][tax_rates][1]`), the value is also added under the same key with
// that segment emptied (`expand[]`, `items[0][tax_rates][]`), in index order
// and after any values already sent in bracket form. The indexed keys stay in
// place, so handlers that parse `items[N][price]` directly are unaffected.
func (h *Handler) arrayParamsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			_ = r.ParseForm()
			normalizeArrayParams(r.Form)
			normalizeArrayParams(r.PostForm)
		}
		next.ServeHTTP(w, r)
	})
}

type indexedValue struct {
	index  int
	values []string
}

// normalizeArrayParams adds the bracket form of every indexed array key to v.
func normalizeArrayParams(v url.Values) {
	if v == nil {
		return
	}
	groups := map[string][]indexedValue{}
	for key, vals := range v {
		base, idx, ok := splitTrailingIndex(key)
		if !ok {
			continue
		}
		groups[base+"[]"] = append(groups[base+"[]"], indexedValue{index: idx, values: vals})
	}
	for key, entries := range groups {
		sort.Slice(entries, func(i, j int) bool { return entries[i].index < entries[j].index })
		for _, e := range entries {
			v[key] = append(v[key], e.values...)
		}
	}
}

// splitTrailingIndex splits "name[3]" into ("name", 3). It reports false when
// the key does not end in a numeric bracket segment.
func splitTrailingIndex(key string) (string, int, bool) {
	if !strings.HasSuffix(key, "]") {
		return "", 0, false
	}
	open := strings.LastIndexByte(key, '[')
	if open <= 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(key[open+1 : len(key)-1])
	if err != nil || n < 0 {
		return "", 0, false
	}
	return key[:open], n, true
}
