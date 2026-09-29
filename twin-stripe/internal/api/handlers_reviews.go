package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// --- Reviews ---

func (h *Handler) GetReview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rev, ok := h.store.Reviews.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such review: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, rev)
}

func (h *Handler) ListReviews(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r, 10)
	page, ok := paginate(w, r, h.store.Reviews, "review", limit, nil)
	if !ok {
		return
	}
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/reviews", "has_more": page.HasMore, "data": page.Data,
	})
}

func (h *Handler) ApproveReview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rev, ok := h.store.Reviews.Get(id)
	if !ok {
		stripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such review: "+id)
		return
	}
	rev.Status = "closed"
	rev.ClosedReason = "approved"
	h.store.Reviews.Set(id, rev)
	h.emitEvent("review.closed", mapFromJSON(rev))
	twincore.JSON(w, http.StatusOK, rev)
}
