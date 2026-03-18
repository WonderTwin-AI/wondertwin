package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

func (h *Handler) CreateOrUpdateVendor(w http.ResponseWriter, r *http.Request) {
	var v store.Vendor
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}

	if v.Id != "" {
		existing, ok := h.store.Vendors.Get(v.Id)
		if !ok {
			notFoundFault(w, "Vendor", v.Id)
			return
		}
		if err := ValidateSyncToken(existing.SyncToken, v.SyncToken); err != nil {
			staleSyncTokenFault(w)
			return
		}
		v.SyncToken = IncrementSyncToken(existing.SyncToken)
		v.MetaData.CreateTime = existing.MetaData.CreateTime
		v.MetaData.LastUpdatedTime = h.store.Now()
		v.Domain = "QBO"
		h.store.Vendors.Set(v.Id, v)
		h.fireEvent("Vendor", v.Id, "Update")
	} else {
		v.Id = h.store.NextID()
		v.SyncToken = "0"
		v.Active = true
		v.Domain = "QBO"
		v.MetaData = h.store.NewMetaData()
		h.store.Vendors.Set(v.Id, v)
		h.fireEvent("Vendor", v.Id, "Create")
	}

	qboJSON(w, http.StatusOK, entityResponse("Vendor", v))
}

func (h *Handler) GetVendor(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	v, ok := h.store.Vendors.Get(id)
	if !ok {
		notFoundFault(w, "Vendor", id)
		return
	}
	qboJSON(w, http.StatusOK, entityResponse("Vendor", v))
}
