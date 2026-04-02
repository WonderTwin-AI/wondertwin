package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-shopify/internal/store"
)

// --- Draft Orders ---

func (h *Handler) ListDraftOrders(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"draft_orders": h.store.DraftOrders.List()})
}

func (h *Handler) GetDraftOrder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, ok := h.store.DraftOrders.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"draft_order": d})
}

func (h *Handler) CreateDraftOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DraftOrder store.DraftOrder `json:"draft_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	d := req.DraftOrder
	d.ID = h.store.NextID()
	now := h.store.Now()
	d.CreatedAt = now
	d.UpdatedAt = now
	d.Name = fmt.Sprintf("#D%d", d.ID)
	if d.Status == "" {
		d.Status = "open"
	}
	if d.Currency == "" {
		d.Currency = "USD"
	}
	if d.TotalPrice == "" {
		d.TotalPrice = "0.00"
	}
	if d.SubtotalPrice == "" {
		d.SubtotalPrice = "0.00"
	}

	h.store.DraftOrders.Set(fmt.Sprintf("%d", d.ID), d)
	shopifyJSON(w, http.StatusCreated, map[string]any{"draft_order": d})
}

func (h *Handler) UpdateDraftOrder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, ok := h.store.DraftOrders.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		DraftOrder store.DraftOrder `json:"draft_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.DraftOrder.Email != "" {
		d.Email = req.DraftOrder.Email
	}
	if req.DraftOrder.Note != "" {
		d.Note = req.DraftOrder.Note
	}
	d.UpdatedAt = h.store.Now()
	h.store.DraftOrders.Set(id, d)
	shopifyJSON(w, http.StatusOK, map[string]any{"draft_order": d})
}

func (h *Handler) DeleteDraftOrder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.DraftOrders.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) CompleteDraftOrder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, ok := h.store.DraftOrders.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	d.Status = "completed"
	d.UpdatedAt = h.store.Now()
	h.store.DraftOrders.Set(id, d)
	shopifyJSON(w, http.StatusOK, map[string]any{"draft_order": d})
}

func (h *Handler) SendDraftOrderInvoice(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, ok := h.store.DraftOrders.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	d.Status = "invoice_sent"
	d.UpdatedAt = h.store.Now()
	h.store.DraftOrders.Set(id, d)
	shopifyJSON(w, http.StatusOK, map[string]any{"draft_order_invoice": map[string]any{
		"to":      d.Email,
		"subject": "Invoice " + d.Name,
	}})
}

// --- Price Rules ---

func (h *Handler) ListPriceRules(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"price_rules": h.store.PriceRules.List()})
}

func (h *Handler) GetPriceRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pr, ok := h.store.PriceRules.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"price_rule": pr})
}

func (h *Handler) CreatePriceRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PriceRule store.PriceRule `json:"price_rule"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	pr := req.PriceRule
	if pr.Title == "" {
		shopifyValidationError(w, "title", []string{"can't be blank"})
		return
	}
	pr.ID = h.store.NextID()
	now := h.store.Now()
	pr.CreatedAt = now
	pr.UpdatedAt = now
	if pr.TargetType == "" {
		pr.TargetType = "line_item"
	}
	if pr.ValueType == "" {
		pr.ValueType = "fixed_amount"
	}
	if pr.StartsAt == "" {
		pr.StartsAt = now
	}

	h.store.PriceRules.Set(fmt.Sprintf("%d", pr.ID), pr)
	shopifyJSON(w, http.StatusCreated, map[string]any{"price_rule": pr})
}

func (h *Handler) UpdatePriceRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pr, ok := h.store.PriceRules.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		PriceRule store.PriceRule `json:"price_rule"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.PriceRule.Title != "" {
		pr.Title = req.PriceRule.Title
	}
	if req.PriceRule.Value != "" {
		pr.Value = req.PriceRule.Value
	}
	if req.PriceRule.ValueType != "" {
		pr.ValueType = req.PriceRule.ValueType
	}
	if req.PriceRule.EndsAt != "" {
		pr.EndsAt = req.PriceRule.EndsAt
	}
	pr.OncePerCustomer = req.PriceRule.OncePerCustomer
	pr.UsageLimit = req.PriceRule.UsageLimit
	pr.UpdatedAt = h.store.Now()
	h.store.PriceRules.Set(id, pr)
	shopifyJSON(w, http.StatusOK, map[string]any{"price_rule": pr})
}

