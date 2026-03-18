package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

func (h *Handler) CreateTaxRate(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	displayName := r.FormValue("display_name")
	percentageStr := r.FormValue("percentage")
	inclusiveStr := r.FormValue("inclusive")
	if displayName == "" || percentageStr == "" || inclusiveStr == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing",
			"Missing required params: display_name, percentage, inclusive.")
		return
	}

	percentage, err := strconv.ParseFloat(percentageStr, 64)
	if err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_invalid",
			"Invalid percentage value.")
		return
	}

	inclusive := inclusiveStr == "true"

	id := h.store.TaxRates.NextID()
	tr := store.TaxRate{
		ID:           id,
		Object:       "tax_rate",
		Active:       true,
		DisplayName:  displayName,
		Description:  r.FormValue("description"),
		Inclusive:    inclusive,
		Percentage:   percentage,
		Country:      r.FormValue("country"),
		State:        r.FormValue("state"),
		Jurisdiction: r.FormValue("jurisdiction"),
		Livemode:     false,
		Metadata:     parseMetadata(r),
		Created:      store.Now(),
	}

	h.store.TaxRates.Set(id, tr)
	h.dispatcher.Enqueue("tax_rate.created", mapFromJSON(tr))
	twincore.JSON(w, http.StatusOK, tr)
}

func (h *Handler) GetTaxRate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tr, ok := h.store.TaxRates.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such tax_rate: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, tr)
}

func (h *Handler) ListTaxRates(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.TaxRates.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/tax_rates",
		"has_more": page.HasMore,
		"data":     page.Data,
	})
}
