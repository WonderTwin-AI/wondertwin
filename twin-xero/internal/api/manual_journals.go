package api

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// CreateManualJournals serves PUT /api.xro/2.0/ManualJournals: it creates the
// journals in {"ManualJournals": [...]} or the one sent bare (crud-001), and
// answers the plural envelope. If any is invalid nothing is created and the
// answer is a 400 ValidationException that echoes every submitted journal,
// unless summarizeErrors=false, which creates the valid ones and answers each
// invalid one in place (crud-006). Xero sends no webhook event for a journal.
func (h *Handler) CreateManualJournals(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	var in struct{ ManualJournals []json.RawMessage }
	if err == nil {
		err = json.Unmarshal(raw, &in)
	}
	if err == nil && in.ManualJournals == nil {
		in.ManualJournals = []json.RawMessage{raw}
	}
	sent := make([]manualJournal, len(in.ManualJournals))
	keys := make([]map[string]json.RawMessage, len(in.ManualJournals))
	for i, item := range in.ManualJournals {
		if err == nil {
			err = decodeBody(item, &sent[i])
		}
		if err == nil {
			err = json.Unmarshal(item, &keys[i])
		}
	}
	if err != nil {
		h.writeBadBody(w, r, err)
		return
	}
	tenant := tenantID(r)
	made := make([]store.ManualJournal, len(sent))
	var elements []validationElement
	invalid := false
	for i := range sent {
		mj, msgs := h.buildManualJournal(tenant, inManualJournal(sent[i]), keys[i])
		invalid = invalid || len(msgs) > 0
		elements = append(elements, validationElement{Object: in.ManualJournals[i], Messages: msgs})
		made[i] = mj
	}
	if invalid && !strings.EqualFold(r.URL.Query().Get("summarizeErrors"), "false") {
		writeValidationException(w, elements...)
		return
	}
	saved := h.store.ManualJournals.For(tenant)
	out := make([]any, len(made))
	for i, mj := range made {
		if msgs := elements[i].Messages; len(msgs) > 0 {
			out[i] = erroredInvoice(in.ManualJournals[i], msgs)
			continue
		}
		saved.Set(mj.ManualJournalID, mj)
		out[i] = wireManualJournal(mj)
	}
	h.writeEnvelope(w, http.StatusOK, "ManualJournals", out)
}

// buildManualJournal validates a new manual journal and fills in what Xero
// computes or defaults: the ID, each line's AccountID, Status DRAFT, today's
// date (time-004, a guess from invoices), LineAmountTypes NoTax and
// ShowOnCashBasisReports true. Anything the client sent for those is ignored.
func (h *Handler) buildManualJournal(tenant string, in store.ManualJournal, keys map[string]json.RawMessage) (store.ManualJournal, []string) {
	now := h.store.Clock.Now().UTC()
	switch in.Status {
	case "":
		in.Status = "DRAFT"
	case "DRAFT", "POSTED":
	default:
		return in, []string{fmt.Sprintf("A manual journal cannot be created with the status %s.", in.Status)}
	}
	if in.LineAmountTypes == "" {
		in.LineAmountTypes = "NoTax"
	}
	if _, ok := keys["ShowOnCashBasisReports"]; !ok {
		in.ShowOnCashBasisReports = true
	}
	if msgs := h.validateManualJournal(tenant, &in); len(msgs) > 0 {
		return in, msgs
	}
	if in.Date == nil {
		d := now.Truncate(24 * time.Hour)
		in.Date = &d
	}
	in.ManualJournalID, in.StatusAttributeString, in.UpdatedDateUTC = h.store.NewID(), "OK", now
	in.HasAttachments, in.Warnings, in.ValidationErrors = false, nil, nil
	return in, nil
}

// validateManualJournal returns the reasons a manual journal is refused, and
// resolves each line's account. A Narration and at least two lines are
// required, each naming an account in the chart. A line whose account code
// matches no account is refused, by operator decision 2026-11-03: the spec's
// example has Xero remove such a line with a Warning (crud-020). A POSTED
// journal must balance, with the spec's own message; whether a DRAFT must too is
// not settled, and this app emulator does not ask it to. The other wording is a
// guess.
func (h *Handler) validateManualJournal(tenant string, in *store.ManualJournal) []string {
	var msgs []string
	if strings.TrimSpace(in.Narration) == "" {
		msgs = append(msgs, "Narration must be specified.")
	}
	if len(in.JournalLines) < 2 {
		msgs = append(msgs, "A manual journal must have at least two journal lines.")
	}
	chart := h.store.Accounts.For(tenant).List()
	var debits, credits float64
	for i := range in.JournalLines {
		l := &in.JournalLines[i]
		var hit *store.Account
		for j := range chart {
			if (l.AccountID != "" && strings.EqualFold(chart[j].AccountID, l.AccountID)) || (l.AccountID == "" && chart[j].Code == l.AccountCode) {
				hit = &chart[j]
			}
		}
		switch {
		case l.AccountCode == "" && l.AccountID == "":
			msgs = append(msgs, fmt.Sprintf("Journal line %d: an account code must be specified.", i+1))
		case hit == nil && l.AccountID != "":
			msgs = append(msgs, fmt.Sprintf("Journal line %d: account %s does not match a recognised account.", i+1, l.AccountID))
		case hit == nil:
			msgs = append(msgs, fmt.Sprintf("Account code '%s' does not match a recognised account.", l.AccountCode))
		default:
			l.AccountCode, l.AccountID = hit.Code, hit.AccountID
		}
		l.LineAmount = round(l.LineAmount, 2)
		if l.TaxAmount == nil || in.LineAmountTypes == "NoTax" {
			l.TaxAmount = ptr(0)
		}
		if l.LineAmount > 0 {
			debits += l.LineAmount
		} else {
			credits += l.LineAmount
		}
	}
	if in.Status == "POSTED" && math.Abs(debits+credits) > 0.005 {
		msgs = append(msgs, fmt.Sprintf("The total debits (%.2f) must equal total credits (%.2f)", debits, credits))
	}
	return msgs
}
