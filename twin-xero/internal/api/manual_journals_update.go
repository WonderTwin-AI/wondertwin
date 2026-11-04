package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// manualJournalStatusChanges are the moves an update may make, inferred from
// Xero's status list (DELETED is "a deleted draft", VOIDED "a voided posted"
// journal; crud-020): a DRAFT may be posted or deleted, a POSTED journal may only
// be voided, and a DELETED or VOIDED one has no moves.
var manualJournalStatusChanges = map[string][]string{
	"DRAFT":  {"DRAFT", "POSTED", "DELETED"},
	"POSTED": {"POSTED", "VOIDED"},
}

// UpdateManualJournal serves POST /api.xro/2.0/ManualJournals/{ManualJournalID}.
// The body is a partial update in {"ManualJournals": [...]} or bare: only the
// fields it carries change. Journal lines carry no ID, so JournalLines sent
// replace every line. Only a DRAFT may change its fields; a POSTED journal may
// only be voided. A refused update is a 400 ValidationException; an unknown ID
// is a 404 whose body is a guess.
func (h *Handler) UpdateManualJournal(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	var env struct{ ManualJournals []json.RawMessage }
	if err == nil {
		err = json.Unmarshal(raw, &env)
	}
	one := json.RawMessage(raw)
	if err == nil && env.ManualJournals != nil {
		if len(env.ManualJournals) == 0 {
			err = fmt.Errorf("no manual journal")
		} else {
			one = env.ManualJournals[0]
		}
	}
	var sent manualJournal
	keys := map[string]json.RawMessage{}
	if err == nil {
		err = json.Unmarshal(one, &keys)
	}
	if err == nil {
		err = decodeBody(one, &sent)
	}
	if err != nil {
		h.writeBadBody(w, r, err)
		return
	}
	tenant := tenantID(r)
	saved := h.store.ManualJournals.For(tenant)
	all := saved.List()
	at := slices.IndexFunc(all, func(m store.ManualJournal) bool {
		return strings.EqualFold(m.ManualJournalID, chi.URLParam(r, "ManualJournalID"))
	})
	if at < 0 {
		h.writeProblem(w, r, http.StatusNotFound, "Not Found", "The resource you have specified cannot be found")
		return
	}
	upd, msgs := h.applyManualJournalUpdate(tenant, all[at], inManualJournal(sent), keys)
	if len(msgs) > 0 {
		writeValidationException(w, validationElement{Object: one, Messages: msgs})
		return
	}
	saved.Set(upd.ManualJournalID, upd)
	h.writeEnvelope(w, http.StatusOK, "ManualJournals", []any{wireManualJournal(upd)})
}

// applyManualJournalUpdate overlays the fields the update carries on the stored
// manual journal and returns the result, or the reasons it is refused. A
// journal that stays or becomes POSTED is checked again, balance included.
func (h *Handler) applyManualJournalUpdate(tenant string, old, in store.ManualJournal, keys map[string]json.RawMessage) (store.ManualJournal, []string) {
	moves, open := manualJournalStatusChanges[old.Status]
	if !open {
		return old, []string{fmt.Sprintf("A manual journal with the status %s cannot be updated.", old.Status)}
	}
	out := old
	out.JournalLines = slices.Clone(old.JournalLines)
	if _, ok := keys["Status"]; ok && in.Status != "" {
		if !slices.Contains(moves, in.Status) {
			return old, []string{fmt.Sprintf("A manual journal cannot change from %s to %s.", old.Status, in.Status)}
		}
		out.Status = in.Status
	}
	set := func(key string, apply func()) {
		if _, ok := keys[key]; ok {
			apply()
		}
	}
	set("Narration", func() { out.Narration = in.Narration })
	set("Date", func() { out.Date = in.Date })
	set("Url", func() { out.URL = in.URL })
	set("LineAmountTypes", func() { out.LineAmountTypes = in.LineAmountTypes })
	set("ShowOnCashBasisReports", func() { out.ShowOnCashBasisReports = in.ShowOnCashBasisReports })
	set("JournalLines", func() { out.JournalLines = in.JournalLines })
	if old.Status != "DRAFT" {
		sent := out
		sent.JournalLines = slices.Clone(out.JournalLines)
		h.validateManualJournal(tenant, &sent)
		if manualJournalDetails(sent) != manualJournalDetails(old) {
			return old, []string{"Only a DRAFT manual journal can have its details changed."}
		}
		status := out.Status
		out = old
		out.JournalLines = slices.Clone(old.JournalLines)
		out.Status = status
	}
	if out.Status == "DRAFT" || out.Status == "POSTED" {
		if msgs := h.validateManualJournal(tenant, &out); len(msgs) > 0 {
			return old, msgs
		}
	}
	out.StatusAttributeString, out.UpdatedDateUTC = "OK", h.store.Clock.Now().UTC()
	return out, nil
}

// manualJournalDetails renders the fields only a DRAFT may change, as the wire
// carries them, so a journal sent back unchanged compares equal to the stored one.
func manualJournalDetails(m store.ManualJournal) string {
	w := wireManualJournal(store.ManualJournal{
		Narration:              m.Narration,
		Date:                   m.Date,
		JournalLines:           slices.Clone(m.JournalLines),
		LineAmountTypes:        m.LineAmountTypes,
		URL:                    m.URL,
		ShowOnCashBasisReports: m.ShowOnCashBasisReports,
	})
	b, _ := json.Marshal(w)
	return string(b)
}
