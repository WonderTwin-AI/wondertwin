package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// UpdateInvoice serves POST /api.xro/2.0/Invoices/{InvoiceID}. The path names the
// invoice by ID or by number (ident-004). The body is a partial update: only the
// fields it carries change, and a whole invoice read earlier can be sent back
// with one field changed, as localstack/finance-ops does. The answer is the
// plural envelope; a refused update is a 400 ValidationException, or with
// summarizeErrors=false a 200 with the errors in place. An unknown invoice is a
// 404 whose body is a guess. Status changes are a later slice, so until it lands
// the Status a body carries is ignored.
func (h *Handler) UpdateInvoice(w http.ResponseWriter, r *http.Request) {
	one, keys, err := readOneInvoice(r)
	var sent invoice
	if err == nil {
		err = json.Unmarshal(one, &sent)
	}
	if err != nil {
		h.writeProblem(w, r, http.StatusBadRequest, "Bad Request", "The request body is not valid JSON")
		return
	}
	tenant := tenantID(r)
	invoices := h.store.Invoices.For(tenant)
	old, others, ok := findInvoice(invoices.List(), chi.URLParam(r, "InvoiceID"))
	if !ok {
		h.writeProblem(w, r, http.StatusNotFound, "Not Found", "The resource you have specified cannot be found")
		return
	}
	unitdp := 2
	if r.URL.Query().Get("unitdp") == "4" {
		unitdp = 4
	}
	upd, msgs := h.applyInvoiceUpdate(tenant, old, inInvoice(sent), keys, others, unitdp)
	if len(msgs) > 0 {
		if strings.EqualFold(r.URL.Query().Get("summarizeErrors"), "false") {
			h.writeEnvelope(w, http.StatusOK, "Invoices", []any{erroredInvoice(one, msgs)})
			return
		}
		writeValidationException(w, validationElement{Object: one, Messages: msgs})
		return
	}
	invoices.Set(upd.InvoiceID, upd)
	h.writeEnvelope(w, http.StatusOK, "Invoices", []any{h.wireInvoice(tenant, upd)})
}

// readOneInvoice reads the invoice from {"Invoices": [...]} or a bare body, and
// the names of the fields it carries. Xero accepts SentToContact as the string
// "true" (its own example sends it), so that string is read as a boolean.
func readOneInvoice(r *http.Request) (json.RawMessage, map[string]json.RawMessage, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		return nil, nil, err
	}
	var in struct{ Invoices []json.RawMessage }
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, nil, err
	}
	one := json.RawMessage(raw)
	if in.Invoices != nil {
		if len(in.Invoices) == 0 {
			return nil, nil, fmt.Errorf("no invoice")
		}
		one = in.Invoices[0]
	}
	keys := map[string]json.RawMessage{}
	if err := json.Unmarshal(one, &keys); err != nil {
		return nil, nil, err
	}
	var s string
	if v, ok := keys["SentToContact"]; ok && len(v) > 0 && v[0] == '"' && json.Unmarshal(v, &s) == nil { // null is not a string
		keys["SentToContact"] = json.RawMessage(strconv.FormatBool(strings.EqualFold(s, "true")))
		one, _ = json.Marshal(keys)
	}
	return one, keys, nil
}

// findInvoice finds the invoice by ID, or failing that by number, and returns
// every other invoice for the uniqueness check.
func findInvoice(all []store.Invoice, ref string) (store.Invoice, []store.Invoice, bool) {
	at := slices.IndexFunc(all, func(e store.Invoice) bool { return strings.EqualFold(e.InvoiceID, ref) })
	if at < 0 {
		at = slices.IndexFunc(all, func(e store.Invoice) bool {
			return e.InvoiceNumber == ref && e.Status != "VOIDED" && e.Status != "DELETED"
		})
	}
	if at < 0 {
		return store.Invoice{}, nil, false
	}
	return all[at], slices.Concat(all[:at], all[at+1:]), true
}

