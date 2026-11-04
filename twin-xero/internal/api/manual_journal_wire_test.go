package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
)

func TestManualJournalWireReadsDatesAndWritesMsdates(t *testing.T) {
	var w manualJournal
	in := `{"Narration":"Advance as a loan","Date":"2026-11-03","JournalLines":[{"LineAmount":5000,"AccountCode":"090"},` +
		`{"LineAmount":-5000,"AccountCode":"2100","Description":"Loan"}]}`
	if err := json.Unmarshal([]byte(in), &w); err != nil {
		t.Fatal(err)
	}
	mj := inManualJournal(w)
	if mj.Date == nil || !mj.Date.Equal(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date = %v", mj.Date)
	}
	if mj.Narration != "Advance as a loan" || len(mj.JournalLines) != 2 || mj.JournalLines[1].LineAmount != -5000 {
		t.Fatalf("the narration and lines must survive the way in, credits negative: %+v", mj)
	}

	mj.UpdatedDateUTC = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	out, _ := json.Marshal(wireManualJournal(mj))
	var m struct {
		Date, UpdatedDateUTC string
		JournalLines         []map[string]any
	}
	_ = json.Unmarshal(out, &m)
	for _, s := range []string{m.Date, m.UpdatedDateUTC} {
		if !msdateValue.MatchString(s) {
			t.Errorf("%q is not /Date(ms+0000)/ (time-001)", s)
		}
	}
	if l, ok := m.JournalLines[0]["Tracking"].([]any); !ok || l == nil {
		t.Errorf("a line's Tracking must be [], got %v", m.JournalLines[0]["Tracking"])
	}

	empty, _ := json.Marshal(wireManualJournal(store.ManualJournal{}))
	var loose map[string]any
	_ = json.Unmarshal(empty, &loose)
	if l, ok := loose["JournalLines"].([]any); !ok || l == nil {
		t.Errorf("JournalLines must be [], got %v", loose["JournalLines"])
	}
	if _, ok := loose["Date"]; ok {
		t.Error("Date is unset and must be omitted, not null")
	}
}
