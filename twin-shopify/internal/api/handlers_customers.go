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

// --- Customers ---

func (h *Handler) ListCustomers(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"customers": h.store.Customers.List()})
}

func (h *Handler) GetCustomer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := h.store.Customers.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"customer": c})
}

func (h *Handler) CreateCustomer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Customer store.Customer `json:"customer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	c := req.Customer
	if c.Email == "" {
		shopifyValidationError(w, "email", []string{"can't be blank"})
		return
	}
	c.ID = h.store.NextID()
	now := h.store.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	if c.State == "" {
		c.State = "enabled"
	}
	if c.TotalSpent == "" {
		c.TotalSpent = "0.00"
	}

	storeID := fmt.Sprintf("%d", c.ID)
	h.store.Customers.Set(storeID, c)
	shopifyJSON(w, http.StatusCreated, map[string]any{"customer": c})
}

func (h *Handler) UpdateCustomer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := h.store.Customers.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Customer store.Customer `json:"customer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Customer.Email != "" {
		c.Email = req.Customer.Email
	}
	if req.Customer.FirstName != "" {
		c.FirstName = req.Customer.FirstName
	}
	if req.Customer.LastName != "" {
		c.LastName = req.Customer.LastName
	}
	if req.Customer.Phone != "" {
		c.Phone = req.Customer.Phone
	}
	if req.Customer.Tags != "" {
		c.Tags = req.Customer.Tags
	}
	if req.Customer.Note != "" {
		c.Note = req.Customer.Note
	}
	c.UpdatedAt = h.store.Now()
	h.store.Customers.Set(id, c)
	shopifyJSON(w, http.StatusOK, map[string]any{"customer": c})
}

func (h *Handler) DeleteCustomer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.Customers.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) SearchCustomers(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	q := strings.ToLower(query)
	customers := h.store.Customers.Filter(func(_ string, c store.Customer) bool {
		return strings.Contains(strings.ToLower(c.Email), q) ||
			strings.Contains(strings.ToLower(c.FirstName), q) ||
			strings.Contains(strings.ToLower(c.LastName), q)
	})
	shopifyJSON(w, http.StatusOK, map[string]any{"customers": customers})
}

func (h *Handler) CountCustomers(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"count": h.store.Customers.Count()})
}

// --- Customer Addresses ---

func (h *Handler) ListAddresses(w http.ResponseWriter, r *http.Request) {
	customerID := chi.URLParam(r, "customer_id")
	cid, err := strconv.ParseInt(customerID, 10, 64)
	if err != nil {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	addrs := h.store.Addresses.Filter(func(_ string, a store.Address) bool {
		return a.CustomerID == cid
	})
	shopifyJSON(w, http.StatusOK, map[string]any{"addresses": addrs})
}

func (h *Handler) CreateAddress(w http.ResponseWriter, r *http.Request) {
	customerID := chi.URLParam(r, "customer_id")
	c, ok := h.store.Customers.Get(customerID)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Address store.Address `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	a := req.Address
	a.ID = h.store.NextID()
	a.CustomerID = c.ID

	h.store.Addresses.Set(fmt.Sprintf("%d", a.ID), a)

	// Update customer's address list
	c.Addresses = append(c.Addresses, a)
	if len(c.Addresses) == 1 || a.Default {
		c.DefaultAddress = &a
	}
	c.UpdatedAt = h.store.Now()
	h.store.Customers.Set(customerID, c)

	shopifyJSON(w, http.StatusCreated, map[string]any{"customer_address": a})
}

func (h *Handler) GetAddress(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, ok := h.store.Addresses.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"customer_address": a})
}

func (h *Handler) UpdateAddress(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	customerID := chi.URLParam(r, "customer_id")
	a, ok := h.store.Addresses.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Address store.Address `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Address.Address1 != "" {
		a.Address1 = req.Address.Address1
	}
	if req.Address.Address2 != "" {
		a.Address2 = req.Address.Address2
	}
	if req.Address.City != "" {
		a.City = req.Address.City
	}
	if req.Address.Province != "" {
		a.Province = req.Address.Province
	}
	if req.Address.Country != "" {
		a.Country = req.Address.Country
	}
	if req.Address.Zip != "" {
		a.Zip = req.Address.Zip
	}
	if req.Address.Phone != "" {
		a.Phone = req.Address.Phone
	}
	if req.Address.FirstName != "" {
		a.FirstName = req.Address.FirstName
	}
	if req.Address.LastName != "" {
		a.LastName = req.Address.LastName
	}
	h.store.Addresses.Set(id, a)

	// Update customer's embedded address list
	if c, ok := h.store.Customers.Get(customerID); ok {
		for i, ca := range c.Addresses {
			if ca.ID == a.ID {
				c.Addresses[i] = a
				break
			}
		}
		if a.Default {
			c.DefaultAddress = &a
		}
		c.UpdatedAt = h.store.Now()
		h.store.Customers.Set(customerID, c)
	}

	shopifyJSON(w, http.StatusOK, map[string]any{"customer_address": a})
}

func (h *Handler) DeleteAddress(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	customerID := chi.URLParam(r, "customer_id")
	if !h.store.Addresses.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}

	// Remove from customer's embedded list
	if c, ok := h.store.Customers.Get(customerID); ok {
		addrID, _ := strconv.ParseInt(id, 10, 64)
		for i, ca := range c.Addresses {
			if ca.ID == addrID {
				c.Addresses = append(c.Addresses[:i], c.Addresses[i+1:]...)
				break
			}
		}
		c.UpdatedAt = h.store.Now()
		h.store.Customers.Set(customerID, c)
	}

	w.WriteHeader(http.StatusOK)
}
