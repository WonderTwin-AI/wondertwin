package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func (h *Handler) CreateManualJournal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ManualJournals []store.ManualJournal `json:"ManualJournals"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}
	if len(req.ManualJournals) == 0 {
		xeroError(w, http.StatusBadRequest, "ValidationException", "At least one manual journal is required")
		return
	}

	var result []store.ManualJournal
	for _, mj := range req.ManualJournals {
		if mj.ManualJournalID == "" {
			mj.ManualJournalID = newUUID()
		}
		mj.UpdatedDateUTC = store.XeroDateNow()

		// Build engine manual journal lines.
		var engineLines []ledger.ManualJournalLine
		for _, line := range mj.JournalLines {
			acctID := h.resolveAccountCode(line.AccountCode)
			var debit, credit int64
			amt := int64(line.LineAmount * 100)
			if amt > 0 {
				debit = amt
			} else if amt < 0 {
				credit = -amt
			}
			engineLines = append(engineLines, ledger.ManualJournalLine{
				AccountID:    acctID,
				Description:  line.Description,
				DebitAmount:  debit,
				CreditAmount: credit,
			})
		}

		engineMJ := &ledger.ManualJournal{
			ID:        mj.ManualJournalID,
			Narration: mj.Narration,
			Lines:     engineLines,
			Date:      parseXeroDate(mj.Date),
		}

		if err := h.engine.PostManualJournal(r.Context(), engineMJ); err != nil {
			xeroError(w, http.StatusBadRequest, "ValidationException", err.Error())
			return
		}

		mj.Status = "POSTED"
		h.store.ManualJournals.Set(mj.ManualJournalID, mj)
		result = append(result, mj)
	}

	xeroJSON(w, http.StatusOK, map[string]any{"ManualJournals": result})
}

func (h *Handler) ListManualJournals(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	all := h.store.ManualJournals.List()
	xeroJSON(w, http.StatusOK, map[string]any{"ManualJournals": paginate(all, page, pageSize)})
}

func (h *Handler) GetManualJournal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ManualJournalID")
	mj, ok := h.store.ManualJournals.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "ManualJournal not found")
		return
	}
	xeroJSON(w, http.StatusOK, map[string]any{"ManualJournals": []store.ManualJournal{mj}})
}
