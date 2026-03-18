package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

// --- Credit Memo ---

func (h *Handler) CreateOrUpdateCreditMemo(w http.ResponseWriter, r *http.Request) {
	var cm store.CreditMemo
	if err := json.NewDecoder(r.Body).Decode(&cm); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if cm.Id != "" {
		existing, ok := h.store.CreditMemos.Get(cm.Id)
		if !ok { notFoundFault(w, "CreditMemo", cm.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, cm.SyncToken); err != nil { staleSyncTokenFault(w); return }
		cm.SyncToken = IncrementSyncToken(existing.SyncToken)
		cm.MetaData.CreateTime = existing.MetaData.CreateTime
		cm.MetaData.LastUpdatedTime = h.store.Now()
		cm.Domain = "QBO"
		h.store.CreditMemos.Set(cm.Id, cm)
	} else {
		cm.Id = h.store.NextID()
		cm.SyncToken = "0"
		cm.Domain = "QBO"
		cm.MetaData = h.store.NewMetaData()
		computeCreditMemoTotals(&cm)
		cm.RemainingCredit = cm.TotalAmt
		h.store.CreditMemos.Set(cm.Id, cm)
		h.fireEvent("CreditMemo", cm.Id, "Create")
	}
	qboJSON(w, http.StatusOK, entityResponse("CreditMemo", cm))
}

func (h *Handler) GetCreditMemo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cm, ok := h.store.CreditMemos.Get(id)
	if !ok { notFoundFault(w, "CreditMemo", id); return }
	qboJSON(w, http.StatusOK, entityResponse("CreditMemo", cm))
}

func computeCreditMemoTotals(cm *store.CreditMemo) {
	var total float64
	for _, line := range cm.Line { total += line.Amount }
	cm.TotalAmt = total
}

// --- Vendor Credit ---

func (h *Handler) CreateOrUpdateVendorCredit(w http.ResponseWriter, r *http.Request) {
	var vc store.VendorCredit
	if err := json.NewDecoder(r.Body).Decode(&vc); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if vc.Id != "" {
		existing, ok := h.store.VendorCredits.Get(vc.Id)
		if !ok { notFoundFault(w, "VendorCredit", vc.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, vc.SyncToken); err != nil { staleSyncTokenFault(w); return }
		vc.SyncToken = IncrementSyncToken(existing.SyncToken)
		vc.MetaData.CreateTime = existing.MetaData.CreateTime
		vc.MetaData.LastUpdatedTime = h.store.Now()
		vc.Domain = "QBO"
		h.store.VendorCredits.Set(vc.Id, vc)
	} else {
		vc.Id = h.store.NextID()
		vc.SyncToken = "0"
		vc.Domain = "QBO"
		vc.MetaData = h.store.NewMetaData()
		var total float64
		for _, line := range vc.Line { total += line.Amount }
		vc.TotalAmt = total
		vc.Balance = total
		h.store.VendorCredits.Set(vc.Id, vc)
	}
	qboJSON(w, http.StatusOK, entityResponse("VendorCredit", vc))
}

func (h *Handler) GetVendorCredit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	vc, ok := h.store.VendorCredits.Get(id)
	if !ok { notFoundFault(w, "VendorCredit", id); return }
	qboJSON(w, http.StatusOK, entityResponse("VendorCredit", vc))
}

// --- Sales Receipt ---

