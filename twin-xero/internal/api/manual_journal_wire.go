package api

import "github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"

// manualJournal is a ManualJournal on the wire, in and out. Its dates are
// /Date(ms+0000)/ (time-001) but a client may send YYYY-MM-DD.
type manualJournal struct {
	store.ManualJournal
	Date           *Msdate `json:"Date,omitempty"`
	UpdatedDateUTC Msdate  `json:"UpdatedDateUTC"`
}

// inManualJournal returns the submitted manual journal whole. The create
// handler owns resetting every computed field.
func inManualJournal(w manualJournal) store.ManualJournal {
	in := w.ManualJournal
	in.Date = tm(w.Date)
	return in
}

// wireManualJournal readies a stored manual journal for the wire: the lists Xero
// always returns are [], and the dates are msdates.
func wireManualJournal(in store.ManualJournal) manualJournal {
	if in.JournalLines == nil {
		in.JournalLines = []store.JournalLine{}
	}
	for i := range in.JournalLines {
		if in.JournalLines[i].Tracking == nil {
			in.JournalLines[i].Tracking = []store.LineItemTracking{}
		}
	}
	return manualJournal{ManualJournal: in, Date: msd(in.Date), UpdatedDateUTC: Msdate(in.UpdatedDateUTC)}
}
