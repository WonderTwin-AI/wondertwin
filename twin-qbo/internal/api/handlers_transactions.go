package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

// --- Credit Memo ---

func (h *Handler) CreateOrUpdateCreditMemo(w http.ResponseWriter, r *http.Request) {
	op := r.URL.Query().Get("operation")
	var cm store.CreditMemo
	if err := json.NewDecoder(r.Body).Decode(&cm); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if op == "delete" || op == "void" {
		existing, ok := h.store.CreditMemos.Get(cm.Id)
		if !ok { notFoundFault(w, "CreditMemo", cm.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, cm.SyncToken); err != nil { staleSyncTokenFault(w); return }
		// Restore customer balance (credit memo had decreased it).
		if existing.CustomerRef != nil {
			if cust, ok := h.store.Customers.Get(existing.CustomerRef.Value); ok {
				cust.Balance += existing.TotalAmt
				h.store.Customers.Set(cust.Id, cust)
			}
		}
		if op == "void" { existing.TotalAmt = 0; existing.RemainingCredit = 0; for i := range existing.Line { existing.Line[i].Amount = 0 } }
		existing.SyncToken = IncrementSyncToken(existing.SyncToken); existing.MetaData.LastUpdatedTime = h.store.Now()
		if op == "delete" { h.store.CreditMemos.Delete(cm.Id); h.fireEvent("CreditMemo", cm.Id, "Delete") } else { h.store.CreditMemos.Set(cm.Id, existing); h.fireEvent("CreditMemo", cm.Id, "Void") }
		qboJSON(w, http.StatusOK, entityResponse("CreditMemo", existing)); return
	}
	if cm.Id != "" {
		existing, ok := h.store.CreditMemos.Get(cm.Id)
		if !ok { notFoundFault(w, "CreditMemo", cm.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, cm.SyncToken); err != nil { staleSyncTokenFault(w); return }
		cm.SyncToken = IncrementSyncToken(existing.SyncToken)
		cm.MetaData.CreateTime = existing.MetaData.CreateTime
		cm.MetaData.LastUpdatedTime = h.store.Now()
		cm.Domain = "QBO"
		// Apply credit to linked invoices.
		for _, link := range cm.LinkedTxn {
			if link.TxnType == "Invoice" {
				if inv, ok := h.store.Invoices.Get(link.TxnId); ok {
					// Determine how much to apply.
					applyAmt := cm.RemainingCredit
					if applyAmt <= 0 {
						applyAmt = existing.RemainingCredit
					}
					if applyAmt > inv.Balance {
						applyAmt = inv.Balance
					}
					if applyAmt <= 0 {
						continue
					}
					inv.Balance -= applyAmt
					if inv.Balance < 0 {
						inv.Balance = 0
					}
					inv.MetaData.LastUpdatedTime = h.store.Now()
					h.store.Invoices.Set(inv.Id, inv)

					// Reduce remaining credit.
					cm.RemainingCredit = existing.RemainingCredit - applyAmt

					// Generate reversing journal entry for applied amount.
					h.journalCreditMemoApplied(cm.Id, applyAmt)
				}
			}
		}
		h.store.CreditMemos.Set(cm.Id, cm)
	} else {
		cm.Id = h.store.NextID()
		cm.SyncToken = "0"
		cm.Domain = "QBO"
		cm.MetaData = h.store.NewMetaData()
		computeCreditMemoTotals(&cm)
		cm.RemainingCredit = cm.TotalAmt
		h.store.CreditMemos.Set(cm.Id, cm)
		h.journalCreditMemoCreated(&cm)
		// Update customer balance.
		if cm.CustomerRef != nil {
			if cust, ok := h.store.Customers.Get(cm.CustomerRef.Value); ok {
				cust.Balance -= cm.TotalAmt
				if cust.Balance < 0 {
					cust.Balance = 0
				}
				h.store.Customers.Set(cust.Id, cust)
			}
		}
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
	op := r.URL.Query().Get("operation")
	var vc store.VendorCredit
	if err := json.NewDecoder(r.Body).Decode(&vc); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if op == "delete" {
		if _, ok := h.store.VendorCredits.Get(vc.Id); !ok { notFoundFault(w, "VendorCredit", vc.Id); return }
		h.store.VendorCredits.Delete(vc.Id); h.fireEvent("VendorCredit", vc.Id, "Delete")
		qboJSON(w, http.StatusOK, entityResponse("VendorCredit", vc)); return
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
	op := r.URL.Query().Get("operation")
	var sr store.SalesReceipt
	if err := json.NewDecoder(r.Body).Decode(&sr); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if op == "delete" || op == "void" {
		existing, ok := h.store.SalesReceipts.Get(sr.Id)
		if !ok { notFoundFault(w, "SalesReceipt", sr.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, sr.SyncToken); err != nil { staleSyncTokenFault(w); return }
		if op == "void" { h.journalSalesReceiptVoided(&existing); existing.TotalAmt = 0; for i := range existing.Line { existing.Line[i].Amount = 0 } }
		existing.SyncToken = IncrementSyncToken(existing.SyncToken); existing.MetaData.LastUpdatedTime = h.store.Now()
		if op == "delete" { h.journalSalesReceiptVoided(&existing); h.store.SalesReceipts.Delete(sr.Id); h.fireEvent("SalesReceipt", sr.Id, "Delete") } else { h.store.SalesReceipts.Set(sr.Id, existing); h.fireEvent("SalesReceipt", sr.Id, "Void") }
		qboJSON(w, http.StatusOK, entityResponse("SalesReceipt", existing)); return
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
	op := r.URL.Query().Get("operation")
	var dep store.Deposit
	if err := json.NewDecoder(r.Body).Decode(&dep); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if op == "delete" {
		if _, ok := h.store.Deposits.Get(dep.Id); !ok { notFoundFault(w, "Deposit", dep.Id); return }
		h.store.Deposits.Delete(dep.Id); h.fireEvent("Deposit", dep.Id, "Delete")
		qboJSON(w, http.StatusOK, entityResponse("Deposit", dep)); return
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
	op := r.URL.Query().Get("operation")
	var xfer store.Transfer
	if err := json.NewDecoder(r.Body).Decode(&xfer); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if op == "delete" {
		if _, ok := h.store.Transfers.Get(xfer.Id); !ok { notFoundFault(w, "Transfer", xfer.Id); return }
		h.store.Transfers.Delete(xfer.Id); h.fireEvent("Transfer", xfer.Id, "Delete")
		qboJSON(w, http.StatusOK, entityResponse("Transfer", xfer)); return
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
	op := r.URL.Query().Get("operation")
	var je store.JournalEntry
	if err := json.NewDecoder(r.Body).Decode(&je); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if op == "delete" {
		if _, ok := h.store.JournalEntries.Get(je.Id); !ok { notFoundFault(w, "JournalEntry", je.Id); return }
		h.store.JournalEntries.Delete(je.Id); h.fireEvent("JournalEntry", je.Id, "Delete")
		qboJSON(w, http.StatusOK, entityResponse("JournalEntry", je)); return
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
	op := r.URL.Query().Get("operation")
	var est store.Estimate
	if err := json.NewDecoder(r.Body).Decode(&est); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if op == "delete" {
		if _, ok := h.store.Estimates.Get(est.Id); !ok { notFoundFault(w, "Estimate", est.Id); return }
		h.store.Estimates.Delete(est.Id); h.fireEvent("Estimate", est.Id, "Delete")
		qboJSON(w, http.StatusOK, entityResponse("Estimate", est)); return
	}
	if est.Id != "" {
		existing, ok := h.store.Estimates.Get(est.Id)
		if !ok { notFoundFault(w, "Estimate", est.Id); return }
		if err := ValidateSyncToken(existing.SyncToken, est.SyncToken); err != nil { staleSyncTokenFault(w); return }
		// Validate status transitions.
		if est.TxnStatus != "" && est.TxnStatus != existing.TxnStatus {
			if !validEstimateTransition(existing.TxnStatus, est.TxnStatus) {
				validationFault(w, "500", "Invalid status transition",
					"Cannot transition from "+existing.TxnStatus+" to "+est.TxnStatus)
				return
			}
		}
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

// validEstimateTransition checks whether a status transition is allowed.
func validEstimateTransition(from, to string) bool {
	switch from {
	case "Pending":
		return to == "Accepted" || to == "Rejected" || to == "Closed"
	case "Accepted":
		return to == "Closed" || to == "Rejected"
	default:
		return false // Closed and Rejected are terminal
	}
}

// ConvertEstimate creates an invoice from an estimate and marks it Closed.
func (h *Handler) ConvertEstimate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	est, ok := h.store.Estimates.Get(id)
	if !ok {
		notFoundFault(w, "Estimate", id)
		return
	}
	if est.TxnStatus == "Closed" || est.TxnStatus == "Rejected" {
		validationFault(w, "500", "Cannot convert estimate",
			"Estimate is "+est.TxnStatus)
		return
	}

	// Create invoice from estimate.
	inv := store.Invoice{
		Id:          h.store.NextID(),
		SyncToken:   "0",
		CustomerRef: est.CustomerRef,
		Line:        est.Line,
		TotalAmt:    est.TotalAmt,
		Balance:     est.TotalAmt,
		Domain:      "QBO",
		MetaData:    h.store.NewMetaData(),
	}
	if est.SalesTermRef != nil {
		inv.SalesTermRef = est.SalesTermRef
	}
	computeInvoiceTotals(&inv)
	inv.Balance = inv.TotalAmt
	h.store.Invoices.Set(inv.Id, inv)
	h.journalInvoiceCreated(&inv)
	h.fireEvent("Invoice", inv.Id, "Create")

	// Update estimate status to Closed and link to invoice.
	est.TxnStatus = "Closed"
	est.LinkedTxnId = inv.Id
	est.SyncToken = IncrementSyncToken(est.SyncToken)
	est.MetaData.LastUpdatedTime = h.store.Now()
	h.store.Estimates.Set(est.Id, est)
	h.fireEvent("Estimate", est.Id, "Update")

	qboJSON(w, http.StatusOK, entityResponse("Invoice", inv))
}

// --- Purchase ---

func (h *Handler) CreateOrUpdatePurchase(w http.ResponseWriter, r *http.Request) {
	op := r.URL.Query().Get("operation")
	var pur store.Purchase
	if err := json.NewDecoder(r.Body).Decode(&pur); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if op == "delete" {
		if _, ok := h.store.Purchases.Get(pur.Id); !ok { notFoundFault(w, "Purchase", pur.Id); return }
		h.store.Purchases.Delete(pur.Id); h.fireEvent("Purchase", pur.Id, "Delete")
		qboJSON(w, http.StatusOK, entityResponse("Purchase", pur)); return
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
		// Seed a default and store it so it's updateable.
		ci = store.CompanyInfo{
			Id: "1", SyncToken: "0", CompanyName: "Sandbox Company",
			Country: "US", FiscalYearStartMonth: "January",
			Domain: "QBO", MetaData: h.store.NewMetaData(),
		}
		h.store.CompanyInfos.Set("1", ci)
	}
	qboJSON(w, http.StatusOK, entityResponse("CompanyInfo", ci))
}

func (h *Handler) UpdateCompanyInfo(w http.ResponseWriter, r *http.Request) {
	var ci store.CompanyInfo
	if err := json.NewDecoder(r.Body).Decode(&ci); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}
	if ci.Id == "" {
		ci.Id = "1"
	}
	existing, ok := h.store.CompanyInfos.Get(ci.Id)
	if !ok {
		existing = store.CompanyInfo{Id: "1", SyncToken: "0", Domain: "QBO", MetaData: h.store.NewMetaData()}
	}
	if err := ValidateSyncToken(existing.SyncToken, ci.SyncToken); err != nil {
		staleSyncTokenFault(w)
		return
	}
	ci.SyncToken = IncrementSyncToken(existing.SyncToken)
	ci.MetaData.CreateTime = existing.MetaData.CreateTime
	ci.MetaData.LastUpdatedTime = h.store.Now()
	ci.Domain = "QBO"
	h.store.CompanyInfos.Set(ci.Id, ci)
	qboJSON(w, http.StatusOK, entityResponse("CompanyInfo", ci))
}
