package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

// --- RefundReceipt ---

func (h *Handler) CreateOrUpdateRefundReceipt(w http.ResponseWriter, r *http.Request) {
	op := r.URL.Query().Get("operation")
	var rr store.RefundReceipt
	if err := json.NewDecoder(r.Body).Decode(&rr); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if op == "delete" {
		if _, ok := h.store.RefundReceipts.Get(rr.Id); !ok { notFoundFault(w, "RefundReceipt", rr.Id); return }
		h.store.RefundReceipts.Delete(rr.Id); h.fireEvent("RefundReceipt", rr.Id, "Delete")
		qboJSON(w, http.StatusOK, entityResponse("RefundReceipt", rr)); return
	}
	if rr.Id != "" {
		existing, ok := h.store.RefundReceipts.Get(rr.Id)
		if !ok { notFoundFault(w, "RefundReceipt", rr.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, rr.SyncToken); err != nil { staleSyncTokenFault(w); return }
		rr.SyncToken = IncrementSyncToken(existing.SyncToken)
		rr.MetaData.CreateTime = existing.MetaData.CreateTime
		rr.MetaData.LastUpdatedTime = h.store.Now()
		rr.Domain = "QBO"
		h.store.RefundReceipts.Set(rr.Id, rr)
	} else {
		rr.Id = h.store.NextID(); rr.SyncToken = "0"; rr.Domain = "QBO"
		rr.MetaData = h.store.NewMetaData()
		var total float64
		for _, line := range rr.Line { total += line.Amount }
		rr.TotalAmt = total
		h.store.RefundReceipts.Set(rr.Id, rr)
	}
	qboJSON(w, http.StatusOK, entityResponse("RefundReceipt", rr))
}

func (h *Handler) GetRefundReceipt(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rr, ok := h.store.RefundReceipts.Get(id)
	if !ok { notFoundFault(w, "RefundReceipt", id); return }
	qboJSON(w, http.StatusOK, entityResponse("RefundReceipt", rr))
}

// --- PurchaseOrder ---

func (h *Handler) CreateOrUpdatePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	op := r.URL.Query().Get("operation")
	var po store.PurchaseOrder
	if err := json.NewDecoder(r.Body).Decode(&po); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if op == "delete" {
		if _, ok := h.store.PurchaseOrders.Get(po.Id); !ok { notFoundFault(w, "PurchaseOrder", po.Id); return }
		h.store.PurchaseOrders.Delete(po.Id); h.fireEvent("PurchaseOrder", po.Id, "Delete")
		qboJSON(w, http.StatusOK, entityResponse("PurchaseOrder", po)); return
	}
	if po.Id != "" {
		existing, ok := h.store.PurchaseOrders.Get(po.Id)
		if !ok { notFoundFault(w, "PurchaseOrder", po.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, po.SyncToken); err != nil { staleSyncTokenFault(w); return }
		po.SyncToken = IncrementSyncToken(existing.SyncToken)
		po.MetaData.CreateTime = existing.MetaData.CreateTime
		po.MetaData.LastUpdatedTime = h.store.Now()
		po.Domain = "QBO"
		h.store.PurchaseOrders.Set(po.Id, po)
	} else {
		po.Id = h.store.NextID(); po.SyncToken = "0"; po.Domain = "QBO"
		if po.POStatus == "" { po.POStatus = "Open" }
		po.MetaData = h.store.NewMetaData()
		var total float64
		for _, line := range po.Line { total += line.Amount }
		po.TotalAmt = total
		h.store.PurchaseOrders.Set(po.Id, po)
	}
	qboJSON(w, http.StatusOK, entityResponse("PurchaseOrder", po))
}

func (h *Handler) GetPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	po, ok := h.store.PurchaseOrders.Get(id)
	if !ok { notFoundFault(w, "PurchaseOrder", id); return }
	qboJSON(w, http.StatusOK, entityResponse("PurchaseOrder", po))
}

// --- TimeActivity ---

func (h *Handler) CreateOrUpdateTimeActivity(w http.ResponseWriter, r *http.Request) {
	op := r.URL.Query().Get("operation")
	var ta store.TimeActivity
	if err := json.NewDecoder(r.Body).Decode(&ta); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if op == "delete" {
		if _, ok := h.store.TimeActivities.Get(ta.Id); !ok { notFoundFault(w, "TimeActivity", ta.Id); return }
		h.store.TimeActivities.Delete(ta.Id)
		qboJSON(w, http.StatusOK, entityResponse("TimeActivity", ta)); return
	}
	if ta.Id != "" {
		existing, ok := h.store.TimeActivities.Get(ta.Id)
		if !ok { notFoundFault(w, "TimeActivity", ta.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, ta.SyncToken); err != nil { staleSyncTokenFault(w); return }
		ta.SyncToken = IncrementSyncToken(existing.SyncToken)
		ta.MetaData.CreateTime = existing.MetaData.CreateTime
		ta.MetaData.LastUpdatedTime = h.store.Now()
		ta.Domain = "QBO"
		h.store.TimeActivities.Set(ta.Id, ta)
	} else {
		ta.Id = h.store.NextID(); ta.SyncToken = "0"; ta.Domain = "QBO"
		ta.MetaData = h.store.NewMetaData()
		h.store.TimeActivities.Set(ta.Id, ta)
	}
	qboJSON(w, http.StatusOK, entityResponse("TimeActivity", ta))
}

func (h *Handler) GetTimeActivity(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ta, ok := h.store.TimeActivities.Get(id)
	if !ok { notFoundFault(w, "TimeActivity", id); return }
	qboJSON(w, http.StatusOK, entityResponse("TimeActivity", ta))
}
