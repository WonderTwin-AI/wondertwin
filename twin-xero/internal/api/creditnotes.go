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

// CreateCreditNotes serves PUT /api.xro/2.0/CreditNotes: it creates the credit
// notes in {"CreditNotes": [...]} or the one sent bare (crud-001), and answers
// the plural envelope with status 200. summarizeErrors works as it does for
// invoices (crud-006): by default one invalid credit note fails the request
// with a 400 that echoes every one submitted; as false, each invalid one is
// answered in place and the valid ones are created.
func (h *Handler) CreateCreditNotes(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	var in struct{ CreditNotes []json.RawMessage }
	if err == nil {
		err = json.Unmarshal(raw, &in)
	}
	if err == nil && in.CreditNotes == nil {
		in.CreditNotes = []json.RawMessage{raw} // a bare credit note
	}
	sent := make([]creditNote, len(in.CreditNotes))
	for i, item := range in.CreditNotes {
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
	notes := h.store.CreditNotes.For(tenant)
	existing := notes.List()
	var elements []validationElement
	var made []store.CreditNote
	invalid := false
	due := map[string]float64{}
	for i := range sent {
		cn, msgs := h.buildCreditNote(tenant, inCreditNote(sent[i]), existing, unitdp)
		if len(msgs) == 0 {
			msgs = h.planAllocations(tenant, cn, due)
		}
		invalid = invalid || len(msgs) > 0
		elements = append(elements, validationElement{Object: in.CreditNotes[i], Messages: msgs})
		made = append(made, cn)
		if len(msgs) == 0 {
			existing = append(existing, cn) // a duplicate within the request is a duplicate
		}
	}
	if invalid && !strings.EqualFold(r.URL.Query().Get("summarizeErrors"), "false") {
		writeValidationException(w, elements...)
		return
	}
	out := make([]any, len(made))
	for i, cn := range made {
		if msgs := elements[i].Messages; len(msgs) > 0 {
			out[i] = erroredInvoice(in.CreditNotes[i], msgs)
			continue
		}
		h.applyAllocations(tenant, &cn)
		notes.Set(cn.CreditNoteID, cn)
		out[i] = h.wireCreditNote(tenant, cn)
	}
	h.writeEnvelope(w, http.StatusOK, "CreditNotes", out)
}

// buildCreditNote validates a credit note and fills in everything Xero computes
// or defaults: IDs, the number, dates, currency, line amounts and totals, and
// RemainingCredit, which equals Total until credit is allocated. Anything the
// client sent for those is ignored. The defaults are guesses where the research
// is silent: status DRAFT, Date today, and an ACCRECCREDIT number of CN- and
// four digits.
func (h *Handler) buildCreditNote(tenant string, in store.CreditNote, existing []store.CreditNote, unitdp int) (store.CreditNote, []string) {
	now := h.store.Clock.Now().UTC()
	msgs := h.validateCreditNote(tenant, in, existing)

	lines, t, lineMsgs := computeLines(in.LineAmountTypes, in.LineItems, unitdp)
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
	if in.CurrencyCode == "" {
		in.CurrencyCode = "USD"
		if org, ok := h.store.Organisations.Get(tenant); ok && org.BaseCurrency != "" {
			in.CurrencyCode = org.BaseCurrency
		}
	}
	if in.CreditNoteNumber == "" && in.Type == "ACCRECCREDIT" {
		in.CreditNoteNumber = nextCreditNoteNumber(existing)
	}
	if len(msgs) > 0 {
		return in, msgs
	}
	in.CreditNoteID, in.LineItems = h.store.NewID(), lines
	in.SubTotal, in.TotalTax, in.Total = t.SubTotal, t.TotalTax, t.Total
	in.RemainingCredit, in.AppliedAmount, in.FullyPaidOnDate = t.Total, 0, nil
	in.HasErrors, in.HasAttachments = false, false
	in.Payments, in.ValidationErrors, in.Warnings = nil, nil, nil
	in.StatusAttributeString = "OK"
	if in.SentToContact && in.Status != "AUTHORISED" { // only an authorised credit note can be marked sent (null-001)
		in.SentToContact, in.StatusAttributeString = false, "WARNING"
		in.Warnings = []store.ValidationError{{Message: "A credit note can only be marked as sent when it is authorised."}}
	}
	in.UpdatedDateUTC, in.UpdatedDateUTCString = now, now.Format("2006-01-02T15:04:05.000")
	return in, nil
}

// nextCreditNoteNumber is CN- and four digits, one past the highest number of
// that form in use.
func nextCreditNoteNumber(existing []store.CreditNote) string {
	highest := 0
	for _, e := range existing {
		var n int
		if _, err := fmt.Sscanf(strings.TrimPrefix(e.CreditNoteNumber, "CN-"), "%d", &n); err == nil && strings.HasPrefix(e.CreditNoteNumber, "CN-") && n > highest {
			highest = n
		}
	}
	return fmt.Sprintf("CN-%04d", highest+1)
}
