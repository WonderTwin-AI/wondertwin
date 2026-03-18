package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
)

func (h *Handler) CreateOrUpdateCustomer(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		validationFault(w, "500", "Failed to read body", err.Error())
		return
	}
	var c store.Customer
	if err := json.Unmarshal(body, &c); err != nil {
		validationFault(w, "500", "Invalid JSON", err.Error())
		return
	}

	if c.Id != "" {
		// Update
		existing, ok := h.store.Customers.Get(c.Id)
		if !ok {
			notFoundFault(w, "Customer", c.Id)
			return
		}
		if err := ValidateSyncToken(existing.SyncToken, c.SyncToken); err != nil {
			staleSyncTokenFault(w)
			return
		}
		// Sparse update: merge only provided fields.
		if IsSparse(body) {
			merged, err := SparseUpdate(existing, body)
			if err != nil {
				validationFault(w, "500", "Sparse merge failed", err.Error())
				return
			}
			c = merged
		}
		c.SyncToken = IncrementSyncToken(existing.SyncToken)
		c.MetaData.CreateTime = existing.MetaData.CreateTime
		c.MetaData.LastUpdatedTime = h.store.Now()
		c.Domain = "QBO"
		h.store.Customers.Set(c.Id, c)
		h.fireEvent("Customer", c.Id, "Update")
	} else {
		// Create
		c.Id = h.store.NextID()
		c.SyncToken = "0"
		c.Active = true
		c.Domain = "QBO"
		c.MetaData = h.store.NewMetaData()
		h.store.Customers.Set(c.Id, c)
		h.fireEvent("Customer", c.Id, "Create")
	}

	qboJSON(w, http.StatusOK, entityResponse("Customer", c))
}

func (h *Handler) GetCustomer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := h.store.Customers.Get(id)
	if !ok {
		notFoundFault(w, "Customer", id)
		return
	}
	qboJSON(w, http.StatusOK, entityResponse("Customer", c))
}

func (h *Handler) fireEvent(entityName, entityID, operation string) {
	if h.dispatcher == nil {
		return
	}
	h.dispatcher.Enqueue(entityName+"."+operation, map[string]any{
		"name":        entityName,
		"id":          entityID,
		"operation":   operation,
		"lastUpdated": h.store.Now(),
	})
}