// applyInvoiceUpdate overlays the fields the update carries on the stored invoice
// and returns the result, or the reasons it is refused. Fields Xero computes or
// owns (the amounts, IDs, payments) are ignored whatever the body says. Line items
// are matched by LineItemID: a line with a known ID updates that line, a line
// without one is new, and a stored line left out is deleted, so LineItems sent
// without IDs replace every line (crud-005). The wording of the refusals is a guess.
func (h *Handler) applyInvoiceUpdate(tenant string, old, in store.Invoice, keys map[string]json.RawMessage, others []store.Invoice, unitdp int) (store.Invoice, []string) {
	out, msgs := old, []string(nil)
	set := func(key string, apply func()) {
		if _, ok := keys[key]; ok {
			apply()
		}
	}
	set("Type", func() {
		if in.Type != old.Type {
			msgs = append(msgs, "The Type of an invoice cannot be changed.")
		}
	})
	set("Reference", func() { out.Reference = in.Reference })
	set("InvoiceNumber", func() { out.InvoiceNumber = in.InvoiceNumber })
	set("BrandingThemeID", func() { out.BrandingThemeID = in.BrandingThemeID })
	set("Url", func() { out.URL = in.URL })
	set("Date", func() { out.Date = in.Date })
	set("DueDate", func() { out.DueDate = in.DueDate })
	set("ExpectedPaymentDate", func() { out.ExpectedPaymentDate = in.ExpectedPaymentDate })
	set("PlannedPaymentDate", func() { out.PlannedPaymentDate = in.PlannedPaymentDate })
	set("LineAmountTypes", func() { out.LineAmountTypes = in.LineAmountTypes })
	set("CurrencyCode", func() { out.CurrencyCode = in.CurrencyCode })
	set("CurrencyRate", func() { out.CurrencyRate = in.CurrencyRate })
	set("Contact", func() {
		if in.Contact != nil {
			out.Contact = in.Contact
		}
	})
	if v, ok := keys["SentToContact"]; ok && string(v) != "null" { // null leaves it unchanged (null-001)
		out.SentToContact = in.SentToContact
	}
	_, linesSent := keys["LineItems"]
	if linesSent {
		known := map[string]bool{}
		for _, l := range old.LineItems {
			known[l.LineItemID] = true
		}
		out.LineItems = in.LineItems
		for i := range out.LineItems {
			if !known[out.LineItems[i].LineItemID] {
				out.LineItems[i].LineItemID = ""
			}
		}
	}
	if _, ok := keys["LineAmountTypes"]; linesSent || ok {
		if !linesSent { // the stored tax was worked out under the old type
			out.LineItems = slices.Clone(old.LineItems)
			for i := range out.LineItems {
				out.LineItems[i].TaxAmount = nil
			}
		}
		lines, t, lineMsgs := computeLines(out.LineAmountTypes, out.LineItems, unitdp)
		msgs = append(msgs, lineMsgs...)
		chart := h.store.Accounts.For(tenant).List()
		for i := range lines {
			if lines[i].LineItemID == "" {
				lines[i].LineItemID = h.store.NewID()
			}
			lines[i].AccountID = ""
			for _, a := range chart {
				if a.Code == lines[i].AccountCode {
					lines[i].AccountID = a.AccountID
				}
			}
		}
		out.LineItems = lines
		out.SubTotal, out.TotalTax, out.Total, out.TotalDiscount = t.SubTotal, t.TotalTax, t.Total, t.TotalDiscount
		out.IsDiscounted = t.TotalDiscount > 0
	}
	msgs = append(msgs, h.validateInvoice(tenant, out, others)...)
	if len(msgs) > 0 {
		return old, msgs
	}

	now := h.store.Clock.Now().UTC()
	out.AmountDue = round(out.Total-out.AmountPaid-out.AmountCredited, 2)
	if out.SentToContact && out.Status != "AUTHORISED" && out.Status != "PAID" {
		out.SentToContact, out.StatusAttributeString = false, "WARNING"
		out.Warnings = []store.ValidationError{{Message: "An invoice can only be marked as sent when it is authorised."}}
	} else {
		out.StatusAttributeString, out.Warnings = "OK", nil
	}
	out.UpdatedDateUTC, out.UpdatedDateUTCString = now, now.Format("2006-01-02T15:04:05.000")
	return out, nil
}
