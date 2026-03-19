package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func (h *Handler) CreateTrackingCategory(w http.ResponseWriter, r *http.Request) {
	var req store.TrackingCategory
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}
	if req.TrackingCategoryID == "" {
		req.TrackingCategoryID = newUUID()
	}
	if req.Status == "" {
		req.Status = "ACTIVE"
	}
	// Generate IDs for options.
	for i := range req.Options {
		if req.Options[i].TrackingOptionID == "" {
			req.Options[i].TrackingOptionID = newUUID()
		}
		if req.Options[i].Status == "" {
			req.Options[i].Status = "ACTIVE"
		}
	}
	h.store.TrackingCategories.Set(req.TrackingCategoryID, req)
	xeroJSON(w, http.StatusOK, map[string]any{"TrackingCategories": []store.TrackingCategory{req}})
}

func (h *Handler) ListTrackingCategories(w http.ResponseWriter, r *http.Request) {
	all := h.store.TrackingCategories.List()
	xeroJSON(w, http.StatusOK, map[string]any{"TrackingCategories": all})
}

func (h *Handler) GetTrackingCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "TrackingCategoryID")
	tc, ok := h.store.TrackingCategories.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "TrackingCategory not found")
		return
	}
	xeroJSON(w, http.StatusOK, map[string]any{"TrackingCategories": []store.TrackingCategory{tc}})
}

func (h *Handler) UpdateTrackingCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "TrackingCategoryID")
	existing, ok := h.store.TrackingCategories.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "TrackingCategory not found")
		return
	}
	var req store.TrackingCategory
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		xeroError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON: "+err.Error())
		return
	}
	req.TrackingCategoryID = id
	if req.Name == "" {
		req.Name = existing.Name
	}
	if req.Status == "" {
		req.Status = existing.Status
	}
	h.store.TrackingCategories.Set(id, req)
	xeroJSON(w, http.StatusOK, map[string]any{"TrackingCategories": []store.TrackingCategory{req}})
}

func (h *Handler) DeleteTrackingCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "TrackingCategoryID")
	tc, ok := h.store.TrackingCategories.Get(id)
	if !ok {
		xeroError(w, http.StatusNotFound, "ValidationException", "TrackingCategory not found")
		return
	}
	tc.Status = "DELETED"
	h.store.TrackingCategories.Set(id, tc)
	xeroJSON(w, http.StatusOK, map[string]any{"TrackingCategories": []store.TrackingCategory{tc}})
}
