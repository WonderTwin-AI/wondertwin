package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func (h *Handler) CreatePrepayment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prepayments []store.Prepayment `json:"Prepayments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}

	var result []store.Prepayment
	for _, pp := range req.Prepayments {
		if pp.PrepaymentID == "" {
			pp.PrepaymentID = newUUID()
		}
		if pp.Status == "" {
			pp.Status = "AUTHORISED"
		}
		var subTotal float64
		for i := range pp.LineItems {
			li := &pp.LineItems[i]
			li.LineAmount = li.Quantity * li.UnitAmount
			subTotal += li.LineAmount
		}
		pp.SubTotal = subTotal
		pp.Total = subTotal
		pp.RemainingCredit = subTotal
		pp.UpdatedDateUTC = store.XeroDateNow()
		h.store.Prepayments.Set(pp.PrepaymentID, pp)
		result = append(result, pp)
	}
	xeroJSON(w, http.StatusOK, map[string]any{"Prepayments": result})
}

func (h *Handler) ListPrepayments(w http.ResponseWriter, r *http.Request) {
	all := h.store.Prepayments.List()
	xeroJSON(w, http.StatusOK, map[string]any{"Prepayments": all})
}

func (h *Handler) GetPrepayment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "PrepaymentID")
	pp, ok := h.store.Prepayments.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "Prepayment not found")
		return
	}
	xeroJSON(w, http.StatusOK, map[string]any{"Prepayments": []store.Prepayment{pp}})
}

func (h *Handler) CreateOverpayment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Overpayments []store.Overpayment `json:"Overpayments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}

	var result []store.Overpayment
	for _, op := range req.Overpayments {
		if op.OverpaymentID == "" {
			op.OverpaymentID = newUUID()
		}
		if op.Status == "" {
			op.Status = "AUTHORISED"
		}
		var subTotal float64
		for i := range op.LineItems {
			li := &op.LineItems[i]
			li.LineAmount = li.Quantity * li.UnitAmount
			subTotal += li.LineAmount
		}
		op.SubTotal = subTotal
		op.Total = subTotal
		op.RemainingCredit = subTotal
		op.UpdatedDateUTC = store.XeroDateNow()
		h.store.Overpayments.Set(op.OverpaymentID, op)
		result = append(result, op)
	}
	xeroJSON(w, http.StatusOK, map[string]any{"Overpayments": result})
}

func (h *Handler) ListOverpayments(w http.ResponseWriter, r *http.Request) {
	all := h.store.Overpayments.List()
	xeroJSON(w, http.StatusOK, map[string]any{"Overpayments": all})
}

func (h *Handler) GetOverpayment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "OverpaymentID")
	op, ok := h.store.Overpayments.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "Overpayment not found")
		return
	}
	xeroJSON(w, http.StatusOK, map[string]any{"Overpayments": []store.Overpayment{op}})
}
