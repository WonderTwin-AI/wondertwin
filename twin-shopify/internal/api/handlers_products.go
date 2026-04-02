package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-shopify/internal/store"
)

// --- Products ---

func (h *Handler) ListProducts(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"products": h.store.Products.List()})
}

func (h *Handler) GetProduct(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, ok := h.store.Products.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"product": p})
}

func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Product store.Product `json:"product"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	p := req.Product
	if p.Title == "" {
		shopifyValidationError(w, "title", []string{"can't be blank"})
		return
	}
	p.ID = h.store.NextID()
	now := h.store.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.Status == "" {
		p.Status = "active"
	}
	if p.Handle == "" {
		p.Handle = strings.ToLower(strings.ReplaceAll(p.Title, " ", "-"))
	}

	// Create default variant if none provided
	if len(p.Variants) == 0 {
		v := store.Variant{
			ID:              h.store.NextID(),
			ProductID:       p.ID,
			Title:           "Default Title",
			Price:           "0.00",
			Position:        1,
			InventoryItemID: h.store.NextID(),
			WeightUnit:      "kg",
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		p.Variants = []store.Variant{v}
		h.store.Variants.Set(fmt.Sprintf("%d", v.ID), v)
	} else {
		for i := range p.Variants {
			p.Variants[i].ID = h.store.NextID()
			p.Variants[i].ProductID = p.ID
			p.Variants[i].InventoryItemID = h.store.NextID()
			p.Variants[i].CreatedAt = now
			p.Variants[i].UpdatedAt = now
			if p.Variants[i].Position == 0 {
				p.Variants[i].Position = i + 1
			}
			h.store.Variants.Set(fmt.Sprintf("%d", p.Variants[i].ID), p.Variants[i])
		}
	}

	storeID := fmt.Sprintf("%d", p.ID)
	h.store.Products.Set(storeID, p)
	shopifyJSON(w, http.StatusCreated, map[string]any{"product": p})
}

func (h *Handler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, ok := h.store.Products.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Product store.Product `json:"product"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Product.Title != "" {
		p.Title = req.Product.Title
	}
	if req.Product.BodyHTML != "" {
		p.BodyHTML = req.Product.BodyHTML
	}
	if req.Product.Vendor != "" {
		p.Vendor = req.Product.Vendor
	}
	if req.Product.ProductType != "" {
		p.ProductType = req.Product.ProductType
	}
	if req.Product.Tags != "" {
		p.Tags = req.Product.Tags
	}
	if req.Product.Status != "" {
		p.Status = req.Product.Status
	}
	p.UpdatedAt = h.store.Now()
	h.store.Products.Set(id, p)
	shopifyJSON(w, http.StatusOK, map[string]any{"product": p})
}

func (h *Handler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.Products.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) CountProducts(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"count": h.store.Products.Count()})
}

// --- Variants ---

func (h *Handler) ListVariants(w http.ResponseWriter, r *http.Request) {
	productID := chi.URLParam(r, "product_id")
	pid, err := strconv.ParseInt(productID, 10, 64)
	if err != nil {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	variants := h.store.Variants.Filter(func(_ string, v store.Variant) bool {
		return v.ProductID == pid
	})
	shopifyJSON(w, http.StatusOK, map[string]any{"variants": variants})
}

func (h *Handler) CreateVariant(w http.ResponseWriter, r *http.Request) {
	productID := chi.URLParam(r, "product_id")
	p, ok := h.store.Products.Get(productID)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Variant store.Variant `json:"variant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	v := req.Variant
	v.ID = h.store.NextID()
	v.ProductID = p.ID
	v.InventoryItemID = h.store.NextID()
	now := h.store.Now()
	v.CreatedAt = now
	v.UpdatedAt = now
	if v.Position == 0 {
		v.Position = len(p.Variants) + 1
	}

	h.store.Variants.Set(fmt.Sprintf("%d", v.ID), v)

	// Update product's variant list
	p.Variants = append(p.Variants, v)
	p.UpdatedAt = now
	h.store.Products.Set(productID, p)

	shopifyJSON(w, http.StatusCreated, map[string]any{"variant": v})
}

func (h *Handler) GetVariant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	v, ok := h.store.Variants.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"variant": v})
}

func (h *Handler) UpdateVariant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	v, ok := h.store.Variants.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Variant store.Variant `json:"variant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Variant.Title != "" {
		v.Title = req.Variant.Title
	}
	if req.Variant.Price != "" {
		v.Price = req.Variant.Price
	}
	if req.Variant.SKU != "" {
		v.SKU = req.Variant.SKU
	}
	if req.Variant.CompareAtPrice != "" {
		v.CompareAtPrice = req.Variant.CompareAtPrice
	}
	if req.Variant.Barcode != "" {
		v.Barcode = req.Variant.Barcode
	}
	if req.Variant.Option1 != "" {
		v.Option1 = req.Variant.Option1
	}
	if req.Variant.Option2 != "" {
		v.Option2 = req.Variant.Option2
	}
	if req.Variant.Option3 != "" {
		v.Option3 = req.Variant.Option3
	}
	v.UpdatedAt = h.store.Now()
	h.store.Variants.Set(id, v)

	// Update product's embedded variant list
	pid := fmt.Sprintf("%d", v.ProductID)
	if p, ok := h.store.Products.Get(pid); ok {
		for i, pv := range p.Variants {
			if pv.ID == v.ID {
				p.Variants[i] = v
				break
			}
		}
		p.UpdatedAt = v.UpdatedAt
		h.store.Products.Set(pid, p)
	}

	shopifyJSON(w, http.StatusOK, map[string]any{"variant": v})
}

