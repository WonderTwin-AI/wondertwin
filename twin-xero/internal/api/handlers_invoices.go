package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger/accounting"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func (h *Handler) CreateOrUpdateInvoice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Invoices []store.Invoice `json:"Invoices"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}
	if len(req.Invoices) == 0 {
		xeroError(w, http.StatusBadRequest, "ValidationException", "At least one invoice is required")
		return
	}

	var result []store.Invoice
	for _, inv := range req.Invoices {
		processed, err := h.processInvoice(r.Context(), inv)
		if err != nil {
			xeroError(w, http.StatusBadRequest, "ValidationException", err.Error())
			return
		}
		result = append(result, processed)
	}

	xeroJSON(w, http.StatusOK, map[string]any{"Invoices": result})
}

func (h *Handler) processInvoice(ctx context.Context, inv store.Invoice) (store.Invoice, error) {
	isNew := inv.InvoiceID == ""
	if isNew {
		inv.InvoiceID = newUUID()
	}

	if inv.Type == "" {
		inv.Type = "ACCREC"
	}

	// Compute line item totals.
	var subTotal float64
	for i := range inv.LineItems {
		li := &inv.LineItems[i]
		li.LineAmount = li.Quantity * li.UnitAmount
		subTotal += li.LineAmount
	}
	inv.SubTotal = subTotal
	inv.Total = subTotal
	inv.UpdatedDateUTC = store.XeroDateNow()

	// Determine target status.
	targetStatus := inv.Status
	if targetStatus == "" {
		targetStatus = "DRAFT"
	}

	// Build engine document for state transitions.
	docType := accounting.DocTypeInvoice
	if inv.Type == "ACCPAY" {
		docType = accounting.DocTypeBill
	}

	// If updating an existing invoice, get current state.
	existing, exists := h.store.Invoices.Get(inv.InvoiceID)
	currentStatus := "DRAFT"
	if exists {
		currentStatus = existing.Status
		if inv.AmountPaid == 0 {
			inv.AmountPaid = existing.AmountPaid
		}
	}

	inv.AmountDue = inv.Total - inv.AmountPaid

	// Build engine line items.
	var engineLineItems []accounting.LineItem
	for _, li := range inv.LineItems {
		acctID := h.resolveAccountCode(li.AccountCode)
		engineLineItems = append(engineLineItems, accounting.LineItem{
			Description: li.Description,
			AccountID:   acctID,
			Quantity:    int64(li.Quantity * 100),
			UnitAmount:  int64(li.UnitAmount * 100),
			Amount:      int64(li.LineAmount * 100),
		})
	}

	doc := &accounting.Document{
		ID:        inv.InvoiceID,
		Type:      docType,
		Status:    accounting.DocumentStatus(currentStatus),
		ContactID: inv.Contact.ContactID,
		Currency:  inv.CurrencyCode,
		LineItems: engineLineItems,
		SubTotal:  int64(inv.SubTotal * 100),
		Total:     int64(inv.Total * 100),
		AmountDue: int64(inv.AmountDue * 100),
		AmountPaid: int64(inv.AmountPaid * 100),
		Date:      parseXeroDate(inv.Date),
		DueDate:   parseXeroDate(inv.DueDate),
	}
	accounting.ComputeLineItemTotals(doc)
	doc.AmountDue = doc.Total - doc.AmountPaid

	// Walk through state transitions.
	transitions := statePathTo(accounting.DocumentStatus(currentStatus), accounting.DocumentStatus(targetStatus))
	for _, to := range transitions {
		if err := h.engine.TransitionDocument(ctx, doc, to); err != nil {
			return inv, err
		}
	}

	inv.Status = string(doc.Status)
	inv.AmountDue = float64(doc.AmountDue) / 100

	h.store.Invoices.Set(inv.InvoiceID, inv)

	if isNew && h.dispatcher != nil {
		evtType := "INVOICE.CREATE"
		h.dispatcher.Enqueue(evtType, map[string]any{
			"resourceId": inv.InvoiceID,
			"eventType":  evtType,
		})
	}

	return inv, nil
}

func (h *Handler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	all := h.store.Invoices.List()
	xeroJSON(w, http.StatusOK, map[string]any{"Invoices": paginate(all, page, pageSize)})
}

func (h *Handler) GetInvoice(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "InvoiceID")
	inv, ok := h.store.Invoices.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "Invoice not found")
		return
	}
	xeroJSON(w, http.StatusOK, map[string]any{"Invoices": []store.Invoice{inv}})
}

// statePathTo returns the intermediate states to walk from→to.
func statePathTo(from, to accounting.DocumentStatus) []accounting.DocumentStatus {
	if from == to {
		return nil
	}
	path := map[accounting.DocumentStatus]map[accounting.DocumentStatus][]accounting.DocumentStatus{
		accounting.StatusDraft: {
			accounting.StatusSubmitted:  {accounting.StatusSubmitted},
			accounting.StatusAuthorised: {accounting.StatusSubmitted, accounting.StatusAuthorised},
			accounting.StatusDeleted:    {accounting.StatusDeleted},
		},
		accounting.StatusSubmitted: {
			accounting.StatusAuthorised: {accounting.StatusAuthorised},
			accounting.StatusDeleted:    {accounting.StatusDeleted},
		},
		accounting.StatusAuthorised: {
			accounting.StatusVoided: {accounting.StatusVoided},
		},
	}
	if paths, ok := path[from]; ok {
		if steps, ok := paths[to]; ok {
			return steps
		}
	}
	// Direct transition attempt (may fail in engine).
	return []accounting.DocumentStatus{to}
}

func (h *Handler) resolveAccountCode(code string) string {
	if code == "" {
		return ""
	}
	acct, ok := h.store.FindAccountByCode(code)
	if ok {
		return acct.AccountID
	}
	return code
}

func parseXeroDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02T15:04:05", s)
	if err != nil {
		t, err = time.Parse("2006-01-02", s)
		if err != nil {
			return time.Time{}
		}
	}
	return t
}
