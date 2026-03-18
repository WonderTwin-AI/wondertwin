package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

func (h *Handler) CreateOrUpdateItem(w http.ResponseWriter, r *http.Request) {
	var item store.Item
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}

	if item.Id != "" {
		existing, ok := h.store.Items.Get(item.Id)
		if !ok {
			notFoundFault(w, "Item", item.Id)
			return
		}
		if err := ValidateSyncToken(existing.SyncToken, item.SyncToken); err != nil {
			staleSyncTokenFault(w)
			return
		}
		item.SyncToken = IncrementSyncToken(existing.SyncToken)
		item.MetaData.CreateTime = existing.MetaData.CreateTime
		item.MetaData.LastUpdatedTime = h.store.Now()
		item.Domain = "QBO"
		h.store.Items.Set(item.Id, item)
		h.fireEvent("Item", item.Id, "Update")
	} else {
		item.Id = h.store.NextID()
		item.SyncToken = "0"
		item.Active = true
		item.Domain = "QBO"
		if item.Type == "" {
			item.Type = "Service"
		}
		item.MetaData = h.store.NewMetaData()
		h.store.Items.Set(item.Id, item)
		h.fireEvent("Item", item.Id, "Create")
	}

	qboJSON(w, http.StatusOK, entityResponse("Item", item))
}

func (h *Handler) GetItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	item, ok := h.store.Items.Get(id)
	if !ok {
		notFoundFault(w, "Item", id)
		return
	}
	qboJSON(w, http.StatusOK, entityResponse("Item", item))
}
