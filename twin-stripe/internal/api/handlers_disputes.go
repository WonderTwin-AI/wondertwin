package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func (h *Handler) GetDispute(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, ok := h.store.Disputes.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such dispute: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, d)
}

func (h *Handler) CloseDispute(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, ok := h.store.Disputes.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such dispute: "+id)
		return
	}
	if d.Status == "won" || d.Status == "lost" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "dispute_already_closed",
			"This dispute is already closed with status: "+d.Status+".")
		return
	}

	d.Status = "lost"
	h.store.Disputes.Set(id, d)
	h.dispatcher.Enqueue("charge.dispute.closed", mapFromJSON(d))
	twincore.JSON(w, http.StatusOK, d)
}

func (h *Handler) ListDisputes(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.Disputes.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/disputes",
		"has_more": page.HasMore,
		"data":     page.Data,
	})
}
