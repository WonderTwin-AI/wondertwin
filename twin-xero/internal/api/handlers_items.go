package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func (h *Handler) CreateItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []store.Item `json:"Items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}
	if len(req.Items) == 0 {
		xeroError(w, http.StatusBadRequest, "ValidationException", "At least one item is required")
		return
	}

	var result []store.Item
	for _, item := range req.Items {
		if item.ItemID == "" {
			item.ItemID = newUUID()
		}
		item.UpdatedDateUTC = store.XeroDateNow()
		h.store.Items.Set(item.ItemID, item)
		result = append(result, item)
	}

	xeroJSON(w, http.StatusOK, map[string]any{"Items": result})
}

func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	all := h.store.Items.List()
	xeroJSON(w, http.StatusOK, map[string]any{"Items": paginate(all, page, pageSize)})
}

func (h *Handler) GetItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ItemID")
	item, ok := h.store.Items.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "Item not found")
		return
	}
	xeroJSON(w, http.StatusOK, map[string]any{"Items": []store.Item{item}})
}