func (h *Handler) CreateOrUpdateSalesReceipt(w http.ResponseWriter, r *http.Request) {
	var sr store.SalesReceipt
	if err := json.NewDecoder(r.Body).Decode(&sr); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if sr.Id != "" {
		existing, ok := h.store.SalesReceipts.Get(sr.Id)
		if !ok { notFoundFault(w, "SalesReceipt", sr.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, sr.SyncToken); err != nil { staleSyncTokenFault(w); return }
		sr.SyncToken = IncrementSyncToken(existing.SyncToken)
		sr.MetaData.CreateTime = existing.MetaData.CreateTime
		sr.MetaData.LastUpdatedTime = h.store.Now()
		sr.Domain = "QBO"
		var total float64
		for _, line := range sr.Line { total += line.Amount }
		sr.TotalAmt = total
		h.store.SalesReceipts.Set(sr.Id, sr)
	} else {
		sr.Id = h.store.NextID()
		sr.SyncToken = "0"
		sr.Domain = "QBO"
		sr.MetaData = h.store.NewMetaData()
		var total float64
		for _, line := range sr.Line { total += line.Amount }
		sr.TotalAmt = total
		h.store.SalesReceipts.Set(sr.Id, sr)
		h.journalSalesReceiptCreated(&sr)
	}
	qboJSON(w, http.StatusOK, entityResponse("SalesReceipt", sr))
}

func (h *Handler) GetSalesReceipt(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sr, ok := h.store.SalesReceipts.Get(id)
	if !ok { notFoundFault(w, "SalesReceipt", id); return }
	qboJSON(w, http.StatusOK, entityResponse("SalesReceipt", sr))
}

// --- Deposit ---

func (h *Handler) CreateOrUpdateDeposit(w http.ResponseWriter, r *http.Request) {
	var dep store.Deposit
	if err := json.NewDecoder(r.Body).Decode(&dep); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if dep.Id != "" {
		existing, ok := h.store.Deposits.Get(dep.Id)
		if !ok { notFoundFault(w, "Deposit", dep.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, dep.SyncToken); err != nil { staleSyncTokenFault(w); return }
		dep.SyncToken = IncrementSyncToken(existing.SyncToken)
		dep.MetaData.CreateTime = existing.MetaData.CreateTime
		dep.MetaData.LastUpdatedTime = h.store.Now()
		dep.Domain = "QBO"
		h.store.Deposits.Set(dep.Id, dep)
	} else {
		dep.Id = h.store.NextID()
		dep.SyncToken = "0"
		dep.Domain = "QBO"
		dep.MetaData = h.store.NewMetaData()
		var total float64
		for _, line := range dep.Line { total += line.Amount }
		dep.TotalAmt = total
		h.store.Deposits.Set(dep.Id, dep)
		h.journalDepositCreated(&dep)
	}
	qboJSON(w, http.StatusOK, entityResponse("Deposit", dep))
}

func (h *Handler) GetDeposit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	dep, ok := h.store.Deposits.Get(id)
	if !ok { notFoundFault(w, "Deposit", id); return }
	qboJSON(w, http.StatusOK, entityResponse("Deposit", dep))
}

// --- Transfer ---

func (h *Handler) CreateOrUpdateTransfer(w http.ResponseWriter, r *http.Request) {
	var xfer store.Transfer
	if err := json.NewDecoder(r.Body).Decode(&xfer); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if xfer.Id != "" {
		existing, ok := h.store.Transfers.Get(xfer.Id)
		if !ok { notFoundFault(w, "Transfer", xfer.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, xfer.SyncToken); err != nil { staleSyncTokenFault(w); return }
		xfer.SyncToken = IncrementSyncToken(existing.SyncToken)
		xfer.MetaData.CreateTime = existing.MetaData.CreateTime
		xfer.MetaData.LastUpdatedTime = h.store.Now()
		xfer.Domain = "QBO"
		h.store.Transfers.Set(xfer.Id, xfer)
	} else {
		xfer.Id = h.store.NextID()
		xfer.SyncToken = "0"
		xfer.Domain = "QBO"
		xfer.MetaData = h.store.NewMetaData()
		h.store.Transfers.Set(xfer.Id, xfer)
		h.journalTransferCreated(&xfer)
	}
	qboJSON(w, http.StatusOK, entityResponse("Transfer", xfer))
}

func (h *Handler) GetTransfer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	xfer, ok := h.store.Transfers.Get(id)
	if !ok { notFoundFault(w, "Transfer", id); return }
	qboJSON(w, http.StatusOK, entityResponse("Transfer", xfer))
}

// --- Journal Entry ---

func (h *Handler) CreateOrUpdateJournalEntry(w http.ResponseWriter, r *http.Request) {
	var je store.JournalEntry
	if err := json.NewDecoder(r.Body).Decode(&je); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if je.Id != "" {
		existing, ok := h.store.JournalEntries.Get(je.Id)
		if !ok { notFoundFault(w, "JournalEntry", je.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, je.SyncToken); err != nil { staleSyncTokenFault(w); return }
		je.SyncToken = IncrementSyncToken(existing.SyncToken)
		je.MetaData.CreateTime = existing.MetaData.CreateTime
		je.MetaData.LastUpdatedTime = h.store.Now()
		je.Domain = "QBO"
		h.store.JournalEntries.Set(je.Id, je)
	} else {
		je.Id = h.store.NextID()
		je.SyncToken = "0"
		je.Domain = "QBO"
		je.MetaData = h.store.NewMetaData()
		var total float64
		for _, line := range je.Line {
			if line.JournalEntryLineDetail != nil && line.JournalEntryLineDetail.PostingType == "Debit" {
				total += line.Amount
			}
		}
		je.TotalAmt = total
		h.store.JournalEntries.Set(je.Id, je)
		h.journalJournalEntryCreated(&je)
	}
	qboJSON(w, http.StatusOK, entityResponse("JournalEntry", je))
}

func (h *Handler) GetJournalEntry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	je, ok := h.store.JournalEntries.Get(id)
	if !ok { notFoundFault(w, "JournalEntry", id); return }
	qboJSON(w, http.StatusOK, entityResponse("JournalEntry", je))
}

// --- Estimate ---

func (h *Handler) CreateOrUpdateEstimate(w http.ResponseWriter, r *http.Request) {
	var est store.Estimate
	if err := json.NewDecoder(r.Body).Decode(&est); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if est.Id != "" {
		existing, ok := h.store.Estimates.Get(est.Id)
		if !ok { notFoundFault(w, "Estimate", est.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, est.SyncToken); err != nil { staleSyncTokenFault(w); return }
		est.SyncToken = IncrementSyncToken(existing.SyncToken)
		est.MetaData.CreateTime = existing.MetaData.CreateTime
		est.MetaData.LastUpdatedTime = h.store.Now()
		est.Domain = "QBO"
		h.store.Estimates.Set(est.Id, est)
	} else {
		est.Id = h.store.NextID()
		est.SyncToken = "0"
		est.Domain = "QBO"
		if est.TxnStatus == "" { est.TxnStatus = "Pending" }
		est.MetaData = h.store.NewMetaData()
		var total float64
		for _, line := range est.Line { total += line.Amount }
		est.TotalAmt = total
		h.store.Estimates.Set(est.Id, est)
	}
	qboJSON(w, http.StatusOK, entityResponse("Estimate", est))
}

func (h *Handler) GetEstimate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	est, ok := h.store.Estimates.Get(id)
	if !ok { notFoundFault(w, "Estimate", id); return }
	qboJSON(w, http.StatusOK, entityResponse("Estimate", est))
}

// --- Purchase ---

func (h *Handler) CreateOrUpdatePurchase(w http.ResponseWriter, r *http.Request) {
	var pur store.Purchase
	if err := json.NewDecoder(r.Body).Decode(&pur); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if pur.Id != "" {
		existing, ok := h.store.Purchases.Get(pur.Id)
		if !ok { notFoundFault(w, "Purchase", pur.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, pur.SyncToken); err != nil { staleSyncTokenFault(w); return }
		pur.SyncToken = IncrementSyncToken(existing.SyncToken)
		pur.MetaData.CreateTime = existing.MetaData.CreateTime
		pur.MetaData.LastUpdatedTime = h.store.Now()
		pur.Domain = "QBO"
		h.store.Purchases.Set(pur.Id, pur)
	} else {
		pur.Id = h.store.NextID()
		pur.SyncToken = "0"
		pur.Domain = "QBO"
		pur.MetaData = h.store.NewMetaData()
		var total float64
		for _, line := range pur.Line { total += line.Amount }
		pur.TotalAmt = total
		h.store.Purchases.Set(pur.Id, pur)
		h.journalPurchaseCreated(&pur)
	}
	qboJSON(w, http.StatusOK, entityResponse("Purchase", pur))
}

func (h *Handler) GetPurchase(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pur, ok := h.store.Purchases.Get(id)
	if !ok { notFoundFault(w, "Purchase", id); return }
	qboJSON(w, http.StatusOK, entityResponse("Purchase", pur))
}

// --- Company Info ---

func (h *Handler) GetCompanyInfo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ci, ok := h.store.CompanyInfos.Get(id)
	if !ok {
		// Return default company info.
		ci = store.CompanyInfo{
			Id: "1", SyncToken: "0", CompanyName: "Sandbox Company",
			Domain: "QBO", MetaData: h.store.NewMetaData(),
		}
	}
	qboJSON(w, http.StatusOK, entityResponse("CompanyInfo", ci))
}