func (h *Handler) DeletePriceRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.PriceRules.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- Discount Codes ---

func (h *Handler) ListDiscountCodes(w http.ResponseWriter, r *http.Request) {
	priceRuleID := chi.URLParam(r, "price_rule_id")
	prid, err := strconv.ParseInt(priceRuleID, 10, 64)
	if err != nil {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	codes := h.store.DiscountCodes.Filter(func(_ string, dc store.DiscountCode) bool {
		return dc.PriceRuleID == prid
	})
	shopifyJSON(w, http.StatusOK, map[string]any{"discount_codes": codes})
}

func (h *Handler) GetDiscountCode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	dc, ok := h.store.DiscountCodes.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"discount_code": dc})
}

func (h *Handler) CreateDiscountCode(w http.ResponseWriter, r *http.Request) {
	priceRuleID := chi.URLParam(r, "price_rule_id")
	prid, err := strconv.ParseInt(priceRuleID, 10, 64)
	if err != nil {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		DiscountCode store.DiscountCode `json:"discount_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	dc := req.DiscountCode
	if dc.Code == "" {
		shopifyValidationError(w, "code", []string{"can't be blank"})
		return
	}
	dc.ID = h.store.NextID()
	dc.PriceRuleID = prid
	now := h.store.Now()
	dc.CreatedAt = now
	dc.UpdatedAt = now

	h.store.DiscountCodes.Set(fmt.Sprintf("%d", dc.ID), dc)
	shopifyJSON(w, http.StatusCreated, map[string]any{"discount_code": dc})
}

func (h *Handler) UpdateDiscountCode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	dc, ok := h.store.DiscountCodes.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		DiscountCode store.DiscountCode `json:"discount_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.DiscountCode.Code != "" {
		dc.Code = req.DiscountCode.Code
	}
	dc.UpdatedAt = h.store.Now()
	h.store.DiscountCodes.Set(id, dc)
	shopifyJSON(w, http.StatusOK, map[string]any{"discount_code": dc})
}

func (h *Handler) DeleteDiscountCode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.DiscountCodes.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- Gift Cards ---

func (h *Handler) ListGiftCards(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"gift_cards": h.store.GiftCards.List()})
}

func (h *Handler) GetGiftCard(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	gc, ok := h.store.GiftCards.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"gift_card": gc})
}

func (h *Handler) CreateGiftCard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GiftCard store.GiftCard `json:"gift_card"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	gc := req.GiftCard
	if gc.InitialValue == "" {
		shopifyValidationError(w, "initial_value", []string{"can't be blank"})
		return
	}
	gc.ID = h.store.NextID()
	now := h.store.Now()
	gc.CreatedAt = now
	gc.UpdatedAt = now
	if gc.Balance == "" {
		gc.Balance = gc.InitialValue
	}
	if gc.Currency == "" {
		gc.Currency = "USD"
	}
	if gc.Code == "" {
		gc.Code = fmt.Sprintf("GIFT%d", gc.ID)
	}

	h.store.GiftCards.Set(fmt.Sprintf("%d", gc.ID), gc)
	shopifyJSON(w, http.StatusCreated, map[string]any{"gift_card": gc})
}

func (h *Handler) UpdateGiftCard(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	gc, ok := h.store.GiftCards.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		GiftCard store.GiftCard `json:"gift_card"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.GiftCard.Balance != "" {
		gc.Balance = req.GiftCard.Balance
	}
	gc.Disabled = req.GiftCard.Disabled
	gc.UpdatedAt = h.store.Now()
	h.store.GiftCards.Set(id, gc)
	shopifyJSON(w, http.StatusOK, map[string]any{"gift_card": gc})
}
