package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-loyaltylion/internal/store"
)

// ListCustomers handles GET /v2/customers with optional ?email= filter.
func (h *Handler) ListCustomers(w http.ResponseWriter, r *http.Request) {
	apiKey := getAPIKey(r)
	email := r.URL.Query().Get("email")

	// Process any expired points before returning data
	h.store.ProcessExpiredPoints(h.store.Clock.Now())

	var customers []store.Customer
	if email != "" {
		c := h.store.GetCustomerByEmail(apiKey, email)
		if c != nil {
			customers = []store.Customer{*c}
		} else {
			customers = []store.Customer{}
		}
	} else {
		customers = h.store.GetCustomersByMerchant(apiKey)
	}

	twincore.JSON(w, http.StatusOK, map[string]any{
		"customers": customers,
	})
}

// GetCustomer handles GET /v2/customers/{id}.
func (h *Handler) GetCustomer(w http.ResponseWriter, r *http.Request) {
	apiKey := getAPIKey(r)
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid customer ID")
		return
	}

	h.store.ProcessExpiredPoints(h.store.Clock.Now())

	c, ok := h.store.Customers.Get(store.CustomerKey(id))
	if !ok || c.APIKey != apiKey {
		twincore.Error(w, http.StatusNotFound, "customer not found")
		return
	}

	twincore.JSON(w, http.StatusOK, c)
}

// CreateCustomer handles POST /v2/customers.
func (h *Handler) CreateCustomer(w http.ResponseWriter, r *http.Request) {
	apiKey := getAPIKey(r)

	var req struct {
		MerchantID string            `json:"merchant_id"`
		Email      string            `json:"email"`
		Properties map[string]string `json:"properties,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		twincore.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.MerchantID == "" {
		twincore.Error(w, http.StatusUnprocessableEntity, "merchant_id is required")
		return
	}
	if req.Email == "" {
		twincore.Error(w, http.StatusUnprocessableEntity, "email is required")
		return
	}

	now := h.store.Clock.Now().Format(time.RFC3339)
	id := h.store.NextCustomerID()
	c := store.Customer{
		ID:             id,
		MerchantID:     req.MerchantID,
		Email:          req.Email,
		PointsApproved: 0,
		PointsPending:  0,
		PointsSpent:    0,
		PointsExpired:  0,
		Properties:     req.Properties,
		CreatedAt:      now,
		UpdatedAt:      now,
		APIKey:         apiKey,
	}

	h.store.Customers.Set(store.CustomerKey(id), c)
	twincore.JSON(w, http.StatusCreated, c)
}
