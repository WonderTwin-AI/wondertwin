package api

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// UpdateOrCreateInvoices serves POST /api.xro/2.0/Invoices (crud-010): an invoice
// that carries the InvoiceID of a stored invoice is updated by the rules of POST
// /Invoices/{InvoiceID}, and one with no InvoiceID is created by the rules of PUT
// /Invoices. Whether Xero also matches on InvoiceNumber is not settled, so this
// does not: a number that belongs to another invoice is refused as PUT refuses
// it. Refusals follow PUT: one invalid invoice fails the request with the 400
// ValidationException, or with summarizeErrors=false the others are written and
// it is answered in place. An InvoiceID that matches nothing is a 404 whose body
// is a guess, and nothing is written. Each write sends an INVOICE event.
func (h *Handler) UpdateOrCreateInvoices(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	var in struct{ Invoices []json.RawMessage }
	if err == nil {
		err = json.Unmarshal(raw, &in)
	}
	if err == nil && in.Invoices == nil {
		in.Invoices = []json.RawMessage{raw} // a bare invoice
	}
	sent := make([]invoice, len(in.Invoices))
	keys := make([]map[string]json.RawMessage, len(in.Invoices))
	for i, item := range in.Invoices {
		if err == nil {
			in.Invoices[i], keys[i], err = normalizeInvoice(item)
		}
		if err == nil {
			err = json.Unmarshal(in.Invoices[i], &sent[i])
		}
	}
	if err != nil {
		h.writeBadBody(w, r, raw, err)
		return
	}
	unitdp := 2
	if r.URL.Query().Get("unitdp") == "4" {
		unitdp = 4
	}

	tenant := tenantID(r)
	invoices := h.store.Invoices.For(tenant)
	stored := invoices.List()
	for _, s := range sent {
		if id := inInvoice(s).InvoiceID; id != "" && !slices.ContainsFunc(stored, func(e store.Invoice) bool { return strings.EqualFold(e.InvoiceID, id) }) {
			h.writeProblem(w, r, http.StatusNotFound, "Not Found", "The resource you have specified cannot be found")
			return
		}
	}

	type result struct {
		inv     store.Invoice
		msgs    []string
		created bool
	}
	existing, results, invalid := slices.Clone(stored), make([]result, len(sent)), false
	var elements []validationElement
	for i := range sent {
		one := inInvoice(sent[i])
		res := result{created: one.InvoiceID == ""}
		if res.created {
			res.inv, res.msgs = h.buildInvoice(tenant, one, existing, unitdp)
			if len(res.msgs) == 0 {
				existing = append(existing, res.inv) // a duplicate within the request is a duplicate
			}
		} else {
			at := slices.IndexFunc(existing, func(e store.Invoice) bool { return strings.EqualFold(e.InvoiceID, one.InvoiceID) })
			res.inv, res.msgs = h.applyInvoiceUpdate(tenant, existing[at], one, keys[i], slices.Concat(existing[:at], existing[at+1:]), unitdp)
			if len(res.msgs) == 0 {
				existing[at] = res.inv
			}
		}
		invalid = invalid || len(res.msgs) > 0
		elements = append(elements, validationElement{Object: in.Invoices[i], Messages: res.msgs})
		results[i] = res
	}
	if invalid && !strings.EqualFold(r.URL.Query().Get("summarizeErrors"), "false") {
		writeValidationException(w, elements...)
		return
	}
	out := make([]any, len(results))
	for i, res := range results {
		if len(res.msgs) > 0 {
			out[i] = erroredInvoice(in.Invoices[i], res.msgs)
			continue
		}
		eventType := "UPDATE"
		if res.created {
			eventType = "CREATE"
		}
		invoices.Set(res.inv.InvoiceID, res.inv)
		h.emit(r, tenant, "INVOICE", eventType, res.inv.InvoiceID, nil)
		out[i] = h.wireInvoice(tenant, res.inv)
	}
	h.writeEnvelope(w, http.StatusOK, "Invoices", out)
}
