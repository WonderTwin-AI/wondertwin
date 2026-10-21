package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// creditNoteStatusChanges are the status moves an update may make, guessed from
// the invoice ones: a PAID, VOIDED or DELETED credit note has none, and an
// authorised one can only be voided (crud-011).
var creditNoteStatusChanges = map[string][]string{
	"DRAFT":      {"DRAFT", "SUBMITTED", "AUTHORISED", "DELETED"},
	"SUBMITTED":  {"SUBMITTED", "AUTHORISED", "DRAFT", "DELETED"},
	"AUTHORISED": {"AUTHORISED", "VOIDED"},
}

// UpdateOrCreateCreditNotes serves POST /api.xro/2.0/CreditNotes (crud-011): a
// credit note that carries the CreditNoteID of a stored one is updated, and one
// with no CreditNoteID is created by the rules of PUT /CreditNotes. An update is
// partial (only the fields the body carries change) and may move the status as
// creditNoteStatusChanges allows; the lines and totals of a credit note that has
// credit applied are fixed. Whether
// Xero also matches on CreditNoteNumber is not settled, so this does not. Refusals
// follow PUT: one invalid credit note fails the request with the 400
// ValidationException, or with summarizeErrors=false the others are written and it
// is answered in place. A CreditNoteID that matches nothing is a 404 whose body is
// a guess and nothing is written. Each write sends a CREDITNOTE event, CREATE or
// UPDATE, and a create's inline Allocations send one INVOICE UPDATE per invoice.
func (h *Handler) UpdateOrCreateCreditNotes(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	var in struct{ CreditNotes []json.RawMessage }
	if err == nil {
		err = json.Unmarshal(raw, &in)
	}
	if err == nil && in.CreditNotes == nil {
		in.CreditNotes = []json.RawMessage{raw} // a bare credit note
	}
	sent := make([]creditNote, len(in.CreditNotes))
	keys := make([]map[string]json.RawMessage, len(in.CreditNotes))
	for i, item := range in.CreditNotes {
		if err == nil {
			err = json.Unmarshal(item, &sent[i])
		}
		if err == nil {
			err = json.Unmarshal(item, &keys[i])
		}
	}
	if err != nil {
		h.writeProblem(w, r, http.StatusBadRequest, "Bad Request", "The request body is not valid JSON")
		return
	}
	unitdp := 2
	if r.URL.Query().Get("unitdp") == "4" {
		unitdp = 4
	}

	tenant := tenantID(r)
	notes := h.store.CreditNotes.For(tenant)
	stored := notes.List()
	for _, s := range sent {
		if id := inCreditNote(s).CreditNoteID; id != "" && !slices.ContainsFunc(stored, func(e store.CreditNote) bool { return strings.EqualFold(e.CreditNoteID, id) }) {
			h.writeProblem(w, r, http.StatusNotFound, "Not Found", "The resource you have specified cannot be found")
			return
		}
	}

	type result struct {
		cn      store.CreditNote
		msgs    []string
		created bool
	}
	existing, results, due, invalid := slices.Clone(stored), make([]result, len(sent)), map[string]float64{}, false
	var elements []validationElement
	for i := range sent {
		one := inCreditNote(sent[i])
		res := result{created: one.CreditNoteID == ""}
		if res.created {
			res.cn, res.msgs = h.buildCreditNote(tenant, one, existing, unitdp)
			if len(res.msgs) == 0 {
				res.msgs = h.planAllocations(tenant, res.cn, due)
			}
			if len(res.msgs) == 0 {
				existing = append(existing, res.cn) // a duplicate within the request is a duplicate
			}
		} else {
			at := slices.IndexFunc(existing, func(e store.CreditNote) bool { return strings.EqualFold(e.CreditNoteID, one.CreditNoteID) })
			res.cn, res.msgs = h.updateCreditNote(tenant, existing[at], one, keys[i], slices.Concat(existing[:at], existing[at+1:]), unitdp)
			if len(res.msgs) == 0 {
				existing[at] = res.cn
			}
		}
		invalid = invalid || len(res.msgs) > 0
		elements = append(elements, validationElement{Object: in.CreditNotes[i], Messages: res.msgs})
		results[i] = res
	}
	if invalid && !strings.EqualFold(r.URL.Query().Get("summarizeErrors"), "false") {
		writeValidationException(w, elements...)
		return
	}
	out := make([]any, len(results))
	reached := map[string]bool{} // an invoice several credit notes allocate to is reported once
	for i, res := range results {
		if len(res.msgs) > 0 {
			out[i] = erroredInvoice(in.CreditNotes[i], res.msgs)
			continue
		}
		cn, eventType := res.cn, "UPDATE"
		if res.created {
			h.applyAllocations(tenant, &cn)
			eventType = "CREATE"
		}
		notes.Set(cn.CreditNoteID, cn)
		h.emit(r, tenant, "CREDITNOTE", eventType, cn.CreditNoteID, creditNoteEventData(cn))
		if res.created {
			for _, a := range cn.Allocations {
				if !reached[a.Invoice.InvoiceID] {
					reached[a.Invoice.InvoiceID] = true
					h.emit(r, tenant, "INVOICE", "UPDATE", a.Invoice.InvoiceID, nil)
				}
			}
		}
		out[i] = h.wireCreditNote(tenant, cn)
	}
	h.writeEnvelope(w, http.StatusOK, "CreditNotes", out)
}

