package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger/accounting"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func (h *Handler) CreateOrUpdateCreditNote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CreditNotes []store.CreditNote `json:"CreditNotes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}
	if len(req.CreditNotes) == 0 {
		xeroError(w, http.StatusBadRequest, "ValidationException", "At least one credit note is required")
		return
	}

	var result []store.CreditNote
	for _, cn := range req.CreditNotes {
		processed, err := h.processCreditNote(r.Context(), cn)
		if err != nil {
			xeroError(w, http.StatusBadRequest, "ValidationException", err.Error())
			return
		}
		result = append(result, processed)
	}

	xeroJSON(w, http.StatusOK, map[string]any{"CreditNotes": result})
}

func (h *Handler) processCreditNote(ctx context.Context, cn store.CreditNote) (store.CreditNote, error) {
	isNew := cn.CreditNoteID == ""
	if isNew {
		cn.CreditNoteID = newUUID()
	}

	if cn.Type == "" {
		cn.Type = "ACCRECCREDIT"
	}

	var subTotal float64
	for i := range cn.LineItems {
		li := &cn.LineItems[i]
		li.LineAmount = li.Quantity * li.UnitAmount
		subTotal += li.LineAmount
	}
	cn.SubTotal = subTotal
	cn.Total = subTotal
	cn.UpdatedDateUTC = store.XeroDateNow()

	targetStatus := cn.Status
	if targetStatus == "" {
		targetStatus = "DRAFT"
	}

	existing, exists := h.store.CreditNotes.Get(cn.CreditNoteID)
	currentStatus := "DRAFT"
	if exists {
		currentStatus = existing.Status
	}

	cn.RemainingCredit = cn.Total

	var engineLineItems []accounting.LineItem
	for _, li := range cn.LineItems {
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
		ID:        cn.CreditNoteID,
		Type:      accounting.DocTypeCreditNote,
		Status:    accounting.DocumentStatus(currentStatus),
		ContactID: cn.Contact.ContactID,
		Currency:  cn.CurrencyCode,
		LineItems: engineLineItems,
		SubTotal:  int64(cn.SubTotal * 100),
		Total:     int64(cn.Total * 100),
		AmountDue: int64(cn.Total * 100),
		Date:      parseXeroDate(cn.Date),
	}
	accounting.ComputeLineItemTotals(doc)

	transitions := statePathTo(accounting.DocumentStatus(currentStatus), accounting.DocumentStatus(targetStatus))
	for _, to := range transitions {
		if err := h.engine.TransitionDocument(ctx, doc, to); err != nil {
			return cn, err
		}
	}

	cn.Status = string(doc.Status)
	h.store.CreditNotes.Set(cn.CreditNoteID, cn)

	if isNew && h.dispatcher != nil {
		h.dispatcher.Enqueue("CREDITNOTE.CREATE", map[string]any{
			"resourceId": cn.CreditNoteID,
			"eventType":  "CREDITNOTE.CREATE",
		})
	}

	return cn, nil
}

func (h *Handler) ListCreditNotes(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	all := h.store.CreditNotes.List()
	xeroJSON(w, http.StatusOK, map[string]any{"CreditNotes": paginate(all, page, pageSize)})
}

func (h *Handler) GetCreditNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "CreditNoteID")
	cn, ok := h.store.CreditNotes.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "CreditNote not found")
		return
	}
	xeroJSON(w, http.StatusOK, map[string]any{"CreditNotes": []store.CreditNote{cn}})
}
