package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// SendInvoice simulates emailing an invoice. Updates EmailStatus to "EmailSent".
func (h *Handler) SendInvoice(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	inv, ok := h.store.Invoices.Get(id)
	if !ok {
		notFoundFault(w, "Invoice", id)
		return
	}
	inv.EmailStatus = "EmailSent"
	inv.SyncToken = IncrementSyncToken(inv.SyncToken)
	inv.MetaData.LastUpdatedTime = h.store.Now()
	h.store.Invoices.Set(id, inv)
	qboJSON(w, http.StatusOK, entityResponse("Invoice", inv))
}

// InvoicePDF returns a minimal PDF placeholder for an invoice.
func (h *Handler) InvoicePDF(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := h.store.Invoices.Get(id); !ok {
		notFoundFault(w, "Invoice", id)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=Invoice-%s.pdf", id))
	w.WriteHeader(http.StatusOK)
	// Minimal valid PDF.
	w.Write([]byte("%PDF-1.0\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/MediaBox[0 0 612 792]/Parent 2 0 R/Resources<<>>>>endobj\nxref\n0 4\n0000000000 65535 f \n0000000009 00000 n \n0000000058 00000 n \n0000000115 00000 n \ntrailer<</Size 4/Root 1 0 R>>\nstartxref\n206\n%%EOF\n"))
}

// SendEstimate simulates emailing an estimate.
func (h *Handler) SendEstimate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	est, ok := h.store.Estimates.Get(id)
	if !ok {
		notFoundFault(w, "Estimate", id)
		return
	}
	est.SyncToken = IncrementSyncToken(est.SyncToken)
	est.MetaData.LastUpdatedTime = h.store.Now()
	h.store.Estimates.Set(id, est)
	qboJSON(w, http.StatusOK, entityResponse("Estimate", est))
}

// SendSalesReceipt simulates emailing a sales receipt.
func (h *Handler) SendSalesReceipt(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sr, ok := h.store.SalesReceipts.Get(id)
	if !ok {
		notFoundFault(w, "SalesReceipt", id)
		return
	}
	sr.SyncToken = IncrementSyncToken(sr.SyncToken)
	sr.MetaData.LastUpdatedTime = h.store.Now()
	h.store.SalesReceipts.Set(id, sr)
	qboJSON(w, http.StatusOK, entityResponse("SalesReceipt", sr))
}
