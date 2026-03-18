package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

func (h *Handler) CreateCustomer(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	id := h.store.Customers.NextID()
	cus := store.Customer{
		ID:          id,
		Object:      "customer",
		Name:        r.FormValue("name"),
		Email:       r.FormValue("email"),
		Phone:       r.FormValue("phone"),
		Description: r.FormValue("description"),
		Livemode:    false,
		Created:     store.Now(),
	}
	cus.Metadata = parseMetadata(r)

	h.store.Customers.Set(id, cus)
	h.emitEvent("customer.created", mapFromJSON(cus))
	twincore.JSON(w, http.StatusOK, cus)
}

func (h *Handler) GetCustomer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cus, ok := h.store.Customers.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such customer: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, cus)
}

func (h *Handler) UpdateCustomer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cus, ok := h.store.Customers.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such customer: "+id)
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if v := r.FormValue("name"); v != "" {
		cus.Name = v
	}
	if v := r.FormValue("email"); v != "" {
		cus.Email = v
	}
	if v := r.FormValue("phone"); v != "" {
		cus.Phone = v
	}
	if v := r.FormValue("description"); v != "" {
		cus.Description = v
	}
	if meta := parseMetadata(r); len(meta) > 0 {
		cus.Metadata = meta
	}

	h.store.Customers.Set(id, cus)
	h.emitEvent("customer.updated", mapFromJSON(cus))
	twincore.JSON(w, http.StatusOK, cus)
}

func (h *Handler) DeleteCustomer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.Customers.Delete(id) {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such customer: "+id)
		return
	}
	h.emitEvent("customer.deleted", map[string]any{"id": id})
	twincore.JSON(w, http.StatusOK, map[string]any{"id": id, "object": "customer", "deleted": true})
}

func (h *Handler) ListCustomers(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.Customers.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/customers",
		"has_more": page.HasMore,
		"data":     page.Data,
	})
}

// parseMetadata extracts metadata[key]=value from form data.
func parseMetadata(r *http.Request) map[string]string {
	meta := map[string]string{}
	for key, vals := range r.Form {
		if len(key) > 9 && key[:9] == "metadata[" && key[len(key)-1] == ']' {
			meta[key[9:len(key)-1]] = vals[0]
		}
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

// parseLimit extracts the limit query parameter.
func parseLimit(r *http.Request, defaultLimit int) int {
	if v := r.URL.Query().Get("limit"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 && n <= 100 {
			return n
		}
	}
	return defaultLimit
}

// mapFromJSON marshals a struct to map[string]any for event payloads.
func mapFromJSON(v any) map[string]any {
	data, _ := json.Marshal(v)
	var m map[string]any
	json.Unmarshal(data, &m)
	return m
}
