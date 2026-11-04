package api

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

// UpdateOrCreateManualJournals serves POST /api.xro/2.0/ManualJournals
// (crud-022): a journal that carries the ManualJournalID of a stored one is
// updated by the rules of POST /ManualJournals/{ManualJournalID}, and one with no
// ManualJournalID is created by the rules of PUT /ManualJournals. Matching is by
// ID only, and a repeated ID applies its items in order. One invalid item fails
// the request with the 400 ValidationException, or with summarizeErrors=false
// the others are written and it is answered in place. A ManualJournalID that
// matches nothing is a 404 whose body is a guess, and nothing is written.
func (h *Handler) UpdateOrCreateManualJournals(w http.ResponseWriter, r *http.Request) {
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
	saved := h.store.ManualJournals.For(tenant)
	stored := saved.List()
	find := func(id string) int {
		return slices.IndexFunc(stored, func(e store.ManualJournal) bool { return strings.EqualFold(e.ManualJournalID, id) })
	}
	for _, s := range sent {
		if id := s.ManualJournalID; id != "" && find(id) < 0 {
			h.writeProblem(w, r, http.StatusNotFound, "Not Found", "The resource you have specified cannot be found")
			return
		}
	}

	made := make([]store.ManualJournal, len(sent))
	var elements []validationElement
	invalid := false
	for i, s := range sent {
		var msgs []string
		if id := s.ManualJournalID; id != "" {
			at := find(id)
			made[i], msgs = h.applyManualJournalUpdate(tenant, stored[at], inManualJournal(s), keys[i])
			if len(msgs) == 0 {
				stored[at] = made[i]
			}
		} else {
			made[i], msgs = h.buildManualJournal(tenant, inManualJournal(s), keys[i])
		}
		invalid = invalid || len(msgs) > 0
		elements = append(elements, validationElement{Object: in.ManualJournals[i], Messages: msgs})
	}
	if invalid && !strings.EqualFold(r.URL.Query().Get("summarizeErrors"), "false") {
		writeValidationException(w, elements...)
		return
	}
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