func (h *Handler) DeleteVariant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	productID := chi.URLParam(r, "product_id")
	if !h.store.Variants.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}

	// Remove from product's embedded list
	if p, ok := h.store.Products.Get(productID); ok {
		vid, _ := strconv.ParseInt(id, 10, 64)
		for i, pv := range p.Variants {
			if pv.ID == vid {
				p.Variants = append(p.Variants[:i], p.Variants[i+1:]...)
				break
			}
		}
		p.UpdatedAt = h.store.Now()
		h.store.Products.Set(productID, p)
	}

	w.WriteHeader(http.StatusOK)
}

// --- Images ---

func (h *Handler) ListImages(w http.ResponseWriter, r *http.Request) {
	productID := chi.URLParam(r, "product_id")
	pid, err := strconv.ParseInt(productID, 10, 64)
	if err != nil {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	images := h.store.Images.Filter(func(_ string, img store.Image) bool {
		return img.ProductID == pid
	})
	shopifyJSON(w, http.StatusOK, map[string]any{"images": images})
}

func (h *Handler) CreateImage(w http.ResponseWriter, r *http.Request) {
	productID := chi.URLParam(r, "product_id")
	p, ok := h.store.Products.Get(productID)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Image store.Image `json:"image"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	img := req.Image
	img.ID = h.store.NextID()
	img.ProductID = p.ID
	now := h.store.Now()
	img.CreatedAt = now
	img.UpdatedAt = now
	if img.Position == 0 {
		img.Position = len(p.Images) + 1
	}

	h.store.Images.Set(fmt.Sprintf("%d", img.ID), img)

	p.Images = append(p.Images, img)
	p.UpdatedAt = now
	h.store.Products.Set(productID, p)

	shopifyJSON(w, http.StatusCreated, map[string]any{"image": img})
}

func (h *Handler) GetImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	img, ok := h.store.Images.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"image": img})
}

func (h *Handler) UpdateImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	productID := chi.URLParam(r, "product_id")
	img, ok := h.store.Images.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Image store.Image `json:"image"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Image.Src != "" {
		img.Src = req.Image.Src
	}
	if req.Image.Alt != "" {
		img.Alt = req.Image.Alt
	}
	if req.Image.Position != 0 {
		img.Position = req.Image.Position
	}
	img.UpdatedAt = h.store.Now()
	h.store.Images.Set(id, img)

	// Update product's embedded image list
	if p, ok := h.store.Products.Get(productID); ok {
		for i, pi := range p.Images {
			if pi.ID == img.ID {
				p.Images[i] = img
				break
			}
		}
		p.UpdatedAt = img.UpdatedAt
		h.store.Products.Set(productID, p)
	}

	shopifyJSON(w, http.StatusOK, map[string]any{"image": img})
}

func (h *Handler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	productID := chi.URLParam(r, "product_id")
	if !h.store.Images.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}

	// Remove from product's embedded list
	if p, ok := h.store.Products.Get(productID); ok {
		imgID, _ := strconv.ParseInt(id, 10, 64)
		for i, pi := range p.Images {
			if pi.ID == imgID {
				p.Images = append(p.Images[:i], p.Images[i+1:]...)
				break
			}
		}
		p.UpdatedAt = h.store.Now()
		h.store.Products.Set(productID, p)
	}

	w.WriteHeader(http.StatusOK)
}
