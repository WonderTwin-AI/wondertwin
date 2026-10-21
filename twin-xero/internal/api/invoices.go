package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// CreateInvoices serves PUT /api.xro/2.0/Invoices: it creates the invoices in
// {"Invoices": [...]} or the one invoice sent bare (crud-001), and answers the
// plural envelope with status 200. If any is invalid nothing is created and the
// answer is a 400 ValidationException that echoes every submitted invoice,
// unless summarizeErrors=false (see below).
func (h *Handler) CreateInvoices(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	var in struct{ Invoices []json.RawMessage }
	if err == nil {
		err = json.Unmarshal(raw, &in)
	}
	if err == nil && in.Invoices == nil {
		in.Invoices = []json.RawMessage{raw} // a bare invoice
	}
	sent := make([]invoice, len(in.Invoices))
	for i, item := range in.Invoices {
		if err == nil {
			err = json.Unmarshal(item, &sent[i])
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
	invoices := h.store.Invoices.For(tenant)
	existing := invoices.List()
	var elements []validationElement
	var made []store.Invoice
	invalid := false
	for i := range sent {
		inv, msgs := h.buildInvoice(tenant, inInvoice(sent[i]), existing, unitdp)
		invalid = invalid || len(msgs) > 0
		elements = append(elements, validationElement{Object: in.Invoices[i], Messages: msgs})
		made = append(made, inv)
		if len(msgs) == 0 {
			existing = append(existing, inv) // a duplicate within the request is a duplicate
		}
	}
	// summarizeErrors is true unless sent as false: one invalid invoice then fails
	// the whole request. As false, the valid ones are created and each invalid one
	// is answered in place with its errors (crud-006).
	if invalid && !strings.EqualFold(r.URL.Query().Get("summarizeErrors"), "false") {
		writeValidationException(w, elements...)
		return
	}
	out := make([]any, len(made))
	for i, inv := range made {
		if msgs := elements[i].Messages; len(msgs) > 0 {
			out[i] = erroredInvoice(in.Invoices[i], msgs)
			continue
		}
		invoices.Set(inv.InvoiceID, inv)
		h.emit(r, tenant, "INVOICE", "CREATE", inv.InvoiceID, nil)
		out[i] = h.wireInvoice(tenant, inv)
	}
	h.writeEnvelope(w, http.StatusOK, "Invoices", out)
}

// buildInvoice validates an invoice and fills in everything Xero computes or
// defaults: IDs, the number, dates, currency, line amounts and totals, and
// AmountDue, which equals Total until a payment arrives (crud-004). Anything
// the client sent for those is ignored. The defaults are guesses where the
// research is silent: status DRAFT, DueDate equal to Date, Date today, and an
// ACCREC number of INV- and four digits.
func (h *Handler) buildInvoice(tenant string, in store.Invoice, existing []store.Invoice, unitdp int) (store.Invoice, []string) {
	now := h.store.Clock.Now().UTC()
	msgs := h.validateInvoice(tenant, in, existing)

	lines, t, lineMsgs := computeLines(in.LineAmountTypes, in.LineItems, unitdp)
	msgs = append(msgs, lineMsgs...)
	chart := h.store.Accounts.For(tenant)
	for i := range lines {
		lines[i].LineItemID, lines[i].AccountID = h.store.NewID(), ""
		for _, a := range chart.List() {
			if a.Code == lines[i].AccountCode {
				lines[i].AccountID = a.AccountID
			}
		}
	}

	if in.Status == "" {
		in.Status = "DRAFT"
	}
	if in.LineAmountTypes == "" {
		in.LineAmountTypes = "Exclusive"
	}
	if in.Date == nil {
		d := now.Truncate(24 * time.Hour)
		in.Date = &d
	}
	if in.DueDate == nil {
		in.DueDate = in.Date
	}
	if in.CurrencyCode == "" {
		in.CurrencyCode = "USD"
		if org, ok := h.store.Organisations.Get(tenant); ok && org.BaseCurrency != "" {
			in.CurrencyCode = org.BaseCurrency
		}
	}
	if in.InvoiceNumber == "" && in.Type == "ACCREC" {
		in.InvoiceNumber = nextInvoiceNumber(existing)
	}
	if len(msgs) > 0 {
		return in, msgs
	}
	in.InvoiceID, in.LineItems = h.store.NewID(), lines
	in.SubTotal, in.TotalTax, in.Total, in.TotalDiscount = t.SubTotal, t.TotalTax, t.Total, t.TotalDiscount
	in.IsDiscounted = t.TotalDiscount > 0
	in.AmountDue, in.AmountPaid, in.AmountCredited, in.FullyPaidOnDate = t.Total, 0, 0, nil
	in.HasErrors, in.HasAttachments = false, false
	in.Payments, in.Prepayments, in.Overpayments, in.CreditNotes, in.Attachments = nil, nil, nil, nil, nil
	in.ValidationErrors, in.Warnings, in.RepeatingInvoiceID = nil, nil, ""
	in.StatusAttributeString = "OK"
	if in.SentToContact && in.Status != "AUTHORISED" { // only an authorised invoice can be marked sent (null-001)
		in.SentToContact, in.StatusAttributeString = false, "WARNING"
		in.Warnings = []store.ValidationError{{Message: "An invoice can only be marked as sent when it is authorised."}}
	}
	in.UpdatedDateUTC, in.UpdatedDateUTCString = now, now.Format("2006-01-02T15:04:05.000")
	return in, nil
}

// nextInvoiceNumber is INV- and four digits, one past the highest number of
// that form in use, as an organisation's default numbering runs.
func nextInvoiceNumber(existing []store.Invoice) string {
	highest := 0
	for _, e := range existing {
		var n int
		if _, err := fmt.Sscanf(strings.TrimPrefix(e.InvoiceNumber, "INV-"), "%d", &n); err == nil && strings.HasPrefix(e.InvoiceNumber, "INV-") && n > highest {
			highest = n
		}
	}
	return fmt.Sprintf("INV-%04d", highest+1)
}

// erroredInvoice is a refused invoice answered in place: the submitted object
// as it was sent, with the reasons it was refused.
func erroredInvoice(submitted json.RawMessage, msgs []string) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal(submitted, &out)
	errs := make([]map[string]string, len(msgs))
	for i, m := range msgs {
		errs[i] = map[string]string{"Message": m}
	}
	out["HasErrors"], out["StatusAttributeString"], out["ValidationErrors"] = true, "ERROR", errs
	return out
}
