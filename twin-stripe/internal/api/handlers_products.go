package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	name := r.FormValue("name")
	if name == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required param: name.")
		return
	}

	id := h.store.Products.NextID()
	prod := store.Product{
		ID:          id,
		Object:      "product",
		Name:        name,
		Active:      true,
		Description: r.FormValue("description"),
		Livemode:    false,
		Metadata:    parseMetadata(r),
		Created:     store.Now(),
		Updated:     store.Now(),
	}

	h.store.Products.Set(id, prod)
	h.emitEvent("product.created", mapFromJSON(prod))
	twincore.JSON(w, http.StatusOK, prod)
}

func (h *Handler) GetProduct(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	prod, ok := h.store.Products.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such product: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, prod)
}

func (h *Handler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	prod, ok := h.store.Products.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such product: "+id)
		return
	}
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	if v := r.FormValue("name"); v != "" {
		prod.Name = v
	}
	if v := r.FormValue("description"); v != "" {
		prod.Description = v
	}
	if v := r.FormValue("active"); v == "false" {
		prod.Active = false
	} else if v == "true" {
		prod.Active = true
	}
	if meta := parseMetadata(r); len(meta) > 0 {
		prod.Metadata = meta
	}
	prod.Updated = store.Now()

	h.store.Products.Set(id, prod)
	h.emitEvent("product.updated", mapFromJSON(prod))
	twincore.JSON(w, http.StatusOK, prod)
}

func (h *Handler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.Products.Delete(id) {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such product: "+id)
		return
	}

	// Cascade: deactivate associated prices.
	prices := h.store.Prices.Filter(func(_ string, p store.Price) bool {
		return p.Product == id && p.Active
	})
	for _, p := range prices {
		p.Active = false
		h.store.Prices.Set(p.ID, p)
	}

	twincore.JSON(w, http.StatusOK, map[string]any{"id": id, "object": "product", "deleted": true})
}

func (h *Handler) ListProducts(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.Products.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/products",
		"has_more": page.HasMore,
		"data":     page.Data,
	})
}
