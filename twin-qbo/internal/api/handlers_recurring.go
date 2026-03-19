package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

func (h *Handler) CreateOrUpdateRecurringTransaction(w http.ResponseWriter, r *http.Request) {
	op := r.URL.Query().Get("operation")
	var rt store.RecurringTransaction
	if err := json.NewDecoder(r.Body).Decode(&rt); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}

	if op == "delete" {
		if _, ok := h.store.RecurringTransactions.Get(rt.Id); !ok {
			notFoundFault(w, "RecurringTransaction", rt.Id)
			return
		}
		h.store.RecurringTransactions.Delete(rt.Id)
		qboJSON(w, http.StatusOK, entityResponse("RecurringTransaction", rt))
		return
	}

	if rt.Id != "" {
		// Update
		existing, ok := h.store.RecurringTransactions.Get(rt.Id)
		if !ok {
			notFoundFault(w, "RecurringTransaction", rt.Id)
			return
		}
		if err := ValidateSyncToken(existing.SyncToken, rt.SyncToken); err != nil {
			staleSyncTokenFault(w)
			return
		}
		rt.SyncToken = IncrementSyncToken(existing.SyncToken)
		rt.MetaData.CreateTime = existing.MetaData.CreateTime
		rt.MetaData.LastUpdatedTime = h.store.Now()
		rt.Domain = "QBO"
		h.store.RecurringTransactions.Set(rt.Id, rt)
	} else {
		// Create
		rt.Id = h.store.NextID()
		rt.SyncToken = "0"
		rt.Active = true
		rt.Domain = "QBO"
		rt.MetaData = h.store.NewMetaData()
		h.store.RecurringTransactions.Set(rt.Id, rt)
	}

	qboJSON(w, http.StatusOK, entityResponse("RecurringTransaction", rt))
}

func (h *Handler) GetRecurringTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rt, ok := h.store.RecurringTransactions.Get(id)
	if !ok {
		notFoundFault(w, "RecurringTransaction", id)
		return
	}
	qboJSON(w, http.StatusOK, entityResponse("RecurringTransaction", rt))
}