// updateCreditNote overlays the fields an update carries on the stored credit note
// and returns the result, or the reasons it is refused. The wording of the refusals is a guess.
func (h *Handler) updateCreditNote(tenant string, old, in store.CreditNote, keys map[string]json.RawMessage, others []store.CreditNote, unitdp int) (store.CreditNote, []string) {
	if old.Status == "VOIDED" || old.Status == "DELETED" {
		return old, []string{fmt.Sprintf("A credit note with the status %s cannot be updated.", old.Status)}
	}
	out, msgs := old, []string(nil)
	set := func(key string, apply func()) {
		if _, ok := keys[key]; ok {
			apply()
		}
	}
	set("Type", func() {
		if in.Type != old.Type {
			msgs = append(msgs, "The Type of a credit note cannot be changed.")
		}
	})
	set("Status", func() {
		switch {
		case in.Status == "" || in.Status == old.Status:
		case !slices.Contains(creditNoteStatusChanges[old.Status], in.Status):
			msgs = append(msgs, fmt.Sprintf("A credit note cannot change from %s to %s.", old.Status, in.Status))
		default:
			out.Status = in.Status
		}
	})
	set("Reference", func() { out.Reference = in.Reference })
	set("CreditNoteNumber", func() { out.CreditNoteNumber = in.CreditNoteNumber })
	set("BrandingThemeID", func() { out.BrandingThemeID = in.BrandingThemeID })
	set("Date", func() { out.Date = in.Date })
	set("DueDate", func() { out.DueDate = in.DueDate })
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

	spent := old.AppliedAmount > 0 || len(old.Payments) > 0
	_, linesSent := keys["LineItems"]
	if _, typesSent := keys["LineAmountTypes"]; linesSent || typesSent {
		if spent {
			msgs = append(msgs, "A credit note that has credit applied or paid cannot have its line items or amounts changed.")
		} else {
			if linesSent {
				out.LineItems = in.LineItems
			}
			lines, t, lineMsgs := computeLines(out.LineAmountTypes, out.LineItems, unitdp)
			msgs = append(msgs, lineMsgs...)
			chart := h.store.Accounts.For(tenant).List()
			for i := range lines {
				lines[i].LineItemID, lines[i].AccountID = h.store.NewID(), ""
				for _, a := range chart {
					if a.Code == lines[i].AccountCode {
						lines[i].AccountID = a.AccountID
					}
				}
			}
			out.LineItems, out.SubTotal, out.TotalTax, out.Total, out.RemainingCredit = lines, t.SubTotal, t.TotalTax, t.Total, t.Total
		}
	}
	if a, ok := keys["Allocations"]; ok && string(a) != "null" && string(a) != "[]" {
		msgs = append(msgs, "Allocations cannot yet be added to an existing credit note.")
	}
	if out.Status == "VOIDED" && spent {
		msgs = append(msgs, "A credit note with credit applied or paid cannot be voided.")
	}
	if out.Status != "VOIDED" && out.Status != "DELETED" {
		chk := out
		if chk.Status == "PAID" {
			chk.Status = "AUTHORISED" // validateCreditNote only knows the statuses a new credit note can have
		}
		msgs = append(msgs, h.validateCreditNote(tenant, chk, others)...)
	}

	if len(msgs) > 0 {
		return old, msgs
	}

	out.UpdatedDateUTC = h.store.Clock.Now().UTC()
	out.UpdatedDateUTCString = out.UpdatedDateUTC.Format("2006-01-02T15:04:05.000")
	if out.Status == "VOIDED" || out.Status == "DELETED" {
		out.RemainingCredit = 0
	}
	if v, ok := keys["SentToContact"]; ok && string(v) != "null" && out.SentToContact && out.Status != "AUTHORISED" && out.Status != "PAID" { // only an authorised credit note can be marked sent (null-001)
		out.SentToContact, out.StatusAttributeString = false, "WARNING"
		out.Warnings = []store.ValidationError{{Message: "A credit note can only be marked as sent when it is authorised."}}
	}
	return out, nil
}
