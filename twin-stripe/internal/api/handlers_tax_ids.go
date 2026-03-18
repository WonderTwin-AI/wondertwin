package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

// --- Tax IDs ---

func (h *Handler) CreateTaxID(w http.ResponseWriter, r *http.Request) {
	customerID := chi.URLParam(r, "customer_id")
	if _, ok := h.store.Customers.Get(customerID); !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such customer: "+customerID)
		return
	}

	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	taxType := r.FormValue("type")
	value := r.FormValue("value")
	if taxType == "" || value == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required params: type, value.")
		return
	}

	id := h.store.TaxIDs.NextID()
	taxID := store.TaxID{
		ID:       id,
		Object:   "tax_id",
		Customer: customerID,
		Type:     taxType,
		Value:    value,
		Verification: &store.TaxIDVerification{
			Status: "pending",
		},
		Livemode: false,
		Created:  store.Now(),
	}

	h.store.TaxIDs.Set(id, taxID)
	h.dispatcher.Enqueue("customer.tax_id.created", mapFromJSON(taxID))
	twincore.JSON(w, http.StatusOK, taxID)
}

func (h *Handler) GetCustomerTaxID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	taxID, ok := h.store.TaxIDs.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such tax_id: "+id)
		return
	}
	customerID := chi.URLParam(r, "customer_id")
	if taxID.Customer != customerID {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such tax_id: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, taxID)
}

func (h *Handler) DeleteTaxID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	taxID, ok := h.store.TaxIDs.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such tax_id: "+id)
		return
	}
	customerID := chi.URLParam(r, "customer_id")
	if taxID.Customer != customerID {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such tax_id: "+id)
		return
	}
	h.store.TaxIDs.Delete(id)
	h.dispatcher.Enqueue("customer.tax_id.deleted", map[string]any{"id": id, "customer": customerID})
	twincore.JSON(w, http.StatusOK, map[string]any{"id": id, "object": "tax_id", "deleted": true})
}

func (h *Handler) ListCustomerTaxIDs(w http.ResponseWriter, r *http.Request) {
	customerID := chi.URLParam(r, "customer_id")
	filtered := h.store.TaxIDs.Filter(func(_ string, t store.TaxID) bool {
		return t.Customer == customerID
	})
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/customers/" + customerID + "/tax_ids", "has_more": false, "data": filtered,
	})
}

func (h *Handler) GetTaxID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	taxID, ok := h.store.TaxIDs.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such tax_id: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, taxID)
}

func (h *Handler) ListTaxIDs(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.TaxIDs.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/tax_ids", "has_more": page.HasMore, "data": page.Data,
	})
}
