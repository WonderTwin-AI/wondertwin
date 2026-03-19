package api

import (
	"encoding/json"
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func (h *Handler) CreateTaxRate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaxRates []store.TaxRate `json:"TaxRates"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}
	if len(req.TaxRates) == 0 {
		xeroError(w, http.StatusBadRequest, "ValidationException", "At least one tax rate is required")
		return
	}

	var result []store.TaxRate
	for _, tr := range req.TaxRates {
		if tr.Status == "" {
			tr.Status = "ACTIVE"
		}
		h.store.TaxRates.Set(tr.TaxType, tr)
		result = append(result, tr)
	}
	xeroJSON(w, http.StatusOK, map[string]any{"TaxRates": result})
}

func (h *Handler) ListTaxRates(w http.ResponseWriter, r *http.Request) {
	all := h.store.TaxRates.List()
	xeroJSON(w, http.StatusOK, map[string]any{"TaxRates": all})
}
