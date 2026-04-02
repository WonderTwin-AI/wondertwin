package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twin-shopify/internal/store"
)

func (h *Handler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"webhooks": h.store.Webhooks.List()})
}

func (h *Handler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	wh, ok := h.store.Webhooks.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	shopifyJSON(w, http.StatusOK, map[string]any{"webhook": wh})
}

func (h *Handler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Webhook store.Webhook `json:"webhook"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	wh := req.Webhook
	if wh.Topic == "" {
		shopifyValidationError(w, "topic", []string{"can't be blank"})
		return
	}
	if wh.Address == "" {
		shopifyValidationError(w, "address", []string{"can't be blank"})
		return
	}
	wh.ID = h.store.NextID()
	now := h.store.Now()
	wh.CreatedAt = now
	wh.UpdatedAt = now
	wh.Active = true
	if wh.Format == "" {
		wh.Format = "json"
	}

	h.store.Webhooks.Set(fmt.Sprintf("%d", wh.ID), wh)
	shopifyJSON(w, http.StatusCreated, map[string]any{"webhook": wh})
}

func (h *Handler) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	wh, ok := h.store.Webhooks.Get(id)
	if !ok {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Webhook store.Webhook `json:"webhook"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shopifyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Webhook.Address != "" {
		wh.Address = req.Webhook.Address
	}
	if req.Webhook.Topic != "" {
		wh.Topic = req.Webhook.Topic
	}
	if req.Webhook.Format != "" {
		wh.Format = req.Webhook.Format
	}
	wh.UpdatedAt = h.store.Now()
	h.store.Webhooks.Set(id, wh)
	shopifyJSON(w, http.StatusOK, map[string]any{"webhook": wh})
}

func (h *Handler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.store.Webhooks.Delete(id) {
		shopifyError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) CountWebhooks(w http.ResponseWriter, r *http.Request) {
	shopifyJSON(w, http.StatusOK, map[string]any{"count": h.store.Webhooks.Count()})
}
