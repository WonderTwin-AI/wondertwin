package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

func (h *Handler) CreateCreditNote(w http.ResponseWriter, r *http.Request) {
	if err := parseFormOrJSON(r); err != nil {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parse_error", err.Error())
		return
	}

	invoice := r.FormValue("invoice")
	amountStr := r.FormValue("amount")
	if invoice == "" || amountStr == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required params: invoice, amount.")
		return
	}
	amount, _ := strconv.ParseInt(amountStr, 10, 64)

	cnType := r.FormValue("type")
	if cnType == "" {
		cnType = "post_payment"
	}

	currency := r.FormValue("currency")
	if currency == "" {
		currency = "usd"
	}

	id := h.store.CreditNotes.NextID()
	cn := store.CreditNote{
		ID:       id,
		Object:   "credit_note",
		Invoice:  invoice,
		Customer: r.FormValue("customer"),
		Amount:   amount,
		Currency: currency,
		Reason:   r.FormValue("reason"),
		Status:   "issued",
		Type:     cnType,
		Memo:     r.FormValue("memo"),
		Number:   "CN-" + strings.ToUpper(h.randomHex(4)),
		Livemode: false,
		Metadata: parseMetadata(r),
		Created:  h.store.Now(),
	}

	h.store.CreditNotes.Set(id, cn)
	h.emitEvent("credit_note.created", mapFromJSON(cn))
	twincore.JSON(w, http.StatusOK, cn)
}

func (h *Handler) GetCreditNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cn, ok := h.store.CreditNotes.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such credit note: "+id)
		return
	}
	twincore.JSON(w, http.StatusOK, cn)
}

func (h *Handler) ListCreditNotes(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("starting_after")
	limit := parseLimit(r, 10)
	page := h.store.CreditNotes.Paginate(cursor, limit)
	twincore.JSON(w, http.StatusOK, map[string]any{
		"object": "list", "url": "/v1/credit_notes", "has_more": page.HasMore, "data": page.Data,
	})
}

func (h *Handler) VoidCreditNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cn, ok := h.store.CreditNotes.Get(id)
	if !ok {
		twincore.StripeError(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such credit note: "+id)
		return
	}
	if cn.Status == "void" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_operation", "Credit note is already void.")
		return
	}
	cn.Status = "void"
	h.store.CreditNotes.Set(id, cn)
	h.emitEvent("credit_note.voided", mapFromJSON(cn))
	twincore.JSON(w, http.StatusOK, cn)
}

func (h *Handler) PreviewCreditNote(w http.ResponseWriter, r *http.Request) {
	invoice := r.URL.Query().Get("invoice")
	amountStr := r.URL.Query().Get("amount")
	if invoice == "" || amountStr == "" {
		twincore.StripeError(w, http.StatusBadRequest, "invalid_request_error", "parameter_missing", "Missing required params: invoice, amount.")
		return
	}
	amount, _ := strconv.ParseInt(amountStr, 10, 64)

	cnType := r.URL.Query().Get("type")
	if cnType == "" {
		cnType = "post_payment"
	}

	currency := r.URL.Query().Get("currency")
	if currency == "" {
		currency = "usd"
	}

	cn := store.CreditNote{
		Object:   "credit_note",
		Invoice:  invoice,
		Customer: r.URL.Query().Get("customer"),
		Amount:   amount,
		Currency: currency,
		Reason:   r.URL.Query().Get("reason"),
		Status:   "issued",
		Type:     cnType,
		Memo:     r.URL.Query().Get("memo"),
		Livemode: false,
		Created:  h.store.Now(),
	}

	twincore.JSON(w, http.StatusOK, cn)
}
