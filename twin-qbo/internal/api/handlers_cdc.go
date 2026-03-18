package api

import (
	"net/http"
	"strings"
	"time"
)

// CDC handles the ChangeDataCapture endpoint.
// GET /v3/company/{realmId}/cdc?entities=Invoice,Payment&changedSince=2024-01-01T00:00:00Z
func (h *Handler) CDC(w http.ResponseWriter, r *http.Request) {
	entitiesParam := r.URL.Query().Get("entities")
	changedSince := r.URL.Query().Get("changedSince")

	if entitiesParam == "" {
		validationFault(w, "500", "Missing parameter", "entities parameter is required.")
		return
	}
	if changedSince == "" {
		validationFault(w, "500", "Missing parameter", "changedSince parameter is required.")
		return
	}

	since, err := time.Parse(time.RFC3339, changedSince)
	if err != nil {
		// Try alternate formats.
		since, err = time.Parse("2006-01-02T15:04:05", changedSince)
		if err != nil {
			since, err = time.Parse("2006-01-02", changedSince)
			if err != nil {
				validationFault(w, "500", "Invalid date", "changedSince must be a valid ISO 8601 date.")
				return
			}
		}
	}

	entities := strings.Split(entitiesParam, ",")
	result := map[string]any{}

	for _, entity := range entities {
		entity = strings.TrimSpace(entity)
		items := h.cdcForEntity(entity, since)
		if items != nil {
			result[entity] = items
		}
	}

	qboJSON(w, http.StatusOK, map[string]any{
		"CDCResponse": []map[string]any{{
			"QueryResponse": []map[string]any{result},
		}},
		"time": time.Now().Format(time.RFC3339),
	})
}

func (h *Handler) cdcForEntity(entity string, since time.Time) any {
	sinceStr := since.Format(time.RFC3339)

	switch strings.ToLower(entity) {
	case "customer":
		var result []any
		for _, c := range h.store.Customers.List() {
			if c.MetaData.LastUpdatedTime >= sinceStr {
				result = append(result, c)
			}
		}
		return result
	case "invoice":
		var result []any
		for _, inv := range h.store.Invoices.List() {
			if inv.MetaData.LastUpdatedTime >= sinceStr {
				result = append(result, inv)
			}
		}
		return result
	case "bill":
		var result []any
		for _, b := range h.store.Bills.List() {
			if b.MetaData.LastUpdatedTime >= sinceStr {
				result = append(result, b)
			}
		}
		return result
	case "payment":
		var result []any
		for _, p := range h.store.Payments.List() {
			if p.MetaData.LastUpdatedTime >= sinceStr {
				result = append(result, p)
			}
		}
		return result
	case "vendor":
		var result []any
		for _, v := range h.store.Vendors.List() {
			if v.MetaData.LastUpdatedTime >= sinceStr {
				result = append(result, v)
			}
		}
		return result
	case "account":
		var result []any
		for _, a := range h.store.Accounts.List() {
			if a.MetaData.LastUpdatedTime >= sinceStr {
				result = append(result, a)
			}
		}
		return result
	case "item":
		var result []any
		for _, i := range h.store.Items.List() {
			if i.MetaData.LastUpdatedTime >= sinceStr {
				result = append(result, i)
			}
		}
		return result
	default:
		return []any{}
	}
}

// filterByUpdated is unused placeholder — entity-specific filtering is done inline above.
func filterByUpdated(_ []any, _ func(any) string, _ string) []any {
	return nil
}
