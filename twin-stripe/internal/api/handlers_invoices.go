package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

func (h *Handler) CreateInvoice(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	customer := r.FormValue("customer")
	if customer == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required param: customer.")
		return
	}

	id := h.store.Invoices.NextID()
	inv := store.Invoice{
		ID:               id,
		Object:           "invoice",
		Customer:         customer,
		Status:           "draft",
		Currency:         r.FormValue("currency"),
		CollectionMethod: "charge_automatically",
		Number:           "INV-" + id,
		Livemode:         false,
		Metadata:         parseMetadata(r),
		Created:          store.Now(),
	}
	if inv.Currency == "" {
		inv.Currency = "usd"
	}
	if cm := r.FormValue("collection_method"); cm != "" {
		inv.CollectionMethod = cm
	}

	// Attach pending invoice items for this customer.
	pendingItems := h.store.InvoiceItems.Filter(func(_ string, ii store.InvoiceItem) bool {
		return ii.Customer == customer && ii.Invoice == ""
	})
	var lines []store.InvoiceLine
	var total int64
	for i, ii := range pendingItems {
		lines = append(lines, store.InvoiceLine{
			ID:          id + "_il_" + strconv.Itoa(i),
			Object:      "line_item",
			Amount:      ii.Amount,
			Currency:    ii.Currency,
			Description: ii.Description,
			Quantity:    ii.Quantity,
			Type:        "invoiceitem",
		})
		total += ii.Amount
		// Mark as attached.
		ii.Invoice = id
		h.store.InvoiceItems.Set(ii.ID, ii)
	}

	inv.Lines = &store.InvoiceLines{Object: "list", Data: lines, URL: "/v1/invoices/" + id + "/lines"}
	inv.Total = total
	inv.Subtotal = total
	inv.AmountDue = total
	inv.AmountRemaining = total

	h.store.Invoices.Set(id, inv)
	h.emitEvent("invoice.created", mapFromJSON(inv))
	twincore.JSON(w, http.StatusOK, inv)
}

func (h *Handler) GetInvoice(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	inv, ok := h.store.Invoices.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such invoice: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, inv)
}

func (h *Handler) FinalizeInvoice(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	inv, ok := h.store.Invoices.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such invoice: "+id)
		return
	}
	if inv.Status != "draft" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "invoice_not_draft", "Invoice is not a draft.")
		return
	}
	inv.Status = "open"
	h.store.Invoices.Set(id, inv)
	h.emitEvent("invoice.finalized", mapFromJSON(inv))
	twincore.JSON(w, http.StatusOK, inv)
}

func (h *Handler) PayInvoice(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	inv, ok := h.store.Invoices.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such invoice: "+id)
		return
	}
	if inv.Status != "open" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "invoice_not_open", "Invoice is not open.")
		return
	}

	inv.Status = "paid"
	inv.Paid = true
	inv.AmountPaid = inv.Total
	inv.AmountRemaining = 0
	inv.AmountDue = 0

	h.store.Invoices.Set(id, inv)
	h.emitEvent("invoice.paid", mapFromJSON(inv))
	twincore.JSON(w, http.StatusOK, inv)
}

func (h *Handler) VoidInvoice(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	inv, ok := h.store.Invoices.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such invoice: "+id)
		return
	}
	if inv.Status != "open" && inv.Status != "draft" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "invoice_not_voidable", "Invoice cannot be voided.")
		return
	}
	inv.Status = "void"
	h.store.Invoices.Set(id, inv)
	h.emitEvent("invoice.voided", mapFromJSON(inv))
	twincore.JSON(w, http.StatusOK, inv)
}

func (h *Handler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	customerFilter := r.URL.Query().Get("customer")
	subFilter := r.URL.Query().Get("subscription")

	if customerFilter != "" || subFilter != "" {
		items := h.store.Invoices.Filter(func(_ string, inv store.Invoice) bool {
			if customerFilter != "" && inv.Customer != customerFilter {
				return false
			}
			if subFilter != "" && inv.Subscription != subFilter {
				return false
			}
			return true
		})
		twincore.JSON(w, http.StatusOK, map[string]any{
			"object": "list", "url": "/v1/invoices", "has_more": false, "data": items,
		})
		return
	}

	page := h.store.Invoices.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/invoices", "has_more": page.HasMore, "data": page.Data,
	})
}

// --- Invoice Items ---

func (h *Handler) CreateInvoiceItem(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	customer := r.FormValue("customer")
	if customer == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required param: customer.")
		return
	}

	id := h.store.InvoiceItems.NextID()
	ii := store.InvoiceItem{
		ID:          id,
		Object:      "invoiceitem",
		Customer:    customer,
		Invoice:     r.FormValue("invoice"),
		Price:       r.FormValue("price"),
		Description: r.FormValue("description"),
		Currency:    r.FormValue("currency"),
		Quantity:    1,
		Livemode:    false,
		Metadata:    parseMetadata(r),
		Created:     store.Now(),
	}
	if ii.Currency == "" {
		ii.Currency = "usd"
	}
	if v := r.FormValue("amount"); v != "" {
		ii.Amount, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := r.FormValue("unit_amount"); v != "" {
		ii.UnitAmount, _ = strconv.ParseInt(v, 10, 64)
		if ii.Amount == 0 {
			ii.Amount = ii.UnitAmount * ii.Quantity
		}
	}
	if v := r.FormValue("quantity"); v != "" {
		ii.Quantity, _ = strconv.ParseInt(v, 10, 64)
		if ii.UnitAmount > 0 {
			ii.Amount = ii.UnitAmount * ii.Quantity
		}
	}

	h.store.InvoiceItems.Set(id, ii)
	twincore.JSON(w, http.StatusOK, ii)
}

func (h *Handler) GetInvoiceItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ii, ok := h.store.InvoiceItems.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such invoiceitem: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, ii)
}

func (h *Handler) DeleteInvoiceItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.InvoiceItems.Delete(id) {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such invoiceitem: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]any{"id": id, "object": "invoiceitem", "deleted": true})
}
